package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
)

//go:embed sql/*.sql
var files embed.FS

var migrationNames = []string{"bootstrap", "domain_foundation", "catalog_search", "identity", "admin_audit", "product_images", "cart", "pricing", "orders", "inventory_reservations", "mock_payments", "shipping", "account", "employee_role"}

const migrationLockID = 703001

// Up wendet fehlende Migrationen in einer gesperrten Transaktion an.
func Up(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lock(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, tx)
	if err != nil {
		return err
	}
	for version := applied + 1; version <= len(migrationNames); version++ {
		if err := executeMigration(ctx, tx, version, "up"); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Down nimmt genau eine Migration zurück. Die SQL-Datei verweigert Datenverlust.
func Down(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lock(ctx, tx); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, tx)
	if err != nil {
		return err
	}
	if applied == 0 {
		return fmt.Errorf("keine Migration zum Zurücknehmen vorhanden")
	}
	if err := executeMigration(ctx, tx, applied, "down"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, applied); err != nil {
		return err
	}
	return tx.Commit()
}

// Check bestätigt einen lückenlosen Stand bis zur neuesten Migration.
func Check(ctx context.Context, db *sql.DB) error {
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	if applied != len(migrationNames) {
		return fmt.Errorf("Migration %d fehlt", applied+1)
	}
	return nil
}

func lock(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID)
	return err
}

type versionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func appliedVersions(ctx context.Context, db versionQueryer) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return 0, err
		}
		count++
		if version != count || count > len(migrationNames) {
			return 0, fmt.Errorf("unbekannter oder lückenhafter Migrationsstand: %d", version)
		}
	}
	return count, rows.Err()
}

func executeMigration(ctx context.Context, tx *sql.Tx, version int, direction string) error {
	path := fmt.Sprintf("sql/%04d_%s.%s.sql", version, migrationNames[version-1], direction)
	content, err := files.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("Migration %d %s: %w", version, direction, err)
	}
	return nil
}
