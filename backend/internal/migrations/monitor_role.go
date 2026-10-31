package migrations

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
)

var monitorRoleName = regexp.MustCompile(`^[a-z_][a-z0-9_]{2,62}$`)

type roleStatement struct {
	query string
	args  []any
}

// GrantMonitor richtet die Rolle für den PostgreSQL-Exporter ein (ADR-012). Sie darf
// sich anmelden und über pg_monitor Statistiken lesen, aber keine Tabellen der
// Anwendung lesen oder ändern und keine Objekte anlegen.
func GrantMonitor(ctx context.Context, db *sql.DB, role, password string) error {
	if !monitorRoleName.MatchString(role) || role == "nordwinds_runtime" || role == "postgres" {
		return errors.New("NORDWINDS_DB_MONITOR_USER muss ein eigener Rollenname aus Kleinbuchstaben, Ziffern und _ sein")
	}
	if len(password) < 32 {
		return errors.New("NORDWINDS_DB_MONITOR_PASSWORD muss mindestens 32 Zeichen haben")
	}
	var isOwner, exists bool
	if err := db.QueryRowContext(ctx,
		`SELECT $1::text = current_user, EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1::text)`, role).
		Scan(&isOwner, &exists); err != nil {
		return err
	}
	if isOwner {
		return errors.New("NORDWINDS_DB_MONITOR_USER darf nicht die Eigentümerrolle der Datenbank sein")
	}
	// PostgreSQL parametrisiert Rollennamen und Passwörter in DDL nicht; format(%I/%L)
	// erzeugt serverseitig korrekt gequotete Bezeichner und Literale.
	var statements []roleStatement
	if !exists {
		statements = append(statements, roleStatement{`SELECT format('CREATE ROLE %I LOGIN', $1::text)`, []any{role}})
	}
	statements = append(statements,
		roleStatement{`SELECT format('ALTER ROLE %I WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 3 PASSWORD %L', $1::text, $2::text)`, []any{role, password}},
		roleStatement{`SELECT format('GRANT pg_monitor TO %I', $1::text)`, []any{role}},
		roleStatement{`SELECT format('REVOKE CREATE ON SCHEMA public FROM %I', $1::text)`, []any{role}},
	)
	for _, builder := range statements {
		var statement string
		if err := db.QueryRowContext(ctx, builder.query, builder.args...).Scan(&statement); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
