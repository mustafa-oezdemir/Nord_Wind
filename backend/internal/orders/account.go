package orders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
)

var ErrOpenOrders = errors.New("offene Bestellungen vorhanden")
var ErrInvalidAddress = errors.New("Adresse ungültig")

type SavedAddress struct {
	ID      string  `json:"id"`
	Label   string  `json:"label"`
	Address Address `json:"address"`
}

func (r *Repository) Addresses(ctx context.Context, customerID string) ([]SavedAddress, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, label, address FROM customer_addresses
		WHERE customer_id = $1 ORDER BY created_at, id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SavedAddress{}
	for rows.Next() {
		var item SavedAddress
		var raw []byte
		if err := rows.Scan(&item.ID, &item.Label, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.Address); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) SaveAddress(ctx context.Context, customerID, id, label string, address Address) (SavedAddress, error) {
	label = strings.TrimSpace(label)
	if len([]rune(label)) < 1 || len([]rune(label)) > 60 || strings.IndexFunc(label, unicode.IsControl) >= 0 || len(address.Validate()) != 0 {
		return SavedAddress{}, ErrInvalidAddress
	}
	raw, err := json.Marshal(address)
	if err != nil {
		return SavedAddress{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return SavedAddress{}, err
	}
	defer tx.Rollback()
	var active bool
	if err := tx.QueryRowContext(ctx, `SELECT anonymized_at IS NULL FROM customers WHERE id = $1 FOR UPDATE`, customerID).Scan(&active); errors.Is(err, sql.ErrNoRows) {
		return SavedAddress{}, ErrNotFound
	} else if err != nil {
		return SavedAddress{}, err
	}
	if !active {
		return SavedAddress{}, ErrNotFound
	}
	var result SavedAddress
	if id == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO customer_addresses (customer_id, label, address)
			VALUES ($1, $2, $3) RETURNING id`, customerID, label, raw).Scan(&result.ID)
	} else {
		err = tx.QueryRowContext(ctx, `UPDATE customer_addresses SET label = $3, address = $4, updated_at = now()
			WHERE customer_id = $1 AND id = $2 RETURNING id`, customerID, id, label, raw).Scan(&result.ID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return SavedAddress{}, ErrNotFound
	}
	if err != nil {
		return SavedAddress{}, err
	}
	result.Label, result.Address = label, address
	if err := tx.Commit(); err != nil {
		return SavedAddress{}, err
	}
	return result, nil
}

func (r *Repository) DeleteAddress(ctx context.Context, customerID, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM customer_addresses WHERE customer_id = $1 AND id = $2`, customerID, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

type AccountExport struct {
	Email      string         `json:"email"`
	Addresses  []SavedAddress `json:"addresses"`
	Orders     []Order        `json:"orders"`
	ExportedAt time.Time      `json:"exported_at"`
}

func (r *Repository) Export(ctx context.Context, customerID string) (AccountExport, error) {
	var result AccountExport
	if err := r.db.QueryRowContext(ctx, `SELECT email FROM customers WHERE id = $1`, customerID).Scan(&result.Email); err != nil {
		return result, err
	}
	var err error
	result.Addresses, err = r.Addresses(ctx, customerID)
	if err != nil {
		return result, err
	}
	// Der Export umfasst alle Bestellungen und deren unveränderte Snapshots.
	rows, err := r.db.QueryContext(ctx, `SELECT order_number FROM orders WHERE customer_id = $1 ORDER BY created_at, id`, customerID)
	if err != nil {
		return result, err
	}
	numbers := []string{}
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			rows.Close()
			return result, err
		}
		numbers = append(numbers, number)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	result.Orders = []Order{}
	for _, number := range numbers {
		order, err := r.Get(ctx, customerID, number)
		if err != nil {
			return result, err
		}
		result.Orders = append(result.Orders, order)
	}
	result.ExportedAt = time.Now().UTC()
	return result, nil
}

// CloseAccount entfernt Zugang und Adressbuch. Buchungsbelege bleiben als Snapshot
// erhalten; die Aufbewahrung und spätere Löschung brauchen eine Betriebsregel.
func (r *Repository) CloseAccount(ctx context.Context, customerID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE id = $1 AND anonymized_at IS NULL FOR UPDATE`, customerID).Scan(&id); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var open bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE customer_id = $1
		AND status NOT IN ('DELIVERED', 'CANCELLED'))`, customerID).Scan(&open); err != nil {
		return err
	}
	if open {
		return ErrOpenOrders
	}
	for _, query := range []string{
		`DELETE FROM auth_sessions WHERE customer_id = $1`,
		`DELETE FROM cart_items WHERE customer_id = $1`,
		`DELETE FROM cart_discounts WHERE customer_id = $1`,
		`DELETE FROM customer_addresses WHERE customer_id = $1`,
	} {
		if _, err := tx.ExecContext(ctx, query, customerID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE customers SET email = 'closed-' || id::text || '@example.invalid',
		password_hash = NULL, anonymized_at = now() WHERE id = $1`, customerID); err != nil {
		return err
	}
	return tx.Commit()
}
