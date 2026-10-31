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
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/analytics"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestAdminSalesAnalyticsOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_ANALYTICS_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_ANALYTICS_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Analyse-Test verweigert Datenbank %q", name)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM customers`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Analyse-Test benötigt eine leere Datenbank: %d, %v", count, err)
	}
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), `TRUNCATE notification_outbox, mock_payment_events,
			mock_payments, order_status_events, order_items, orders, product_categories,
			product_variants, products, categories, auth_sessions, auth_rate_limits, customers CASCADE`)
		if err != nil {
			t.Errorf("Analyse-Testdaten bereinigen: %v", err)
		}
		_ = db.Close()
	})
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	RegisterAdminAnalyticsRoutes(router, auth, analytics.NewRepository(db))
	const password = "Synthetic-Analytics-Passphrase-2026"
	for _, email := range []string{"sales-admin@example.invalid", "sales-employee@example.invalid", "sales-customer@example.invalid"} {
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		if response := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); response.Code != 201 {
			t.Fatalf("Registrierung %s: %d %s", email, response.Code, response.Body.String())
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE email = 'sales-admin@example.invalid'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'employee' WHERE email = 'sales-employee@example.invalid'`); err != nil {
		t.Fatal(err)
	}
	login := func(email string) *http.Cookie {
		t.Helper()
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login",
			fmt.Sprintf(`{"email":%q,"password":%q}`, email, password), nil, "")
		if response.Code != 200 {
			t.Fatalf("Anmeldung %s: %d", email, response.Code)
		}
		return sessionCookie(t, response)
	}
	admin := login("sales-admin@example.invalid")
	employee := login("sales-employee@example.invalid")
	customer := login("sales-customer@example.invalid")
	const path = "/api/v1/admin/analytics/sales?group_by=category&period=week"
	if response := authRequest(router, http.MethodGet, path, "", nil, ""); response.Code != 401 {
		t.Fatalf("Anonyme Analyse: %d", response.Code)
	}
	assertProblem(t, authRequest(router, http.MethodGet, path, "", customer, ""), 403, "FORBIDDEN")
	assertProblem(t, authRequest(router, http.MethodGet, path, "", employee, ""), 403, "FORBIDDEN")
	assertProblem(t, authRequest(router, http.MethodGet,
		"/api/v1/admin/analytics/sales?group_by=invalid&period=week", "", admin, ""), 400, "INVALID_REQUEST")

	categoryA := insertShippingID(t, db, `INSERT INTO categories (name, slug) VALUES ('Küche', 'analytics-kueche') RETURNING id`)
	categoryB := insertShippingID(t, db, `INSERT INTO categories (name, slug) VALUES ('Bücher', 'analytics-buecher') RETURNING id`)
	productA := insertShippingID(t, db, `INSERT INTO products (name, slug, status) VALUES ('Pfanne', 'analytics-pfanne', 'published') RETURNING id`)
	productB := insertShippingID(t, db, `INSERT INTO products (name, slug, status) VALUES ('Buch', 'analytics-buch', 'published') RETURNING id`)
	for _, pair := range [][2]string{{productA, categoryA}, {productB, categoryB}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO product_categories (product_id, category_id) VALUES ($1, $2)`, pair[0], pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	variantA := insertShippingID(t, db, `INSERT INTO product_variants (product_id, sku, label, price_cents) VALUES ($1, 'NW-AN-PFANNE', 'Standard', 3000) RETURNING id`, productA)
	variantB := insertShippingID(t, db, `INSERT INTO product_variants (product_id, sku, label, price_cents) VALUES ($1, 'NW-AN-BUCH', 'Standard', 1000) RETURNING id`, productB)
	var customerID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM customers WHERE email = 'sales-customer@example.invalid'`).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	orderID := insertShippingID(t, db, `INSERT INTO orders (order_number, customer_id, status, total_cents,
		shipping_snapshot, idempotency_key, request_hash, customer_email, goods_cents,
		discount_cents, shipping_cents, tax_lines)
		VALUES ('NW-2026-990001', $1, 'PAID', 4090, '{"address":{}}'::jsonb,
		'analytics-key-0001', decode(repeat('ab', 32), 'hex'), 'sales-customer@example.invalid',
		4000, 400, 490, '[{"tax_class":"standard","rate_bp":1900}]'::jsonb)
		RETURNING id`, customerID)
	for index, line := range []struct {
		variant, name, sku string
		price              int
	}{{variantA, "Pfanne", "NW-AN-PFANNE", 3000}, {variantB, "Buch", "NW-AN-BUCH", 1000}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO order_items (order_id, variant_id, position,
			name_snapshot, variant_label_snapshot, sku_snapshot, quantity, unit_price_cents,
			line_total_cents, tax_snapshot)
			VALUES ($1, $2, $3, $4, 'Standard', $5, 1, $6, $6,
			'{"tax_class":"standard","rate_bp":1900}'::jsonb)`,
			orderID, line.variant, index+1, line.name, line.sku, line.price); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO order_status_events (order_id, to_status, actor)
		VALUES ($1, 'PAID', 'system')`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO mock_payments (id, order_id, method, state, amount_cents)
		VALUES ('pay_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', $1, 'paypal', 'succeeded', 4090)`, orderID); err != nil {
		t.Fatal(err)
	}

	read := func(group, period string) analytics.Report {
		t.Helper()
		response := authRequest(router, http.MethodGet,
			"/api/v1/admin/analytics/sales?group_by="+group+"&period="+period, "", admin, "")
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Analyse %s/%s: %d %s", group, period, response.Code, response.Body.String())
		}
		var report analytics.Report
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	total := read("total", "week")
	if total.TotalCents != 3600 || len(total.Buckets) != 7 || len(total.Series) != 1 || total.Series[0].Quantity != 2 {
		t.Fatalf("Rabattbereinigter Gesamtumsatz: %+v", total)
	}
	productSales := read("product", "month")
	if productSales.TotalCents != 3600 || len(productSales.Buckets) != 30 ||
		len(productSales.Series) != 2 || productSales.Series[0].TotalCents != 2700 || productSales.Series[1].TotalCents != 900 {
		t.Fatalf("Produktumsatz: %+v", productSales)
	}
	categorySales := read("category", "quarter")
	if categorySales.TotalCents != 3600 || len(categorySales.Buckets) != 3 ||
		len(categorySales.Series) != 2 || categorySales.Series[0].TotalCents != 2700 || categorySales.Series[1].TotalCents != 900 {
		t.Fatalf("Kategorieumsatz: %+v", categorySales)
	}
	if _, err := db.ExecContext(ctx, `UPDATE mock_payments SET state = 'refunded' WHERE order_id = $1`, orderID); err != nil {
		t.Fatal(err)
	}
	if refunded := read("total", "week"); refunded.TotalCents != 0 {
		t.Fatalf("Erstattung darf nicht als Umsatz zählen: %+v", refunded)
	}
}
