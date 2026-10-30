package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

func TestShippingAndAccountOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_SHIPPING_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_SHIPPING_TEST_URL für eine separate Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"orders", "customer_addresses", "customers"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("Leere Testtabelle %s erforderlich: %d, %v", table, count, err)
		}
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
	repository := orders.NewRepository(db)
	RegisterOrderRoutes(router, auth, repository)
	var shipmentLog bytes.Buffer
	RegisterAdminOrderRoutes(router, auth, repository, slog.New(slog.NewJSONHandler(&shipmentLog, nil)))
	RegisterAccountRoutes(router, auth, repository)
	login := func(email string, admin bool) (*http.Cookie, string, string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":"Synthetic-Shipping-Passphrase-2026"}`, email)
		if response := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); response.Code != 201 {
			t.Fatalf("Registrierung: %s", response.Body.String())
		}
		var id string
		if err := db.QueryRowContext(ctx, `SELECT id FROM customers WHERE email = $1`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if admin {
			if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login", body, nil, "")
		if response.Code != 200 {
			t.Fatalf("Login: %s", response.Body.String())
		}
		return sessionCookie(t, response), csrfFromResponse(t, response), id
	}
	owner, ownerCSRF, ownerID := login("shipping-owner@example.invalid", false)
	other, otherCSRF, otherID := login("shipping-other@example.invalid", false)
	admin, adminCSRF, _ := login("shipping-admin@example.invalid", true)
	productID := insertShippingID(t, db, `INSERT INTO products (name, slug, status) VALUES ('Testware', 'shipping-testware', 'published') RETURNING id`)
	variantID := insertShippingID(t, db, `INSERT INTO product_variants (product_id, sku, label, price_cents, stock_quantity)
		VALUES ($1, 'NW-SHIPPING', 'Standard', 1000, 10) RETURNING id`, productID)
	createOrder := func(id, email, number, key string) string {
		t.Helper()
		orderID := insertShippingID(t, db, `INSERT INTO orders (order_number, customer_id, status, total_cents, shipping_snapshot,
			idempotency_key, request_hash, customer_email, goods_cents, discount_cents, shipping_cents, tax_lines)
			VALUES ($1, $2, 'PAID', 1490, $3, $4, decode(repeat('ab', 32), 'hex'), $5, 1000, 0, 490,
			'[{"tax_class":"standard","rate_bp":1900,"gross_cents":1490,"net_cents":1252,"vat_cents":238}]'::jsonb)
			RETURNING id`, number, id, `{"address":`+orderAddress+`}`, key, email)
		if _, err := db.ExecContext(ctx, `INSERT INTO order_items (order_id, variant_id, position, name_snapshot,
			variant_label_snapshot, sku_snapshot, quantity, unit_price_cents, line_total_cents, tax_snapshot)
			VALUES ($1, $2, 1, 'Testware', 'Standard', 'NW-SHIPPING', 1, 1000, 1000,
			'{"tax_class":"standard","rate_bp":1900}'::jsonb)`, orderID, variantID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO order_status_events (order_id, to_status, actor) VALUES ($1, 'PAID', 'system')`, orderID); err != nil {
			t.Fatal(err)
		}
		return orderID
	}
	number := "NW-2026-900001"
	orderID := createOrder(ownerID, "shipping-owner@example.invalid", number, "shipping-key-00001")
	otherNumber := "NW-2026-900002"
	createOrder(otherID, "shipping-other@example.invalid", otherNumber, "shipping-key-00002")
	if _, err := db.ExecContext(ctx, `UPDATE product_variants SET price_cents = 2500 WHERE id = $1`, variantID); err != nil {
		t.Fatal(err)
	}
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/orders/"+number, "", other, ""), 404, "NOT_FOUND")
	list := authRequest(router, http.MethodGet, "/api/v1/orders", "", other, "")
	if list.Code != 200 || strings.Contains(list.Body.String(), number) || !strings.Contains(list.Body.String(), otherNumber) ||
		!strings.Contains(list.Body.String(), `"unit_price_cents":1000`) || strings.Contains(list.Body.String(), `"unit_price_cents":2500`) {
		t.Fatalf("IDOR in Historie: %d %s", list.Code, list.Body.String())
	}
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/orders", "", owner, ""), 403, "FORBIDDEN")
	patch := func(cookie *http.Cookie, csrf, status, tracking string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"status":%q,"tracking_number":%q}`, status, tracking)
		return authRequest(router, http.MethodPatch, "/api/v1/admin/orders/"+number+"/status", body, cookie, csrf)
	}
	assertProblem(t, patch(owner, ownerCSRF, "PROCESSING", ""), 403, "FORBIDDEN")
	assertProblem(t, patch(admin, adminCSRF, "SHIPPED", "NW-12345"), 409, "CONFLICT")
	if response := patch(admin, adminCSRF, "PROCESSING", ""); response.Code != 200 {
		t.Fatalf("Bearbeitung: %s", response.Body.String())
	}
	assertProblem(t, patch(admin, adminCSRF, "SHIPPED", "bad!"), 422, "VALIDATION_FAILED")
	shipped := patch(admin, adminCSRF, "SHIPPED", "NW-TRACK-12345")
	if shipped.Code != 200 || !strings.Contains(shipped.Body.String(), "NW-TRACK-12345") {
		t.Fatalf("Versand: %d %s", shipped.Code, shipped.Body.String())
	}
	assertProblem(t, patch(admin, adminCSRF, "SHIPPED", "NW-OTHER"), 409, "CONFLICT")
	metrics := authRequest(router, http.MethodGet, "/api/v1/admin/orders/shipment-metrics", "", admin, "")
	if metrics.Code != 200 || !strings.Contains(metrics.Body.String(), `"invalid_tracking":1`) || !strings.Contains(metrics.Body.String(), `"invalid_state":2`) || !strings.Contains(metrics.Body.String(), `"success":1`) {
		t.Fatalf("Versandmetriken: %d %s", metrics.Code, metrics.Body.String())
	}
	if !strings.Contains(shipmentLog.String(), `"event":"shipment.failed"`) ||
		strings.Contains(shipmentLog.String(), "NW-TRACK-12345") || strings.Contains(shipmentLog.String(), "shipping-owner@example.invalid") {
		t.Fatalf("Versandfehlerlog fehlt oder enthält Kundendaten: %s", shipmentLog.String())
	}
	ownerView := authRequest(router, http.MethodGet, "/api/v1/orders/"+number, "", owner, "")
	if ownerView.Code != 200 || !strings.Contains(ownerView.Body.String(), "NW-TRACK-12345") {
		t.Fatalf("Tracking: %s", ownerView.Body.String())
	}
	addressBody := `{"label":"Zuhause","address":` + orderAddress + `}`
	address := authRequest(router, http.MethodPost, "/api/v1/customer/addresses", addressBody, owner, ownerCSRF)
	if address.Code != 201 {
		t.Fatalf("Adresse: %d %s", address.Code, address.Body.String())
	}
	var saved orders.SavedAddress
	if err := json.Unmarshal(address.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/customer/addresses/"+saved.ID, addressBody, other, otherCSRF), 404, "NOT_FOUND")
	otherAddresses := authRequest(router, http.MethodGet, "/api/v1/customer/addresses", "", other, "")
	if otherAddresses.Code != 200 || strings.Contains(otherAddresses.Body.String(), saved.ID) {
		t.Fatalf("Adress-IDOR: %s", otherAddresses.Body.String())
	}
	export := authRequest(router, http.MethodGet, "/api/v1/customer/export", "", owner, "")
	if export.Code != 200 || !strings.Contains(export.Body.String(), number) || strings.Contains(export.Body.String(), otherNumber) {
		t.Fatalf("Export-IDOR: %s", export.Body.String())
	}
	assertProblem(t, authRequest(router, http.MethodDelete, "/api/v1/customer/me", "", owner, ownerCSRF), 409, "CONFLICT")
	if response := patch(admin, adminCSRF, "DELIVERED", ""); response.Code != 200 {
		t.Fatalf("Zustellung: %s", response.Body.String())
	}
	closed := authRequest(router, http.MethodDelete, "/api/v1/customer/me", "", owner, ownerCSRF)
	if closed.Code != 204 {
		t.Fatalf("Kontoschließung: %d %s", closed.Code, closed.Body.String())
	}
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/orders/"+number, "", owner, ""), 401, "UNAUTHENTICATED")
	var retainedEmail, accountEmail string
	if err := db.QueryRowContext(ctx, `SELECT o.customer_email, c.email FROM orders o JOIN customers c ON c.id = o.customer_id WHERE o.id = $1`, orderID).Scan(&retainedEmail, &accountEmail); err != nil {
		t.Fatal(err)
	}
	if retainedEmail != "shipping-owner@example.invalid" || !strings.HasPrefix(accountEmail, "closed-") {
		t.Fatalf("Snapshot/Anonymisierung: %q %q", retainedEmail, accountEmail)
	}
	if _, err := repository.SaveAddress(ctx, ownerID, "", "Spät", orders.Address{
		FirstName: "Erika", LastName: "Musterfrau", Street: "Heidestraße", HouseNumber: "17",
		PostalCode: "51147", City: "Köln", Country: "DE",
	}); !errors.Is(err, orders.ErrNotFound) {
		t.Fatalf("Geschlossenes Konto darf keine neue Adresse erhalten: %v", err)
	}
}

func insertShippingID(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var id string
	if err := db.QueryRow(query, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
