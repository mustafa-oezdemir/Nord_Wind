package orders

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
)

var ErrInvalidTransition = errors.New("Statuswechsel nicht erlaubt")
var ErrInvalidTracking = errors.New("Sendungsnummer ungültig")

var trackingPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 -]{4,63}$`)

// List liefert die Bestellhistorie. customerID ist für Adminabfragen leer.
func (r *Repository) List(ctx context.Context, customerID string) ([]Order, error) {
	query := `SELECT customer_id, order_number FROM orders WHERE customer_id = $1 ORDER BY created_at DESC, id DESC`
	args := []any{customerID}
	if customerID == "" {
		query = `SELECT customer_id, order_number FROM orders ORDER BY created_at DESC, id DESC`
		args = nil
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type key struct{ customer, number string }
	keys := []key{}
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.customer, &item.number); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	result := make([]Order, 0, len(keys))
	for _, item := range keys {
		order, err := get(ctx, r.db, item.customer, item.number)
		if err != nil {
			return nil, err
		}
		result = append(result, order)
	}
	return result, nil
}

func (r *Repository) AdminGet(ctx context.Context, number string) (Order, error) {
	var customerID string
	err := r.db.QueryRowContext(ctx, `SELECT customer_id FROM orders WHERE order_number = $1`, number).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	return get(ctx, r.db, customerID, number)
}

// StaffTransition sperrt die Bestellung und schreibt Status, Sendung und Akteur atomar.
// Bei SHIPPED ist ein manueller Tracking-Code nötig; andere Ziele dürfen keinen enthalten.
func (r *Repository) StaffTransition(ctx context.Context, number string, target Status, tracking, actor, actorID string) (Order, error) {
	if actor != "admin" && actor != "employee" {
		return Order{}, ErrInvalidTransition
	}
	tracking = strings.TrimSpace(tracking)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()
	var id, customerID string
	var current Status
	err = tx.QueryRowContext(ctx, `SELECT id, customer_id, status FROM orders WHERE order_number = $1 FOR UPDATE`, number).
		Scan(&id, &customerID, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if !CanTransition(current, target) || (target != Processing && target != Shipped && target != Delivered) {
		return Order{}, ErrInvalidTransition
	}
	if (target == Shipped && !trackingPattern.MatchString(tracking)) || (target != Shipped && tracking != "") {
		return Order{}, ErrInvalidTracking
	}
	if target == Shipped {
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_shipments (order_id, carrier, tracking_number)
			VALUES ($1, 'MANUAL', $2)`, id, tracking); err != nil {
			return Order{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET status = $2, updated_at = now() WHERE id = $1`, id, target); err != nil {
		return Order{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO order_status_events (order_id, from_status, to_status, actor, actor_customer_id)
		VALUES ($1, $2, $3, $4, $5)`, id, current, target, actor, actorID); err != nil {
		return Order{}, err
	}
	if target == Shipped {
		if _, err := tx.ExecContext(ctx, `INSERT INTO shipment_attempts (order_id, outcome) VALUES ($1, 'success')`, id); err != nil {
			return Order{}, err
		}
	}
	order, err := get(ctx, tx, customerID, number)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(); err != nil {
		return Order{}, err
	}
	return order, nil
}

// RecordShipmentFailure macht abgewiesene Versandversuche dauerhaft zählbar.
func (r *Repository) RecordShipmentFailure(ctx context.Context, number, outcome string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO shipment_attempts (order_id, outcome)
		SELECT id, $2 FROM orders WHERE order_number = $1`, number, outcome)
	return err
}

func (r *Repository) ShipmentMetrics(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT outcome, count(*) FROM shipment_attempts GROUP BY outcome`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{"success": 0, "invalid_state": 0, "invalid_tracking": 0, "shipment_failed": 0}
	for rows.Next() {
		var outcome string
		var count int64
		if err := rows.Scan(&outcome, &count); err != nil {
			return nil, err
		}
		result[outcome] = count
	}
	return result, rows.Err()
}
