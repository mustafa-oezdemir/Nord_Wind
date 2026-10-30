package identity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound  = errors.New("identity not found")
	ErrDuplicate = errors.New("identity duplicate")
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) CreateCustomer(ctx context.Context, email, passwordHash string) (Customer, error) {
	var result Customer
	err := r.db.QueryRowContext(ctx, `INSERT INTO customers (email, password_hash)
		VALUES ($1, $2) RETURNING id, email, role, created_at`, email, passwordHash).
		Scan(&result.ID, &result.Email, &result.Role, &result.CreatedAt)
	return result, classifyError(err)
}

func (r *Repository) CustomerByEmail(ctx context.Context, email string) (Customer, string, error) {
	var result Customer
	var hash sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, email, role, created_at, password_hash
		FROM customers WHERE lower(email) = lower($1)`, email).
		Scan(&result.ID, &result.Email, &result.Role, &result.CreatedAt, &hash)
	return result, hash.String, classifyError(err)
}

func (r *Repository) ReplaceSession(ctx context.Context, oldHash, newHash []byte, customer Customer, csrfToken string, now time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= $1 OR idle_expires_at <= $1`, now); err != nil {
		return err
	}
	if len(oldHash) == 32 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = $1`, oldHash); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO auth_sessions
		(token_hash, customer_id, role_at_issue, csrf_token, created_at, expires_at, idle_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, newHash, customer.ID, customer.Role, csrfToken,
		now, now.Add(12*time.Hour), now.Add(30*time.Minute))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ActiveSession(ctx context.Context, tokenHash []byte, now time.Time) (Session, error) {
	var result Session
	err := r.db.QueryRowContext(ctx, `WITH active AS (
		UPDATE auth_sessions SET idle_expires_at = LEAST(expires_at, $2::timestamptz + interval '30 minutes')
		WHERE token_hash = $1 AND expires_at > $2 AND idle_expires_at > $2
		RETURNING customer_id, role_at_issue, csrf_token, expires_at
	) SELECT c.id, c.email, c.role, c.created_at, active.csrf_token, active.expires_at
	FROM active JOIN customers c ON c.id = active.customer_id AND c.role = active.role_at_issue`, tokenHash, now).
		Scan(&result.Customer.ID, &result.Customer.Email, &result.Customer.Role,
			&result.Customer.CreatedAt, &result.CSRFToken, &result.ExpiresAt)
	return result, classifyError(err)
}

func (r *Repository) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auth_sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (r *Repository) CountLoginAttempt(ctx context.Context, keyHash []byte, now time.Time) (int, error) {
	var attempts int
	err := r.db.QueryRowContext(ctx, `INSERT INTO auth_rate_limits (key_hash, window_started_at, attempts)
		VALUES ($1, $2, 1)
		ON CONFLICT (key_hash) DO UPDATE SET
		attempts = CASE WHEN auth_rate_limits.window_started_at <= EXCLUDED.window_started_at - interval '15 minutes'
			THEN 1 ELSE LEAST(auth_rate_limits.attempts + 1, 1000000) END,
		window_started_at = CASE WHEN auth_rate_limits.window_started_at <= EXCLUDED.window_started_at - interval '15 minutes'
			THEN EXCLUDED.window_started_at ELSE auth_rate_limits.window_started_at END
		RETURNING attempts`, keyHash, now).Scan(&attempts)
	return attempts, err
}

func (r *Repository) ClearLoginAttempts(ctx context.Context, keyHash []byte) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auth_rate_limits WHERE key_hash = $1`, keyHash)
	return err
}

func (r *Repository) PruneLoginAttempts(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM auth_rate_limits WHERE window_started_at < $1::timestamptz - interval '1 day'`, now)
	return err
}

func classifyError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicate
	}
	return err
}
