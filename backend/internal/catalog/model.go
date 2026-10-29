package catalog

import "time"

// Category gehört zum hierarchischen, öffentlich filterbaren Katalog.
type Category struct {
	ID          string
	ParentID    *string
	Name        string
	Slug        string
	Description string
	Status      CategoryStatus
	SortOrder   int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CategoryStatus string

const (
	CategoryActive   CategoryStatus = "active"
	CategoryInactive CategoryStatus = "inactive"
)

// Product ist die redaktionelle Einheit; Verkaufspreis und Bestand liegen an der Variante.
type Product struct {
	ID          string
	Name        string
	Slug        string
	Description string
	Status      ProductStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	ID            string
	ProductID     string
	SKU           string
	Label         string
	PriceCents    int64
	Currency      string
	StockQuantity int32
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
