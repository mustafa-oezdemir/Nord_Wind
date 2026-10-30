package cart

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

const MaxQuantity = 99
const MaxLines = 50

var ErrNotFound = errors.New("Warenkorbposition nicht gefunden")
var ErrVariantNotFound = errors.New("Variante nicht gefunden")
var ErrUnavailable = errors.New("Variante oder Bestand nicht verfügbar")
var ErrCartLimit = errors.New("Warenkorbgrenze erreicht")

type Item struct {
	VariantID      string `json:"variant_id"`
	ProductID      string `json:"product_id"`
	ProductSlug    string `json:"product_slug"`
	ProductName    string `json:"product_name"`
	SKU            string `json:"sku"`
	Label          string `json:"label"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	LineTotalCents int64  `json:"line_total_cents"`
	Currency       string `json:"currency"`
	Available      bool   `json:"available"`
	Unavailable    string `json:"unavailable_reason"`
	TaxClass       string `json:"tax_class"`
	TaxRateBP      int64  `json:"tax_rate_bp"`
}

// Snapshot ist der Warenkorb mit serverseitig berechneter Preisaufstellung (ADR-007).
type Snapshot struct {
	Items []Item `json:"items"`
	// SubtotalCents ist der Warenwert der lieferbaren Positionen (= Pricing.GoodsCents).
	SubtotalCents int64     `json:"subtotal_cents"`
	Currency      string    `json:"currency"`
	Discount      *Discount `json:"discount"`
	Pricing       Pricing   `json:"pricing"`
}

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, customerID string) (Snapshot, error) {
	snapshot := Snapshot{Items: []Item{}, Currency: "EUR"}
	lines := []pricing.Line{}
	rows, err := r.db.QueryContext(ctx, `SELECT ci.variant_id, v.product_id, p.slug, p.name,
		v.sku, v.label, ci.quantity, v.price_cents, v.currency, v.stock_quantity,
		v.is_active, p.status, p.tax_class
		FROM cart_items ci JOIN product_variants v ON v.id = ci.variant_id
		JOIN products p ON p.id = v.product_id
		WHERE ci.customer_id = $1 ORDER BY ci.created_at, ci.variant_id`, customerID)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		var stock int32
		var active bool
		var status string
		if err := rows.Scan(&item.VariantID, &item.ProductID, &item.ProductSlug,
			&item.ProductName, &item.SKU, &item.Label, &item.Quantity,
			&item.UnitPriceCents, &item.Currency, &stock, &active, &status, &item.TaxClass); err != nil {
			return snapshot, err
		}
		rate, ok := pricing.RateBP(pricing.TaxClass(item.TaxClass))
		if !ok {
			return snapshot, pricing.ErrInvalidLine
		}
		item.TaxRateBP = rate
		if item.UnitPriceCents > math.MaxInt64/int64(item.Quantity) {
			return snapshot, errors.New("Warenkorbpreis überläuft")
		}
		item.LineTotalCents = item.UnitPriceCents * int64(item.Quantity)
		switch {
		case status != "published" || !active:
			item.Unavailable = "product_unavailable"
		case stock < item.Quantity:
			item.Unavailable = "insufficient_stock"
		default:
			item.Available = true
			lines = append(lines, pricing.Line{UnitGrossCents: item.UnitPriceCents,
				Quantity: int64(item.Quantity), TaxClass: pricing.TaxClass(item.TaxClass)})
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if err := rows.Err(); err != nil {
		return snapshot, err
	}
	rows.Close()
	discount, err := r.storedDiscount(ctx, customerID)
	if err != nil {
		return snapshot, err
	}
	snapshot.Discount = discount
	var discountBP int64
	if discount != nil && discount.Applied {
		discountBP = discount.PercentBP
	}
	quote, err := pricing.Calculate(lines, discountBP, pricing.DefaultShipping)
	if err != nil {
		return snapshot, err
	}
	snapshot.Pricing = newPricing(quote, pricing.DefaultShipping)
	snapshot.SubtotalCents = quote.GoodsCents
	return snapshot, nil
}

func (r *Repository) Add(ctx context.Context, customerID, variantID string, quantity int32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedCustomer string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM customers WHERE id = $1 FOR UPDATE`, customerID).Scan(&lockedCustomer); err != nil {
		return err
	}
	var existing bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cart_items WHERE customer_id = $1 AND variant_id = $2)`, customerID, variantID).Scan(&existing); err != nil {
		return err
	}
	if !existing {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cart_items WHERE customer_id = $1`, customerID).Scan(&count); err != nil {
			return err
		}
		if count >= MaxLines {
			return ErrCartLimit
		}
	}
	stock, err := purchasableStock(ctx, tx, variantID)
	if err != nil {
		return err
	}
	if stock < quantity {
		return ErrUnavailable
	}
	var total int32
	err = tx.QueryRowContext(ctx, `INSERT INTO cart_items (customer_id, variant_id, quantity)
		VALUES ($1, $2, $3)
		ON CONFLICT (customer_id, variant_id) DO UPDATE
		SET quantity = cart_items.quantity + EXCLUDED.quantity, updated_at = now()
		WHERE cart_items.quantity + EXCLUDED.quantity <= $4
			AND cart_items.quantity + EXCLUDED.quantity <= $5
		RETURNING quantity`, customerID, variantID, quantity, MaxQuantity, stock).Scan(&total)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnavailable
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) Set(ctx context.Context, customerID, variantID string, quantity int32) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stock, err := purchasableStock(ctx, tx, variantID)
	if err != nil {
		return err
	}
	if stock < quantity {
		return ErrUnavailable
	}
	var updated int32
	err = tx.QueryRowContext(ctx, `UPDATE cart_items SET quantity = $3, updated_at = now()
		WHERE customer_id = $1 AND variant_id = $2 RETURNING quantity`, customerID, variantID, quantity).Scan(&updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) Remove(ctx context.Context, customerID, variantID string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM cart_items WHERE customer_id = $1 AND variant_id = $2`, customerID, variantID)
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

func purchasableStock(ctx context.Context, tx *sql.Tx, variantID string) (int32, error) {
	var stock int32
	var active bool
	var status string
	err := tx.QueryRowContext(ctx, `SELECT v.stock_quantity, v.is_active, p.status
		FROM product_variants v JOIN products p ON p.id = v.product_id
		WHERE v.id = $1 FOR UPDATE OF v`, variantID).Scan(&stock, &active, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVariantNotFound
	}
	if err != nil {
		return 0, err
	}
	if !active || status != "published" {
		return 0, ErrUnavailable
	}
	return stock, nil
}
