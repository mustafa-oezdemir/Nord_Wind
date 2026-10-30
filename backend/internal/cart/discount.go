package cart

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

// MaxDiscountAttempts begrenzt fehlgeschlagene Codeeingaben je Kunde und 15 Minuten.
const MaxDiscountAttempts = 10

var (
	// ErrDiscountInvalid fasst unbekannte, deaktivierte und abgelaufene Codes zusammen,
	// damit die API nicht verrät, welche Codes existieren.
	ErrDiscountInvalid = errors.New("Rabattcode ungültig")
	ErrRateLimited     = errors.New("zu viele Rabattcode-Versuche")
)

var discountCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{2,31}$`)

// NormalizeDiscountCode entfernt Leerraum und schreibt Codes groß.
func NormalizeDiscountCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// ValidDiscountCode prüft das gespeicherte Codeformat.
func ValidDiscountCode(code string) bool { return discountCodePattern.MatchString(code) }

// Discount ist der im Warenkorb hinterlegte Code.
type Discount struct {
	Code      string `json:"code"`
	PercentBP int64  `json:"percent_bp"`
	// Applied ist false, wenn der Code inzwischen deaktiviert oder abgelaufen ist.
	Applied bool `json:"applied"`
}

// TaxLine ist eine Steuerzeile der Preisaufstellung.
type TaxLine struct {
	TaxClass   string `json:"tax_class"`
	RateBP     int64  `json:"rate_bp"`
	GrossCents int64  `json:"gross_cents"`
	NetCents   int64  `json:"net_cents"`
	VATCents   int64  `json:"vat_cents"`
}

// Pricing ist die serverseitige Preisaufstellung; die Oberfläche rechnet nicht selbst.
type Pricing struct {
	GoodsCents                  int64     `json:"goods_cents"`
	DiscountCents               int64     `json:"discount_cents"`
	ShippingCents               int64     `json:"shipping_cents"`
	TotalCents                  int64     `json:"total_cents"`
	MissingForFreeShippingCents int64     `json:"missing_for_free_shipping_cents"`
	ShippingFlatCents           int64     `json:"shipping_flat_cents"`
	FreeShippingFromCents       int64     `json:"free_shipping_from_cents"`
	Currency                    string    `json:"currency"`
	Taxes                       []TaxLine `json:"taxes"`
}

func newPricing(quote pricing.Quote, policy pricing.ShippingPolicy) Pricing {
	result := Pricing{
		GoodsCents: quote.GoodsCents, DiscountCents: quote.DiscountCents,
		ShippingCents: quote.ShippingCents, TotalCents: quote.TotalCents,
		MissingForFreeShippingCents: quote.MissingForFreeShippingCents,
		ShippingFlatCents:           policy.FlatCents, FreeShippingFromCents: policy.FreeFromCents,
		Currency: "EUR", Taxes: make([]TaxLine, 0, len(quote.Taxes)),
	}
	for _, tax := range quote.Taxes {
		result.Taxes = append(result.Taxes, TaxLine{TaxClass: string(tax.Class), RateBP: tax.RateBP,
			GrossCents: tax.GrossCents, NetCents: tax.NetCents, VATCents: tax.VATCents})
	}
	return result
}

func (r *Repository) storedDiscount(ctx context.Context, customerID string) (*Discount, error) {
	var discount Discount
	err := r.db.QueryRowContext(ctx, `SELECT dc.code, dc.percent_bp,
		dc.is_active AND (dc.valid_from IS NULL OR dc.valid_from <= now())
			AND (dc.valid_until IS NULL OR dc.valid_until > now())
		FROM cart_discounts cd JOIN discount_codes dc ON dc.id = cd.discount_code_id
		WHERE cd.customer_id = $1`, customerID).Scan(&discount.Code, &discount.PercentBP, &discount.Applied)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &discount, nil
}

// ApplyDiscount hinterlegt einen gültigen Code im Warenkorb des Kunden und ersetzt einen
// vorhandenen. Fehlversuche werden gezählt; nach MaxDiscountAttempts folgt ErrRateLimited.
func (r *Repository) ApplyDiscount(ctx context.Context, customerID, rawCode string) error {
	key := discountAttemptKey(customerID)
	var attempts int
	err := r.db.QueryRowContext(ctx, `SELECT attempts FROM auth_rate_limits
		WHERE key_hash = $1 AND window_started_at > now() - interval '15 minutes'`, key).Scan(&attempts)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if attempts >= MaxDiscountAttempts {
		return ErrRateLimited
	}
	code := NormalizeDiscountCode(rawCode)
	var codeID string
	if ValidDiscountCode(code) {
		err = r.db.QueryRowContext(ctx, `SELECT id FROM discount_codes
			WHERE code = $1 AND is_active AND (valid_from IS NULL OR valid_from <= now())
				AND (valid_until IS NULL OR valid_until > now())`, code).Scan(&codeID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	if codeID == "" {
		if err := r.recordDiscountFailure(ctx, key); err != nil {
			return err
		}
		return ErrDiscountInvalid
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO cart_discounts (customer_id, discount_code_id)
		VALUES ($1, $2) ON CONFLICT (customer_id) DO UPDATE
		SET discount_code_id = EXCLUDED.discount_code_id, applied_at = now()`, customerID, codeID)
	return err
}

