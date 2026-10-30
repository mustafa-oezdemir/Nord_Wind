package inventory

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"
)

const DefaultTTL = 15 * time.Minute

var (
	ErrInvalidLines = errors.New("ungültige Reservierungspositionen")
	ErrUnavailable  = errors.New("Bestand nicht verfügbar")
	ErrExpired      = errors.New("Reservierung abgelaufen")
	ErrState        = errors.New("Bestellstatus erlaubt diese Änderung nicht")
)

type Line struct {
	VariantID string
	Quantity  int32
}

// Reserve muss in derselben Transaktion wie Bestellkopf und Positions-Snapshots laufen.
// Der Aufrufer muss bei jedem Fehler die gesamte Transaktion zurückrollen.
func Reserve(ctx context.Context, tx *sql.Tx, orderID string, lines []Line, expiresAt time.Time) error {
	if orderID == "" || len(lines) == 0 || len(lines) > 50 || !expiresAt.After(time.Now()) {
		return ErrInvalidLines
	}
	ordered := append([]Line(nil), lines...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].VariantID < ordered[j].VariantID })
	for index, line := range ordered {
		if line.VariantID == "" || line.Quantity < 1 || line.Quantity > 99 ||
			(index > 0 && ordered[index-1].VariantID == line.VariantID) {
			return ErrInvalidLines
		}
	}
	for _, line := range ordered {
		var variantID string
		err := tx.QueryRowContext(ctx, `UPDATE product_variants v
			SET stock_quantity = stock_quantity - $2, updated_at = now()
			WHERE v.id = $1 AND v.stock_quantity >= $2 AND v.is_active
				AND EXISTS (SELECT 1 FROM products p WHERE p.id = v.product_id AND p.status = 'published')
			RETURNING v.id`, line.VariantID, line.Quantity).Scan(&variantID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUnavailable
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_reservations
			(order_id, variant_id, quantity, expires_at) VALUES ($1, $2, $3, $4)`,
			orderID, line.VariantID, line.Quantity, expiresAt); err != nil {
			return err
		}
	}
	return nil
}

// Release gibt aktive Positionen genau einmal frei. Der Bestellkopf wird zuerst gesperrt;
// Varianten werden auch hier in stabiler ID-Reihenfolge aktualisiert.
func Release(ctx context.Context, tx *sql.Tx, orderID string) (bool, error) {
	status, err := lockOrder(ctx, tx, orderID)
	if err != nil {
		return false, err
	}
	if status == "PAID" || status == "PROCESSING" || status == "SHIPPED" || status == "DELIVERED" {
		return false, ErrState
	}
	rows, err := tx.QueryContext(ctx, `SELECT variant_id, quantity FROM order_reservations
		WHERE order_id = $1 AND state = 'active' ORDER BY variant_id FOR UPDATE`, orderID)
	if err != nil {
		return false, err
	}
	var lines []Line
	for rows.Next() {
		var line Line
		if err := rows.Scan(&line.VariantID, &line.Quantity); err != nil {
			rows.Close()
			return false, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()
	for _, line := range lines {
		if _, err := tx.ExecContext(ctx, `UPDATE product_variants SET stock_quantity = stock_quantity + $2,
			updated_at = now() WHERE id = $1`, line.VariantID, line.Quantity); err != nil {
			return false, err
		}
	}
	if len(lines) > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE order_reservations SET state = 'released', settled_at = now()
			WHERE order_id = $1 AND state = 'active'`, orderID); err != nil {
			return false, err
		}
	}
	return len(lines) > 0, nil
}

// Consume bestätigt eine rechtzeitige Zahlung. Späte Webhooks dürfen keinen
// bereits freigegebenen Bestand nachträglich verkaufen.
func Consume(ctx context.Context, tx *sql.Tx, orderID string) error {
	status, err := lockOrder(ctx, tx, orderID)
	if err != nil {
		return err
	}
	if status != "PAYMENT_PENDING" {
		return ErrState
	}
	var total, active, expired int
	if err := tx.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE state = 'active'),
		count(*) FILTER (WHERE state = 'active' AND expires_at <= clock_timestamp())
		FROM order_reservations WHERE order_id = $1`, orderID).Scan(&total, &active, &expired); err != nil {
		return err
	}
	if total == 0 || active != total {
		return ErrState
	}
	if expired > 0 {
		return ErrExpired
	}
	_, err = tx.ExecContext(ctx, `UPDATE order_reservations SET state = 'consumed', settled_at = now()
		WHERE order_id = $1 AND state = 'active'`, orderID)
	return err
}

func lockOrder(ctx context.Context, tx *sql.Tx, orderID string) (string, error) {
	var status string
	err := tx.QueryRowContext(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrState
	}
	return status, err
}

// ReleaseExpired verarbeitet höchstens limit Bestellungen pro Aufruf. Mehrere
// Worker können dank SKIP LOCKED gleichzeitig laufen, ohne doppelt freizugeben.
func ReleaseExpired(ctx context.Context, db *sql.DB, now time.Time, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, ErrInvalidLines
	}
	released := 0
	for released < limit {
		found := false
		err := WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
			found = false
			var orderID, status string
			err := tx.QueryRowContext(ctx, `SELECT o.id, o.status FROM orders o
				WHERE o.status IN ('CREATED', 'PAYMENT_PENDING') AND EXISTS (
					SELECT 1 FROM order_reservations r WHERE r.order_id = o.id
					AND r.state = 'active' AND r.expires_at <= $1)
				ORDER BY o.id FOR UPDATE OF o SKIP LOCKED LIMIT 1`, now).Scan(&orderID, &status)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			found, err = Release(ctx, tx, orderID)
			if err != nil {
				return err
			}
			nextStatus := "PAYMENT_FAILED"
			if status == "CREATED" {
				nextStatus = "CANCELLED"
			}
			_, err = tx.ExecContext(ctx, `UPDATE orders SET status = $2, updated_at = now()
				WHERE id = $1 AND status IN ('CREATED', 'PAYMENT_PENDING')`, orderID, nextStatus)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO order_status_events
				(order_id, from_status, to_status, actor) VALUES ($1, $2, $3, 'system')`,
				orderID, status, nextStatus)
			return err
		})
		if err != nil {
			return released, err
		}
		if !found {
			break
		}
		released++
	}
	return released, nil
}

// Cancel gibt eine offene Reservierung frei und markiert die Bestellung als storniert.
func Cancel(ctx context.Context, db *sql.DB, orderID string) error {
	return WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		status, err := lockOrder(ctx, tx, orderID)
		if err != nil {
			return err
		}
		if status == "CANCELLED" {
			return nil
		}
		if status != "CREATED" && status != "PAYMENT_PENDING" {
			return ErrState
		}
		if _, err := Release(ctx, tx, orderID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE orders SET status = 'CANCELLED', updated_at = now()
			WHERE id = $1`, orderID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO order_status_events
			(order_id, from_status, to_status, actor) VALUES ($1, $2, 'CANCELLED', 'customer')`,
			orderID, status)
		return err
	})
}
