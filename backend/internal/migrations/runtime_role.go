package migrations

import (
	"context"
	"database/sql"
	"errors"
)

// GrantRuntime richtet nach den Migrationen eine Rolle ohne DDL- und
// Superuser-Rechte ein. Der Eigentümerzugang bleibt dem Migrationsjob vorbehalten.
func GrantRuntime(ctx context.Context, db *sql.DB, password string) error {
	if len(password) < 32 {
		return errors.New("NORDWINDS_DB_RUNTIME_PASSWORD muss mindestens 32 Zeichen haben")
	}
	statements := []string{
		`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nordwinds_runtime') THEN
				CREATE ROLE nordwinds_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
			END IF;
		END $$`,
		`GRANT USAGE ON SCHEMA public TO nordwinds_runtime`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO nordwinds_runtime`,
		`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO nordwinds_runtime`,
		`REVOKE ALL ON TABLE schema_migrations FROM nordwinds_runtime`,
		`REVOKE UPDATE, DELETE ON TABLE admin_audit_events FROM nordwinds_runtime`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	// PostgreSQL parametrisiert Passwörter nicht in ALTER ROLE. format(%L)
	// erzeugt serverseitig ein korrekt gequotetes SQL-Literal.
	var alterRole string
	if err := db.QueryRowContext(ctx,
		`SELECT format('ALTER ROLE nordwinds_runtime WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD %L', $1::text)`,
		password).Scan(&alterRole); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, alterRole)
	return err
}
