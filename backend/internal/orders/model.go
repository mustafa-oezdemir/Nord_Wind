package orders

import (
	"encoding/json"
	"time"
)

type Status string

const (
	Created        Status = "CREATED"
	PaymentPending Status = "PAYMENT_PENDING"
	Paid           Status = "PAID"
	Processing     Status = "PROCESSING"
	Shipped        Status = "SHIPPED"
	Delivered      Status = "DELIVERED"
	Cancelled      Status = "CANCELLED"
	PaymentFailed  Status = "PAYMENT_FAILED"
)

// Order enthält den Cent-Gesamtbetrag und den Versand-Snapshot des Kaufzeitpunkts.
type Order struct {
	ID               string
	OrderNumber      string
	CustomerID       string
	Status           Status
	TotalCents       int64
	Currency         string
	ShippingSnapshot json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// OrderItem bewahrt SKU, Name, Stückpreis und Steuerdaten unabhängig vom Katalog.
type OrderItem struct {
	ID             string
	OrderID        string
	VariantID      string
	NameSnapshot   string
	SKUSnapshot    string
	Quantity       int32
	UnitPriceCents int64
	Currency       string
	TaxSnapshot    json.RawMessage
	CreatedAt      time.Time
}
