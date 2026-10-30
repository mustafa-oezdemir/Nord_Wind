package payment

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/inventory"
)

var (
	ErrNotFound     = errors.New("Mock-Zahlung nicht gefunden")
	ErrConflict     = errors.New("Mock-Zahlung im Konflikt")
	ErrInvalidEvent = errors.New("ungültiges Mock-Ereignis")
)

type Payment struct {
	ID          string `json:"id"`
	OrderID     string `json:"order_id"`
	Method      Method `json:"method"`
	State       string `json:"state"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// Initiate ist nach Order-ID idempotent. Die Methode kann nach der ersten
// Initiierung nicht für dieselbe Bestellung gewechselt werden.
func (r *Repository) Initiate(ctx context.Context, customerID, orderID string, method Method) (Payment, error) {
	if !method.Valid() {
		return Payment{}, ErrConflict
	}
	newID, err := NewID("pay_")
	if err != nil {
		return Payment{}, err
	}
	var result Payment
	err = inventory.WithDeadlockRetry(ctx, r.db, func(tx *sql.Tx) error {
		var owner, status, currency string
		var amount int64
		err := tx.QueryRowContext(ctx, `SELECT customer_id, status, total_cents, currency
			FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&owner, &status, &amount, &currency)
		if errors.Is(err, sql.ErrNoRows) || owner != customerID {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "CREATED" && status != "PAYMENT_PENDING" {
			return ErrConflict
		}
		var active, expired int
		if err := tx.QueryRowContext(ctx, `SELECT count(*), count(*) FILTER (WHERE expires_at <= clock_timestamp())
			FROM order_reservations WHERE order_id = $1 AND state = 'active'`, orderID).Scan(&active, &expired); err != nil {
			return err
		}
		if active == 0 || expired > 0 {
			return inventory.ErrExpired
		}
		err = tx.QueryRowContext(ctx, `SELECT id, order_id, method, state, amount_cents, currency
			FROM mock_payments WHERE order_id = $1`, orderID).Scan(&result.ID, &result.OrderID,
			&result.Method, &result.State, &result.AmountCents, &result.Currency)
		if err == nil {
			if result.Method != method {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		result = Payment{ID: newID, OrderID: orderID, Method: method, State: "pending", AmountCents: amount, Currency: currency}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mock_payments
			(id, order_id, method, amount_cents, currency) VALUES ($1, $2, $3, $4, $5)`,
			result.ID, orderID, method, amount, currency); err != nil {
			return err
		}
		if status == "CREATED" {
			if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'PAYMENT_PENDING', updated_at = now()
				WHERE id = $1`, orderID); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO order_status_events
				(order_id, from_status, to_status, actor)
				VALUES ($1, 'CREATED', 'PAYMENT_PENDING', 'customer')`, orderID)
			return err
		}
		return nil
	})
	return result, err
}

func (r *Repository) Get(ctx context.Context, customerID, paymentID string) (Payment, error) {
	var result Payment
	err := r.db.QueryRowContext(ctx, `SELECT mp.id, mp.order_id, mp.method, mp.state,
		mp.amount_cents, mp.currency FROM mock_payments mp JOIN orders o ON o.id = mp.order_id
		WHERE mp.id = $1 AND o.customer_id = $2`, paymentID, customerID).Scan(
		&result.ID, &result.OrderID, &result.Method, &result.State, &result.AmountCents, &result.Currency)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	return result, err
}

func (r *Repository) GetByOrder(ctx context.Context, customerID, orderID string) (Payment, error) {
	var result Payment
	err := r.db.QueryRowContext(ctx, `SELECT mp.id, mp.order_id, mp.method, mp.state,
		mp.amount_cents, mp.currency FROM mock_payments mp JOIN orders o ON o.id = mp.order_id
		WHERE mp.order_id = $1 AND o.customer_id = $2`, orderID, customerID).Scan(
		&result.ID, &result.OrderID, &result.Method, &result.State, &result.AmountCents, &result.Currency)
	if errors.Is(err, sql.ErrNoRows) {
		return Payment{}, ErrNotFound
	}
	return result, err
}

// SyncExpired gleicht Mock-Zahlungen nach einem Reservierungsablauf ab.
func (r *Repository) SyncExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE mock_payments mp SET state = 'failed', updated_at = now()
		FROM orders o WHERE o.id = mp.order_id AND o.status IN ('PAYMENT_FAILED', 'CANCELLED')
			AND mp.state = 'pending' AND EXISTS (
				SELECT 1 FROM order_reservations r WHERE r.order_id = o.id AND r.state = 'released')`)
	return err
}

