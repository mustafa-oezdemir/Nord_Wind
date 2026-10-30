package orders

import (
	"errors"
	"regexp"
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

// transitions sind die erlaubten Statuswechsel nach ADR-008. Der Datenbank-Trigger
// `nordwinds_guard_order` erzwingt dieselbe Liste.
var transitions = map[Status][]Status{
	Created:        {PaymentPending, Cancelled},
	PaymentPending: {Paid, PaymentFailed, Cancelled},
	PaymentFailed:  {PaymentPending, Cancelled},
	Paid:           {Processing},
	Processing:     {Shipped},
	Shipped:        {Delivered},
}

// CanTransition meldet, ob ein Statuswechsel erlaubt ist.
func CanTransition(from, to Status) bool {
	for _, next := range transitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

var (
	ErrNotFound            = errors.New("Bestellung nicht gefunden")
	ErrCartEmpty           = errors.New("Warenkorb ist leer")
	ErrIdempotencyMismatch = errors.New("Idempotency-Key wurde mit einer anderen Anfrage verwendet")
)

// CartChangedError meldet, dass der Warenkorb nicht mehr dem bestätigten Stand entspricht.
// Cart enthält den aktuellen Warenkorb, damit die Oberfläche ihn erneut zur Bestätigung zeigt.
type CartChangedError struct {
	Reason string
	Cart   any
}

func (e *CartChangedError) Error() string { return "Warenkorb geändert: " + e.Reason }

const (
	ReasonTotalChanged    = "total_changed"
	ReasonItemUnavailable = "item_unavailable"
)

var (
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)
	orderNumberPattern    = regexp.MustCompile(`^NW-[0-9]{4}-[0-9]{6,12}$`)
)

// ValidIdempotencyKey prüft den Header `Idempotency-Key` (z. B. eine UUID).
func ValidIdempotencyKey(key string) bool { return idempotencyKeyPattern.MatchString(key) }

// ValidOrderNumber prüft das Format `NW-2026-100001`.
func ValidOrderNumber(number string) bool { return orderNumberPattern.MatchString(number) }

// Item ist eine unveränderliche Bestellposition.
type Item struct {
	Position       int16  `json:"position"`
	VariantID      string `json:"variant_id"`
	ProductName    string `json:"product_name"`
	VariantLabel   string `json:"variant_label"`
	SKU            string `json:"sku"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	LineTotalCents int64  `json:"line_total_cents"`
	TaxClass       string `json:"tax_class"`
	TaxRateBP      int64  `json:"tax_rate_bp"`
}

// TaxLine entspricht der Steuerzeile der Preisaufstellung zum Bestellzeitpunkt.
type TaxLine struct {
	TaxClass   string `json:"tax_class"`
	RateBP     int64  `json:"rate_bp"`
	GrossCents int64  `json:"gross_cents"`
	NetCents   int64  `json:"net_cents"`
	VATCents   int64  `json:"vat_cents"`
}

// Pricing ist die gespeicherte Preisaufstellung der Bestellung.
type Pricing struct {
	GoodsCents    int64     `json:"goods_cents"`
	DiscountCents int64     `json:"discount_cents"`
	ShippingCents int64     `json:"shipping_cents"`
	TotalCents    int64     `json:"total_cents"`
	Currency      string    `json:"currency"`
	Taxes         []TaxLine `json:"taxes"`
}

// Discount ist der beim Kauf angerechnete Rabattcode.
type Discount struct {
	Code      string `json:"code"`
	PercentBP int64  `json:"percent_bp"`
}

// Order ist die Kundensicht einer Bestellung.
type Order struct {
	ID            string        `json:"id"`
	OrderNumber   string        `json:"order_number"`
	Status        Status        `json:"status"`
	CustomerEmail string        `json:"customer_email"`
	Address       Address       `json:"address"`
	Items         []Item        `json:"items"`
	Pricing       Pricing       `json:"pricing"`
	Discount      *Discount     `json:"discount"`
	Shipment      *Shipment     `json:"shipment"`
	StatusEvents  []StatusEvent `json:"status_events"`
	CreatedAt     time.Time     `json:"created_at"`
}

type Shipment struct {
	Carrier        string    `json:"carrier"`
	TrackingNumber string    `json:"tracking_number"`
	CreatedAt      time.Time `json:"created_at"`
}

type StatusEvent struct {
	FromStatus *Status   `json:"from_status"`
	ToStatus   Status    `json:"to_status"`
	CreatedAt  time.Time `json:"created_at"`
}

// shippingSnapshot ist der Inhalt von orders.shipping_snapshot.
type shippingSnapshot struct {
	Address       Address `json:"address"`
	Method        string  `json:"method"`
	FlatCents     int64   `json:"flat_cents"`
	FreeFromCents int64   `json:"free_from_cents"`
}

type taxSnapshot struct {
	TaxClass string `json:"tax_class"`
	RateBP   int64  `json:"rate_bp"`
}
