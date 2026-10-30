package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

// Prüft die Preisaufstellung des Warenkorbs (ADR-007) und die Pflege von Rabattcodes gegen PostgreSQL.
func TestCartPricingOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_PRICING_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_PRICING_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"cart_items", "discount_codes", "products", "customers"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("Preistest benötigt leere Tabelle %s: %d, %v", table, count, err)
		}
	}
	t.Cleanup(func() {
		for _, table := range []string{"cart_discounts", "cart_items", "admin_audit_events", "discount_codes",
			"product_variants", "products", "auth_sessions", "auth_rate_limits", "customers"} {
			if _, err := db.ExecContext(context.Background(), "DELETE FROM "+table); err != nil {
				t.Errorf("Cleanup %s: %v", table, err)
			}
		}
		_ = db.Close()
	})

	products := catalog.NewRepository(db)
	pan, err := products.CreateProduct(ctx, catalog.Product{Name: "Pfanne", Slug: "pfanne", Status: catalog.ProductPublished})
	if err != nil || pan.TaxClass != pricing.TaxStandard {
		t.Fatalf("Standard-Steuerklasse: %+v, %v", pan, err)
	}
	book, err := products.CreateProduct(ctx, catalog.Product{Name: "Kochbuch", Slug: "kochbuch", Status: catalog.ProductPublished, TaxClass: pricing.TaxReduced})
	if err != nil {
		t.Fatal(err)
	}
	panVariant, err := products.CreateVariant(ctx, catalog.ProductVariant{ProductID: pan.ID, SKU: "NW-PFANNE", Label: "28 cm", PriceCents: 3000, StockQuantity: 10, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	bookVariant, err := products.CreateVariant(ctx, catalog.ProductVariant{ProductID: book.ID, SKU: "NW-BUCH", Label: "Gebunden", PriceCents: 1000, StockQuantity: 10, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}

	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	RegisterCatalogRoutes(router, products)
	RegisterCartRoutes(router, auth, cart.NewRepository(db))
	RegisterDiscountAdminRoutes(router, auth, cart.NewDiscountAdmin(db))
	RegisterAdminRoutes(router, auth, catalog.NewAdminRepository(db))
	password := "Synthetic-Pricing-Passphrase-2026"
	login := func(email string) (*http.Cookie, string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		if response := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); response.Code != http.StatusCreated {
			t.Fatalf("Registrierung: %d", response.Code)
		}
		if email == "preis-admin@example.invalid" {
			if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE email = $1`, email); err != nil {
				t.Fatal(err)
			}
		}
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login", body, nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("Login: %d", response.Code)
		}
		return sessionCookie(t, response), csrfFromResponse(t, response)
	}
	customer, csrf := login("preis-kunde@example.invalid")
	other, otherCSRF := login("preis-andere@example.invalid")
	admin, adminCSRF := login("preis-admin@example.invalid")
	readCart := func(response *http.Response, body []byte) cart.Snapshot {
		t.Helper()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("Warenkorb: %d %s", response.StatusCode, body)
		}
		var snapshot cart.Snapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	call := func(method, path, body string, cookie *http.Cookie, token string) cart.Snapshot {
		t.Helper()
		recorder := authRequest(router, method, path, body, cookie, token)
		return readCart(recorder.Result(), recorder.Body.Bytes())
	}
	add := func(variantID string, quantity int) cart.Snapshot {
		t.Helper()
		return call(http.MethodPost, "/api/v1/cart/items", fmt.Sprintf(`{"variant_id":%q,"quantity":%d}`, variantID, quantity), customer, csrf)
	}

	t.Run("Produktdetail nennt Steuersatz", func(t *testing.T) {
		recorder := getCatalog(t, router, "/api/v1/products/kochbuch")
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"tax_class":"reduced","tax_rate_bp":700`) {
			t.Fatalf("Steuersatz fehlt im Produktdetail: %d %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("Gemischte Steuersätze mit anteiligem Versand", func(t *testing.T) {
		add(panVariant.ID, 1)
		snapshot := add(bookVariant.ID, 1)
		p := snapshot.Pricing
		if p.GoodsCents != 4000 || p.ShippingCents != 490 || p.TotalCents != 4490 || p.MissingForFreeShippingCents != 1000 ||
			p.ShippingFlatCents != 490 || p.FreeShippingFromCents != 5000 || snapshot.SubtotalCents != 4000 {
			t.Fatalf("Summen: %+v", p)
		}
		want := []cart.TaxLine{
			{TaxClass: "standard", RateBP: 1900, GrossCents: 3368, NetCents: 2830, VATCents: 538},
			{TaxClass: "reduced", RateBP: 700, GrossCents: 1122, NetCents: 1049, VATCents: 73},
		}
		if len(p.Taxes) != 2 || p.Taxes[0] != want[0] || p.Taxes[1] != want[1] {
			t.Fatalf("Steuerzeilen: %+v", p.Taxes)
		}
		if snapshot.Items[0].TaxRateBP != 1900 || snapshot.Items[1].TaxClass != "reduced" {
			t.Fatalf("Steuersatz je Position: %+v", snapshot.Items)
		}
	})

	t.Run("Admins setzen die Steuerklasse eines Produkts", func(t *testing.T) {
		path := "/api/v1/admin/products/" + pan.ID
		assertProblem(t, authRequest(router, http.MethodPut, path, `{"name":"Pfanne","slug":"pfanne","status":"published","tax_class":"luxus"}`, admin, adminCSRF), http.StatusBadRequest, "INVALID_REQUEST")
		updated := authRequest(router, http.MethodPut, path, `{"name":"Pfanne","slug":"pfanne","status":"published","tax_class":"reduced"}`, admin, adminCSRF)
		if updated.Code != http.StatusOK || !strings.Contains(updated.Body.String(), `"tax_class":"reduced"`) {
			t.Fatalf("Steuerklasse setzen: %d %s", updated.Code, updated.Body.String())
		}
		if reduced := call(http.MethodGet, "/api/v1/cart", "", customer, ""); len(reduced.Pricing.Taxes) != 1 || reduced.Pricing.Taxes[0].RateBP != 700 {
			t.Fatalf("Warenkorb nutzt neue Steuerklasse nicht: %+v", reduced.Pricing.Taxes)
		}
		restored := authRequest(router, http.MethodPut, path, `{"name":"Pfanne","slug":"pfanne","status":"published","tax_class":"standard"}`, admin, adminCSRF)
		if restored.Code != http.StatusOK {
			t.Fatalf("Steuerklasse zurücksetzen: %d", restored.Code)
		}
	})

	var codeID string
	t.Run("Rabattcodes pflegen nur Admins mit CSRF", func(t *testing.T) {
		body := `{"code":" herbst10 ","percent_bp":1000}`
		assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/discount-codes", body, customer, csrf), http.StatusForbidden, "FORBIDDEN")
		assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/discount-codes", body, admin, ""), http.StatusForbidden, "FORBIDDEN")
		for _, invalid := range []string{`{"code":"x","percent_bp":1000}`, `{"code":"OK-CODE","percent_bp":0}`,
			`{"code":"OK-CODE","percent_bp":10001}`, `{"code":"ÄRGER","percent_bp":500}`,
			`{"code":"OK-CODE","percent_bp":500,"valid_from":"2026-12-01T00:00:00Z","valid_until":"2026-11-01T00:00:00Z"}`} {
			assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/discount-codes", invalid, admin, adminCSRF), http.StatusBadRequest, "INVALID_REQUEST")
		}
		created := authRequest(router, http.MethodPost, "/api/v1/admin/discount-codes", body, admin, adminCSRF)
		var code cart.DiscountCode
		if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &code) != nil || code.Code != "HERBST10" || !code.IsActive {
			t.Fatalf("Code anlegen: %d %s", created.Code, created.Body.String())
		}
		codeID = code.ID
		assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/discount-codes", body, admin, adminCSRF), http.StatusConflict, "CONFLICT")
		assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/admin/discount-codes/"+codeID, `{"code":"ANDERS","percent_bp":1000}`, admin, adminCSRF), http.StatusBadRequest, "INVALID_REQUEST")
		if listed := authRequest(router, http.MethodGet, "/api/v1/admin/discount-codes", "", admin, ""); listed.Code != http.StatusOK ||
			!strings.Contains(listed.Body.String(), `{"data":[{"id":"`+codeID+`","code":"HERBST10","percent_bp":1000,"is_active":true`) {
			t.Fatalf("Codes auflisten: %d %s", listed.Code, listed.Body.String())
		}
		assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/discount-codes", "", customer, ""), http.StatusForbidden, "FORBIDDEN")
		var audits int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM admin_audit_events WHERE entity_type = 'discount_code' AND entity_id = $1`, codeID).Scan(&audits); err != nil || audits != 1 {
			t.Fatalf("Audit-Ereignis fehlt: %d, %v", audits, err)
		}
	})

	t.Run("Rabatt senkt Summe und Steuer; Code gilt nur für eigenen Warenkorb", func(t *testing.T) {
		add(panVariant.ID, 1) // Pfanne jetzt 2 × 30,00 €: Warenwert 70,00 €
		snapshot := call(http.MethodPut, "/api/v1/cart/discount", `{"code":"herbst10"}`, customer, csrf)
		p := snapshot.Pricing
		if snapshot.Discount == nil || snapshot.Discount.Code != "HERBST10" || !snapshot.Discount.Applied ||
			p.GoodsCents != 7000 || p.DiscountCents != 700 || p.ShippingCents != 0 || p.TotalCents != 6300 {
			t.Fatalf("Rabatt: %+v %+v", snapshot.Discount, p)
		}
		if p.Taxes[0].GrossCents+p.Taxes[1].GrossCents != p.TotalCents {
			t.Fatalf("Steuerzeilen ergeben nicht die Summe: %+v", p.Taxes)
		}
		if untouched := call(http.MethodGet, "/api/v1/cart", "", other, ""); untouched.Discount != nil || untouched.Pricing.DiscountCents != 0 {
			t.Fatalf("Rabatt wirkt auf fremden Warenkorb: %+v", untouched)
		}
		assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/cart/discount", `{"code":"HERBST10"}`, customer, ""), http.StatusForbidden, "FORBIDDEN")
	})

	t.Run("Deaktivierter oder abgelaufener Code wird nicht mehr angewendet", func(t *testing.T) {
		updated := authRequest(router, http.MethodPut, "/api/v1/admin/discount-codes/"+codeID, `{"percent_bp":1000,"is_active":false}`, admin, adminCSRF)
		if updated.Code != http.StatusOK {
			t.Fatalf("Code deaktivieren: %d %s", updated.Code, updated.Body.String())
		}
		snapshot := call(http.MethodGet, "/api/v1/cart", "", customer, "")
		if snapshot.Discount == nil || snapshot.Discount.Applied || snapshot.Pricing.DiscountCents != 0 || snapshot.Pricing.TotalCents != 7000 {
			t.Fatalf("Deaktivierter Code wirkt noch: %+v %+v", snapshot.Discount, snapshot.Pricing)
		}
		assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/cart/discount", `{"code":"HERBST10"}`, customer, csrf), http.StatusNotFound, "NOT_FOUND")
		if _, err := db.ExecContext(ctx, `INSERT INTO discount_codes (code, percent_bp, valid_until) VALUES ('ABGELAUFEN', 2000, now() - interval '1 minute'),
			('ZUKUNFT', 2000, NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE discount_codes SET valid_from = now() + interval '1 day' WHERE code = 'ZUKUNFT'`); err != nil {
			t.Fatal(err)
		}
		for _, code := range []string{"ABGELAUFEN", "ZUKUNFT", "GIBTESNICHT"} {
			assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/cart/discount", fmt.Sprintf(`{"code":%q}`, code), customer, csrf), http.StatusNotFound, "NOT_FOUND")
		}
		removed := call(http.MethodDelete, "/api/v1/cart/discount", "", customer, csrf)
		if removed.Discount != nil || removed.Pricing.TotalCents != 7000 {
			t.Fatalf("Code entfernen: %+v", removed)
		}
	})

	t.Run("Fehlversuche werden begrenzt", func(t *testing.T) {
		for attempt := 0; attempt < cart.MaxDiscountAttempts; attempt++ {
			assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/cart/discount", fmt.Sprintf(`{"code":"RATEN-%d"}`, attempt), other, otherCSRF), http.StatusNotFound, "NOT_FOUND")
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO discount_codes (code, percent_bp) VALUES ('GUELTIG', 500)`); err != nil {
			t.Fatal(err)
		}
		assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/cart/discount", `{"code":"GUELTIG"}`, other, otherCSRF), http.StatusTooManyRequests, "RATE_LIMITED")
		if applied := call(http.MethodPut, "/api/v1/cart/discount", `{"code":"GUELTIG"}`, customer, csrf); applied.Discount == nil || applied.Pricing.DiscountCents != 350 {
			t.Fatalf("Begrenzung darf andere Kunden nicht treffen: %+v", applied.Pricing)
		}
	})

	t.Run("Leerer Warenkorb hat keine Versandkosten", func(t *testing.T) {
		for _, id := range []string{panVariant.ID, bookVariant.ID} {
			if response := authRequest(router, http.MethodDelete, "/api/v1/cart/items/"+id, "", customer, csrf); response.Code != http.StatusNoContent {
				t.Fatalf("Entfernen: %d", response.Code)
			}
		}
		empty := call(http.MethodGet, "/api/v1/cart", "", customer, "")
		if empty.Pricing.TotalCents != 0 || empty.Pricing.ShippingCents != 0 || len(empty.Pricing.Taxes) != 0 || empty.Pricing.Taxes == nil {
			t.Fatalf("Leerer Warenkorb: %+v", empty.Pricing)
		}
	})
}
