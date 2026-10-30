package orders

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/inventory"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

// CreateInput ist eine bestätigte Bestellung aus der Kasse.
type CreateInput struct {
	CustomerID     string
	IdempotencyKey string
	// Address muss normalisiert und gültig sein.
	Address Address
	// ExpectedTotalCents ist die Gesamtsumme, die dem Kunden vor dem Klick angezeigt wurde.
	ExpectedTotalCents int64
}

// RequestHash ist der Fingerabdruck der fachlichen Anfrage. Dieselbe Idempotency-Key darf
// nur mit demselben Fingerabdruck wiederholt werden.
func (in CreateInput) RequestHash() []byte {
	payload, _ := json.Marshal(struct {
		Address            Address `json:"address"`
		ExpectedTotalCents int64   `json:"expected_total_cents"`
	}{in.Address, in.ExpectedTotalCents})
	sum := sha256.Sum256(payload)
	return sum[:]
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// Create legt Bestellung, Positionen und ersten Statuseintrag an und leert den Warenkorb –
// alles in einer Transaktion. Ein bereits verarbeiteter Idempotency-Key liefert die
// vorhandene Bestellung mit replayed = true.
//
// Die Kundensperre serialisiert Checkout und Warenkorbänderungen desselben Kunden.
// Der atomare Bestandsabzug verhindert auch bei verschiedenen Kunden Überverkäufe.
func (r *Repository) Create(ctx context.Context, in CreateInput) (order Order, replayed bool, err error) {
	err = inventory.WithDeadlockRetry(ctx, r.db, func(tx *sql.Tx) error {
		order, replayed, err = r.createTx(ctx, tx, in)
		return err
	})
	if errors.Is(err, inventory.ErrUnavailable) {
		fresh, loadErr := cart.Load(ctx, r.db, in.CustomerID)
		if loadErr != nil {
			return Order{}, false, loadErr
		}
		return Order{}, false, &CartChangedError{Reason: ReasonItemUnavailable, Cart: fresh}
	}
	return order, replayed, err
}

func (r *Repository) createTx(ctx context.Context, tx *sql.Tx, in CreateInput) (order Order, replayed bool, err error) {

	var email string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM customers WHERE id = $1 FOR UPDATE`,
		in.CustomerID).Scan(&email); err != nil {
		return Order{}, false, err
	}

	hash := in.RequestHash()
	var existingNumber string
	var existingHash []byte
	err = tx.QueryRowContext(ctx, `SELECT order_number, request_hash FROM orders
		WHERE customer_id = $1 AND idempotency_key = $2`, in.CustomerID, in.IdempotencyKey).
		Scan(&existingNumber, &existingHash)
	switch {
	case err == nil:
		if !bytes.Equal(existingHash, hash) {
			return Order{}, false, ErrIdempotencyMismatch
		}
		order, err := get(ctx, tx, in.CustomerID, existingNumber)
		return order, true, err
	case !errors.Is(err, sql.ErrNoRows):
		return Order{}, false, err
	}

	snapshot, err := cart.Load(ctx, tx, in.CustomerID)
	if err != nil {
		return Order{}, false, err
	}
	if len(snapshot.Items) == 0 {
		return Order{}, false, ErrCartEmpty
	}
	for _, item := range snapshot.Items {
		if !item.Available {
			return Order{}, false, &CartChangedError{Reason: ReasonItemUnavailable, Cart: snapshot}
		}
	}
	if snapshot.Pricing.TotalCents != in.ExpectedTotalCents {
		return Order{}, false, &CartChangedError{Reason: ReasonTotalChanged, Cart: snapshot}
	}

	var discountCode sql.NullString
	var discountBP sql.NullInt64
	if snapshot.Discount != nil && snapshot.Discount.Applied {
		discountCode = sql.NullString{String: snapshot.Discount.Code, Valid: true}
		discountBP = sql.NullInt64{Int64: snapshot.Discount.PercentBP, Valid: true}
	}
	shipping, err := json.Marshal(shippingSnapshot{Address: in.Address, Method: "DE_STANDARD",
		FlatCents: pricing.DefaultShipping.FlatCents, FreeFromCents: pricing.DefaultShipping.FreeFromCents})
	if err != nil {
		return Order{}, false, err
	}
	taxes, err := json.Marshal(snapshot.Pricing.Taxes)
	if err != nil {
		return Order{}, false, err
	}

	var orderID, orderNumber string
	p := snapshot.Pricing
	if err := tx.QueryRowContext(ctx, `INSERT INTO orders (order_number, customer_id, status,
			total_cents, currency, shipping_snapshot, idempotency_key, request_hash, customer_email,
			goods_cents, discount_cents, shipping_cents, discount_code, discount_percent_bp, tax_lines)
		VALUES ('NW-' || to_char(now() AT TIME ZONE 'Europe/Berlin', 'YYYY') || '-' || nextval('order_number_seq'),
			$1, $2, $3, 'EUR', $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, order_number`,
		in.CustomerID, Created, p.TotalCents, shipping, in.IdempotencyKey, hash, email,
		p.GoodsCents, p.DiscountCents, p.ShippingCents, discountCode, discountBP, taxes).
		Scan(&orderID, &orderNumber); err != nil {
		return Order{}, false, err
	}
	for index, item := range snapshot.Items {
		tax, err := json.Marshal(taxSnapshot{TaxClass: item.TaxClass, RateBP: item.TaxRateBP})
		if err != nil {
			return Order{}, false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_items (order_id, variant_id, position,
				name_snapshot, variant_label_snapshot, sku_snapshot, quantity, unit_price_cents, currency,
				line_total_cents, tax_snapshot)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'EUR', $9, $10)`,
			orderID, item.VariantID, index+1, item.ProductName, item.Label, item.SKU, item.Quantity,
			item.UnitPriceCents, item.LineTotalCents, tax); err != nil {
			return Order{}, false, err
		}
	}
	lines := make([]inventory.Line, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		lines = append(lines, inventory.Line{VariantID: item.VariantID, Quantity: item.Quantity})
	}
	if err := inventory.Reserve(ctx, tx, orderID, lines, time.Now().Add(inventory.DefaultTTL)); err != nil {
		return Order{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO order_status_events (order_id, from_status, to_status, actor)
		VALUES ($1, NULL, $2, 'customer')`, orderID, Created); err != nil {
		return Order{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_items WHERE customer_id = $1`, in.CustomerID); err != nil {
		return Order{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_discounts WHERE customer_id = $1`, in.CustomerID); err != nil {
		return Order{}, false, err
	}
	order, err = get(ctx, tx, in.CustomerID, orderNumber)
	if err != nil {
		return Order{}, false, err
	}
	return order, false, nil
}

// Get liefert eine Bestellung nur ihrem eigenen Kunden; fremde Bestellungen ergeben ErrNotFound.
func (r *Repository) Get(ctx context.Context, customerID, orderNumber string) (Order, error) {
	return get(ctx, r.db, customerID, orderNumber)
}

// Cancel gibt eine noch offene Reservierung frei; derselbe Abbruch darf wiederholt werden.
func (r *Repository) Cancel(ctx context.Context, customerID, orderNumber string) (Order, error) {
	order, err := r.Get(ctx, customerID, orderNumber)
	if err != nil {
		return Order{}, err
	}
	if err := inventory.Cancel(ctx, r.db, order.ID); err != nil {
		return Order{}, err
	}
	return r.Get(ctx, customerID, orderNumber)
}

func get(ctx context.Context, q cart.Querier, customerID, orderNumber string) (Order, error) {
	var order Order
	var shipping, taxes []byte
	var discountCode sql.NullString
	var discountBP sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT id, order_number, status, customer_email, shipping_snapshot,
			goods_cents, discount_cents, shipping_cents, total_cents, currency, tax_lines,
			discount_code, discount_percent_bp, created_at
		FROM orders WHERE customer_id = $1 AND order_number = $2`, customerID, orderNumber).
		Scan(&order.ID, &order.OrderNumber, &order.Status, &order.CustomerEmail, &shipping,
			&order.Pricing.GoodsCents, &order.Pricing.DiscountCents, &order.Pricing.ShippingCents,
			&order.Pricing.TotalCents, &order.Pricing.Currency, &taxes, &discountCode, &discountBP,
			&order.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	var ship shippingSnapshot
	if err := json.Unmarshal(shipping, &ship); err != nil {
		return Order{}, err
	}
	order.Address = ship.Address
	if err := json.Unmarshal(taxes, &order.Pricing.Taxes); err != nil {
		return Order{}, err
	}
	if discountCode.Valid {
		order.Discount = &Discount{Code: discountCode.String, PercentBP: discountBP.Int64}
	}
	var shipment Shipment
	err = q.QueryRowContext(ctx, `SELECT carrier, tracking_number, created_at FROM order_shipments WHERE order_id = $1`, order.ID).
		Scan(&shipment.Carrier, &shipment.TrackingNumber, &shipment.CreatedAt)
	if err == nil {
		order.Shipment = &shipment
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Order{}, err
	}
	events, err := q.QueryContext(ctx, `SELECT from_status, to_status, created_at FROM order_status_events
		WHERE order_id = $1 ORDER BY id`, order.ID)
	if err != nil {
		return Order{}, err
	}
	order.StatusEvents = []StatusEvent{}
	for events.Next() {
		var event StatusEvent
		var from sql.NullString
		if err := events.Scan(&from, &event.ToStatus, &event.CreatedAt); err != nil {
			events.Close()
			return Order{}, err
		}
		if from.Valid {
			status := Status(from.String)
			event.FromStatus = &status
		}
		order.StatusEvents = append(order.StatusEvents, event)
	}
	if err := events.Err(); err != nil {
		events.Close()
		return Order{}, err
	}
	events.Close()
	rows, err := q.QueryContext(ctx, `SELECT position, variant_id, name_snapshot, variant_label_snapshot,
			sku_snapshot, quantity, unit_price_cents, line_total_cents, tax_snapshot
		FROM order_items WHERE order_id = $1 ORDER BY position`, order.ID)
	if err != nil {
		return Order{}, err
	}
	defer rows.Close()
	order.Items = []Item{}
	for rows.Next() {
		var item Item
		var tax []byte
		if err := rows.Scan(&item.Position, &item.VariantID, &item.ProductName, &item.VariantLabel,
			&item.SKU, &item.Quantity, &item.UnitPriceCents, &item.LineTotalCents, &tax); err != nil {
			return Order{}, err
		}
		var snapshot taxSnapshot
		if err := json.Unmarshal(tax, &snapshot); err != nil {
			return Order{}, err
		}
		item.TaxClass, item.TaxRateBP = snapshot.TaxClass, snapshot.RateBP
		order.Items = append(order.Items, item)
	}
	return order, rows.Err()
}
