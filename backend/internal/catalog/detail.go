package catalog

import (
	"context"
	"strings"
)

// StockStatus beschreibt den Bestand öffentlich, ohne exakte Lagermengen preiszugeben.
type StockStatus string

const (
	StockInStock    StockStatus = "in_stock"
	StockLow        StockStatus = "low_stock"
	StockOutOfStock StockStatus = "out_of_stock"
)

// LowStockThreshold ist die Menge, bis zu der eine Variante als knapp gilt.
const LowStockThreshold = 5

// SaleStatus fasst zusammen, ob ein Produkt derzeit verkauft wird.
type SaleStatus string

const (
	// SaleAvailable: mindestens eine aktive Variante hat Bestand.
	SaleAvailable SaleStatus = "available"
	// SaleSoldOut: aktive Varianten existieren, aber keine hat Bestand.
	SaleSoldOut SaleStatus = "sold_out"
	// SaleUnavailable: veröffentlicht, aber ohne aktive Variante.
	SaleUnavailable SaleStatus = "unavailable"
	// SaleDiscontinued: archiviert; Seite bleibt für bestehende Links erreichbar.
	SaleDiscontinued SaleStatus = "discontinued"
)

// CategoryRef ist die öffentliche Kurzform einer Kategorie für Brotkrumen.
type CategoryRef struct {
	ID   string
	Slug string
	Name string
}

// VariantOffer ist eine aktive, verkaufbare Variante mit serverseitigem Preis.
// Varianten sind nach Preis und Bezeichnung sortiert, damit die Reihenfolge stabil ist.
type VariantOffer struct {
	ID          string
	SKU         string
	Label       string
	PriceCents  int64
	Currency    string
	StockStatus StockStatus
}

// ProductDetail enthält nur öffentlich darstellbare Produktdaten.
type ProductDetail struct {
	ID          string
	Slug        string
	Name        string
	Description string
	SaleStatus  SaleStatus
	Categories  []CategoryRef
	Variants    []VariantOffer
}

// GetPublicProduct lädt ein veröffentlichtes oder archiviertes Produkt über seinen Slug.
// Entwürfe bleiben unsichtbar und ergeben ErrNotFound. Archivierte Produkte liefern keine
// Varianten, damit nichts mehr bestellt werden kann.
func (r *Repository) GetPublicProduct(ctx context.Context, slug string) (ProductDetail, error) {
	detail := ProductDetail{Categories: []CategoryRef{}, Variants: []VariantOffer{}}
	var status ProductStatus
	err := r.db.QueryRowContext(ctx, `SELECT id, slug, name, description, status FROM products
		WHERE lower(slug) = lower($1) AND status IN ('published', 'archived')`,
		strings.TrimSpace(slug),
	).Scan(&detail.ID, &detail.Slug, &detail.Name, &detail.Description, &status)
	if err != nil {
		return ProductDetail{}, notFound(err)
	}
	if detail.Categories, err = r.publicProductCategories(ctx, detail.ID); err != nil {
		return ProductDetail{}, err
	}
	if status == ProductPublished {
		if detail.Variants, err = r.variantOffers(ctx, detail.ID); err != nil {
			return ProductDetail{}, err
		}
	}
	detail.SaleStatus = saleStatus(status, detail.Variants)
	return detail, nil
}

func (r *Repository) publicProductCategories(ctx context.Context, productID string) ([]CategoryRef, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.id, c.slug, c.name
		FROM categories c JOIN product_categories pc ON pc.category_id = c.id
		WHERE pc.product_id = $1 AND c.status = 'active'
		ORDER BY c.sort_order, c.name, c.id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := []CategoryRef{}
	for rows.Next() {
		var category CategoryRef
		if err := rows.Scan(&category.ID, &category.Slug, &category.Name); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

func (r *Repository) variantOffers(ctx context.Context, productID string) ([]VariantOffer, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, sku, label, price_cents, currency, stock_quantity
		FROM product_variants WHERE product_id = $1 AND is_active
		ORDER BY price_cents, label, id`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variants := []VariantOffer{}
	for rows.Next() {
		var variant VariantOffer
		var stock int32
		if err := rows.Scan(&variant.ID, &variant.SKU, &variant.Label, &variant.PriceCents,
			&variant.Currency, &stock); err != nil {
			return nil, err
		}
		variant.StockStatus = stockStatus(stock)
		variants = append(variants, variant)
	}
	return variants, rows.Err()
}

func stockStatus(quantity int32) StockStatus {
	switch {
	case quantity <= 0:
		return StockOutOfStock
	case quantity <= LowStockThreshold:
		return StockLow
	default:
		return StockInStock
	}
}

func saleStatus(status ProductStatus, variants []VariantOffer) SaleStatus {
	if status != ProductPublished {
		return SaleDiscontinued
	}
	if len(variants) == 0 {
		return SaleUnavailable
	}
	for _, variant := range variants {
		if variant.StockStatus != StockOutOfStock {
			return SaleAvailable
		}
	}
	return SaleSoldOut
}
