package inventory

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const maxTransactionAttempts = 3

// WithDeadlockRetry wiederholt nur vollständig zurückgerollte DB-Transaktionen.
// callback darf keine externen Seiteneffekte ausführen.
func WithDeadlockRetry(ctx context.Context, db *sql.DB, callback func(*sql.Tx) error) error {
	for attempt := 0; attempt < maxTransactionAttempts; attempt++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		err = callback(tx)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			_ = tx.Rollback()
		}
		if err == nil || !retryable(err) || attempt == maxTransactionAttempts-1 {
			return err
		}
		backoff := time.NewTimer(time.Duration(10*(1<<attempt)) * time.Millisecond)
		select {
		case <-ctx.Done():
			backoff.Stop()
			return ctx.Err()
		case <-backoff.C:
		}
	}
	return nil
}

func retryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001")
}