// Process prüft die bereits signierte Nutzlast und ändert Zahlung, Bestellung
// und Reservierung in einer einzigen Transaktion. Jede Event-ID wirkt einmal.
func (r *Repository) Process(ctx context.Context, event Event, raw []byte) error {
	if !event.Valid() {
		return ErrInvalidEvent
	}
	bodyHash := sha256.Sum256(raw)
	return inventory.WithDeadlockRetry(ctx, r.db, func(tx *sql.Tx) error {
		var orderID, orderStatus string
		err := tx.QueryRowContext(ctx, `SELECT id, status FROM orders WHERE id = $1 FOR UPDATE`, event.OrderID).Scan(&orderID, &orderStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidEvent
		}
		if err != nil {
			return err
		}
		var stored Payment
		err = tx.QueryRowContext(ctx, `SELECT id, order_id, method, state, amount_cents, currency
			FROM mock_payments WHERE id = $1 FOR UPDATE`, event.PaymentID).Scan(&stored.ID,
			&stored.OrderID, &stored.Method, &stored.State, &stored.AmountCents, &stored.Currency)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidEvent
		}
		if err != nil {
			return err
		}
		if stored.OrderID != event.OrderID || stored.Method != event.Method ||
			stored.AmountCents != event.AmountCents || stored.Currency != event.Currency {
			return ErrInvalidEvent
		}
		var oldHash []byte
		err = tx.QueryRowContext(ctx, `SELECT body_hash FROM mock_payment_events WHERE id = $1`, event.ID).Scan(&oldHash)
		if err == nil {
			if !equalHash(oldHash, bodyHash[:]) {
				return ErrInvalidEvent
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		disposition := "applied"
		switch event.Outcome {
		case Succeeded:
			if stored.State != "pending" {
				disposition = "ignored"
				break
			}
			if err := inventory.Consume(ctx, tx, stored.OrderID); err != nil {
				if errors.Is(err, inventory.ErrExpired) || errors.Is(err, inventory.ErrState) {
					disposition = "manual_review"
					break
				}
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE mock_payments SET state = 'succeeded', updated_at = now()
				WHERE id = $1`, stored.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'PAID', updated_at = now()
				WHERE id = $1`, stored.OrderID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO order_status_events
				(order_id, from_status, to_status, actor)
				VALUES ($1, 'PAYMENT_PENDING', 'PAID', 'system')`, stored.OrderID); err != nil {
				return err
			}
		case Failed:
			if stored.State != "pending" {
				disposition = "ignored"
				break
			}
			if orderStatus == "CANCELLED" || orderStatus == "PAYMENT_FAILED" {
				disposition = "ignored"
				if _, err := tx.ExecContext(ctx, `UPDATE mock_payments SET state = 'failed', updated_at = now()
					WHERE id = $1`, stored.ID); err != nil {
					return err
				}
				break
			}
			if _, err := inventory.Release(ctx, tx, stored.OrderID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE mock_payments SET state = 'failed', updated_at = now()
				WHERE id = $1`, stored.ID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = 'PAYMENT_FAILED', updated_at = now()
				WHERE id = $1`, stored.OrderID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO order_status_events
				(order_id, from_status, to_status, actor)
				VALUES ($1, 'PAYMENT_PENDING', 'PAYMENT_FAILED', 'system')`, stored.OrderID); err != nil {
				return err
			}
		case Refunded:
			if stored.State == "refunded" {
				disposition = "ignored"
				break
			}
			if stored.State != "succeeded" {
				disposition = "manual_review"
				break
			}
			if _, err := tx.ExecContext(ctx, `UPDATE mock_payments SET state = 'refunded', updated_at = now()
				WHERE id = $1`, stored.ID); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO mock_payment_events
			(id, payment_id, body_hash, outcome, disposition) VALUES ($1, $2, $3, $4, $5)`,
			event.ID, event.PaymentID, bodyHash[:], event.Outcome, disposition)
		return err
	})
}

func equalHash(left, right []byte) bool {
	return subtle.ConstantTimeCompare(left, right) == 1
}

// MarshalEvent ist die einzige Serialisierung für intern ausgelöste Simulationen.
func MarshalEvent(event Event) ([]byte, error) { return json.Marshal(event) }