// RemoveDiscount entfernt einen hinterlegten Code; ohne Code ist der Aufruf wirkungslos.
func (r *Repository) RemoveDiscount(ctx context.Context, customerID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM cart_discounts WHERE customer_id = $1`, customerID)
	return err
}

// Gleiches Zeitfenster und gleiche Tabelle wie die Anmeldebegrenzung (NW-11), eigener Schlüssel.
func (r *Repository) recordDiscountFailure(ctx context.Context, key []byte) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO auth_rate_limits (key_hash, window_started_at, attempts)
		VALUES ($1, now(), 1)
		ON CONFLICT (key_hash) DO UPDATE SET
		attempts = CASE WHEN auth_rate_limits.window_started_at <= now() - interval '15 minutes'
			THEN 1 ELSE LEAST(auth_rate_limits.attempts + 1, 1000000) END,
		window_started_at = CASE WHEN auth_rate_limits.window_started_at <= now() - interval '15 minutes'
			THEN now() ELSE auth_rate_limits.window_started_at END`, key)
	return err
}

func discountAttemptKey(customerID string) []byte {
	sum := sha256.Sum256([]byte("discount-code:" + customerID))
	return sum[:]
}

// DiscountCode ist die Verwaltungssicht eines Rabattcodes.
type DiscountCode struct {
	ID         string     `json:"id"`
	Code       string     `json:"code"`
	PercentBP  int64      `json:"percent_bp"`
	IsActive   bool       `json:"is_active"`
	ValidFrom  *time.Time `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// DiscountAdmin pflegt Rabattcodes; jede Änderung erzeugt atomar ein Audit-Ereignis.
type DiscountAdmin struct{ db *sql.DB }

func NewDiscountAdmin(db *sql.DB) *DiscountAdmin { return &DiscountAdmin{db: db} }

func (a *DiscountAdmin) List(ctx context.Context) ([]DiscountCode, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id, code, percent_bp, is_active, valid_from,
		valid_until, created_at, updated_at FROM discount_codes ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DiscountCode{}
	for rows.Next() {
		item, err := scanDiscountCode(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (a *DiscountAdmin) Create(ctx context.Context, actorID string, input DiscountCode) (DiscountCode, error) {
	return a.write(ctx, actorID, "create", func(tx *sql.Tx) (DiscountCode, error) {
		return scanDiscountCode(tx.QueryRowContext(ctx, `INSERT INTO discount_codes
			(code, percent_bp, is_active, valid_from, valid_until) VALUES ($1, $2, $3, $4, $5)
			RETURNING id, code, percent_bp, is_active, valid_from, valid_until, created_at, updated_at`,
			input.Code, input.PercentBP, input.IsActive, input.ValidFrom, input.ValidUntil))
	})
}

// Update ändert Prozentsatz, Aktivierung und Gültigkeit; der Code selbst bleibt unverändert.
func (a *DiscountAdmin) Update(ctx context.Context, actorID string, input DiscountCode) (DiscountCode, error) {
	return a.write(ctx, actorID, "update", func(tx *sql.Tx) (DiscountCode, error) {
		return scanDiscountCode(tx.QueryRowContext(ctx, `UPDATE discount_codes SET percent_bp = $2,
			is_active = $3, valid_from = $4, valid_until = $5, updated_at = now() WHERE id = $1
			RETURNING id, code, percent_bp, is_active, valid_from, valid_until, created_at, updated_at`,
			input.ID, input.PercentBP, input.IsActive, input.ValidFrom, input.ValidUntil))
	})
}

func (a *DiscountAdmin) write(ctx context.Context, actorID, action string, change func(*sql.Tx) (DiscountCode, error)) (DiscountCode, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return DiscountCode{}, err
	}
	defer tx.Rollback()
	result, err := change(tx)
	if errors.Is(err, sql.ErrNoRows) {
		return DiscountCode{}, ErrNotFound
	}
	if err != nil {
		return DiscountCode{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin_audit_events
		(actor_customer_id, entity_type, entity_id, action) VALUES ($1, 'discount_code', $2, $3)`,
		actorID, result.ID, action); err != nil {
		return DiscountCode{}, err
	}
	return result, tx.Commit()
}

type scanner interface{ Scan(...any) error }

func scanDiscountCode(row scanner) (DiscountCode, error) {
	var item DiscountCode
	var from, until sql.NullTime
	err := row.Scan(&item.ID, &item.Code, &item.PercentBP, &item.IsActive, &from, &until,
		&item.CreatedAt, &item.UpdatedAt)
	if from.Valid {
		item.ValidFrom = &from.Time
	}
	if until.Valid {
		item.ValidUntil = &until.Time
	}
	return item, err
}
