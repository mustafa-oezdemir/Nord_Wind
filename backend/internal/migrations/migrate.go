package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
)

//go:embed sql/*.up.sql
var files embed.FS

const version = 1

// Up führt die erste Migration genau einmal in einer gesperrten Transaktion aus.
func Up(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(703001)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	var applied bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
		return err
	}
	if !applied {
		migration, readErr := files.ReadFile("sql/0001_bootstrap.up.sql")
		if readErr != nil {
			return readErr
		}
		if _, err = tx.ExecContext(ctx, string(migration)); err != nil {
			return fmt.Errorf("Migration %d: %w", version, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Check bestätigt, dass die erwartete Migration angewendet wurde.
func Check(ctx context.Context, db *sql.DB) error {
	var applied bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
		return err
	}
	if !applied {
		return fmt.Errorf("Migration %d fehlt", version)
	}
	return nil
}
