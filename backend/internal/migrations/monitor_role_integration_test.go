package migrations

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestMonitorRoleOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_MONITOR_ROLE_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_MONITOR_ROLE_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	const role = "nordwinds_monitor_test"
	t.Cleanup(func() { _, _ = db.Exec(`DROP ROLE IF EXISTS ` + role) })

	for _, invalid := range []struct{ role, password string }{
		{"Monitor; DROP TABLE orders", strings.Repeat("p", 32)},
		{"nordwinds_runtime", strings.Repeat("p", 32)},
		{role, "zu-kurz"},
	} {
		if err := GrantMonitor(ctx, db, invalid.role, invalid.password); err == nil {
			t.Fatalf("ungültige Eingabe akzeptiert: %q", invalid.role)
		}
	}
	var owner string
	if err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if monitorRoleName.MatchString(owner) {
		if err := GrantMonitor(ctx, db, owner, strings.Repeat("p", 32)); err == nil {
			t.Fatal("Eigentümerrolle als Monitor-Rolle akzeptiert")
		}
	}

	password := "Monitor-Test-Passwort-mit-'Quote'-und-$-2026"
	for run := 0; run < 2; run++ { // zweimal: Wiederholung beim nächsten migrate up
		if err := GrantMonitor(ctx, db, role, password); err != nil {
			t.Fatalf("Lauf %d: %v", run+1, err)
		}
	}
	var superuser, createRole, member bool
	var limit int
	if err := db.QueryRowContext(ctx, `SELECT rolsuper, rolcreaterole, rolconnlimit, pg_has_role($1, 'pg_monitor', 'member')
		FROM pg_roles WHERE rolname = $1`, role).Scan(&superuser, &createRole, &limit, &member); err != nil {
		t.Fatal(err)
	}
	if superuser || createRole || limit != 3 || !member {
		t.Fatalf("Rechte: superuser=%v createrole=%v limit=%d pg_monitor=%v", superuser, createRole, limit, member)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	parsed.User = url.UserPassword(role, password)
	monitor, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	defer monitor.Close()
	var backends int
	if err := monitor.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity`).Scan(&backends); err != nil || backends < 1 {
		t.Fatalf("Statistik nicht lesbar: %d %v", backends, err)
	}
	var size int64
	if err := monitor.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&size); err != nil || size <= 0 {
		t.Fatalf("Datenbankgröße nicht lesbar: %v", err)
	}
	for _, forbidden := range []string{
		`SELECT count(*) FROM customers`,
		`SELECT count(*) FROM orders`,
		`CREATE TABLE monitor_darf_das_nicht (id int)`,
	} {
		_, err := monitor.ExecContext(ctx, forbidden)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s: erwartet 42501, erhalten %v", forbidden, err)
		}
	}
}
