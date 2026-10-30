package catalog

import (
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

// Category gehört zum hierarchischen, öffentlich filterbaren Katalog.
type Category struct {
	ID          string         `json:"id"`
	ParentID    *string        `json:"parent_id"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Description string         `json:"description"`
	Status      CategoryStatus `json:"status"`
	SortOrder   int32          `json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type CategoryStatus string

const (
	CategoryActive   CategoryStatus = "active"
	CategoryInactive CategoryStatus = "inactive"
)

// Product ist die redaktionelle Einheit; Verkaufspreis und Bestand liegen an der Variante.
type Product struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Slug        string        `json:"slug"`
	Description string        `json:"description"`
	Status      ProductStatus `json:"status"`
	// TaxClass bestimmt den Umsatzsteuersatz aller Varianten (ADR-007).
	TaxClass  pricing.TaxClass `json:"tax_class"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type ProductStatus string

const (
	ProductDraft     ProductStatus = "draft"
	ProductPublished ProductStatus = "published"
	ProductArchived  ProductStatus = "archived"
)

type ProductCategory struct {
	ProductID  string
	CategoryID string
}

// ProductVariant ist die verkaufbare SKU mit ganzzahligem Cent-Betrag.
type ProductVariant struct {
	ID            string    `json:"id"`
	ProductID     string    `json:"product_id"`
	SKU           string    `json:"sku"`
	Label         string    `json:"label"`
	PriceCents    int64     `json:"price_cents"`
	Currency      string    `json:"currency"`
	StockQuantity int32     `json:"stock_quantity"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
