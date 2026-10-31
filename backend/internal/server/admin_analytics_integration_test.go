package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	// Unterkategorie ohne Umsatz, Produkt darin und Produkt ohne Kategorie: Alle erscheinen als eigene Reihe.
	subcategory := insertShippingID(t, db, `INSERT INTO categories (name, slug, parent_id) VALUES ('Pfannen', 'analytics-pfannen', $1) RETURNING id`, categoryA)
	grill := insertShippingID(t, db, `INSERT INTO products (name, slug, status) VALUES ('Grillpfanne', 'analytics-grillpfanne', 'draft') RETURNING id`)
	if _, err := db.ExecContext(ctx, `INSERT INTO product_categories (product_id, category_id) VALUES ($1, $2)`, grill, subcategory); err != nil {
		t.Fatal(err)
	}
	voucher := insertShippingID(t, db, `INSERT INTO products (name, slug, status) VALUES ('Gutschein', 'analytics-gutschein', 'published') RETURNING id`)

	get := func(query string) *httptest.ResponseRecorder {
		t.Helper()
		return authRequest(router, http.MethodGet, "/api/v1/admin/analytics/sales?"+query, "", admin, "")
	}
	read := func(query string) analytics.Report {
		t.Helper()
		response := get(query)
		if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("Analyse %s: %d %s", query, response.Code, response.Body.String())
		}
		var report analytics.Report
		if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		return report
	}
	type row struct {
		key   string
		total int64
	}
	rows := func(report analytics.Report) []row {
		result := make([]row, 0, len(report.Series))
		for _, series := range report.Series {
			result = append(result, row{series.Key, series.TotalCents})
		}
		return result
	}
	expect := func(name string, report analytics.Report, total int64, want ...row) {
		t.Helper()
		got := rows(report)
		if report.TotalCents != total || fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: Summe %d, Reihen %v; erwartet %d, %v", name, report.TotalCents, got, total, want)
		}
	}

	total := read("group_by=total&period=week")
	if total.TotalCents != 3600 || len(total.Buckets) != 7 || len(total.Series) != 1 || total.Series[0].Quantity != 2 || total.Granularity != "day" {
		t.Fatalf("Rabattbereinigter Gesamtumsatz: %+v", total)
	}
	// Kategorien in Baumreihenfolge (sort_order, Name), Unterkategorie direkt unter ihrer Oberkategorie.
	categories := read("group_by=category&period=quarter")
	expect("Kategorien", categories, 3600, row{categoryB, 900}, row{categoryA, 2700}, row{subcategory, 0}, row{"uncategorized", 0})
	if len(categories.Buckets) != 3 || categories.Series[2].ParentKey != categoryA || categories.Series[2].Depth != 1 {
		t.Fatalf("Unterkategorie: %+v", categories.Series[2])
	}
	expect("Oberkategorie mit Unterkategorie", read("group_by=category&period=week&category_id="+categoryA), 2700,
		row{categoryA, 2700}, row{subcategory, 0})
	expect("nur Unterkategorie", read("group_by=category&period=week&category_id="+subcategory), 0, row{subcategory, 0})

	// Produkte gruppiert nach Kategorie; jede Reihe kennt ihre Kategorie für Farbe und Gruppierung.
	products := read("group_by=product&period=month")
	expect("Produkte", products, 3600, row{productB, 900}, row{productA, 2700}, row{grill, 0}, row{voucher, 0})
	if len(products.Buckets) != 30 || products.Series[1].CategoryKey != categoryA || products.Series[1].CategoryLabel != "Küche" ||
		products.Series[2].CategoryKey != subcategory || products.Series[3].CategoryKey != "uncategorized" {
		t.Fatalf("Produktkategorien: %+v", products.Series)
	}
	expect("Produkte einer Kategorie samt Unterkategorie", read("group_by=product&period=week&category_id="+categoryA), 2700,
		row{productA, 2700}, row{grill, 0})
	expect("ein Produkt", read("group_by=product&period=week&category_id="+categoryA+"&product_id="+productA), 2700, row{productA, 2700})
	expect("Gesamtumsatz einer Kategorie", read("group_by=total&period=week&category_id="+categoryB), 900, row{"total", 900})

	// Eigener Zeitraum: gestern bis heute in Berlin.
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(berlin)
	custom := read("group_by=total&period=custom&from=" + today.AddDate(0, 0, -1).Format("2006-01-02") + "&to=" + today.Format("2006-01-02"))
	if custom.TotalCents != 3600 || len(custom.Buckets) != 2 || custom.StartDate != today.AddDate(0, 0, -1).Format("2006-01-02") {
		t.Fatalf("Eigener Zeitraum: %+v", custom)
	}
	if empty := read("group_by=total&period=custom&from=2025-01-01&to=2025-01-31"); empty.TotalCents != 0 || len(empty.Buckets) != 31 {
		t.Fatalf("Zeitraum ohne Verkäufe: %+v", empty)
	}
	for _, invalid := range []string{
		"group_by=total&period=custom&from=" + today.Format("2006-01-02") + "&to=" + today.AddDate(0, 0, 1).Format("2006-01-02"),
		"group_by=total&period=custom&from=2026-10-31",
		"group_by=total&period=week&from=2026-10-01",
		"group_by=category&period=week&category_id=kein-uuid",
		"group_by=category&period=week&product_id=" + productA,
		"group_by=product&period=week&category_id=" + categoryB + "&product_id=" + productA,
	} {
		assertProblem(t, get(invalid), 400, "INVALID_REQUEST")
	}
	assertProblem(t, get("group_by=category&period=week&category_id=00000000-0000-4000-8000-000000000000"), 404, "NOT_FOUND")
	assertProblem(t, get("group_by=product&period=week&product_id=00000000-0000-4000-8000-000000000000"), 404, "NOT_FOUND")

	// Auswahllisten: alle Kategorien und Produkte der Datenbank, nur für Admins.
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/analytics/catalog", "", employee, ""), 403, "FORBIDDEN")
	catalogResponse := authRequest(router, http.MethodGet, "/api/v1/admin/analytics/catalog", "", admin, "")
	var catalog analytics.Catalog
	if catalogResponse.Code != 200 || json.Unmarshal(catalogResponse.Body.Bytes(), &catalog) != nil {
		t.Fatalf("Katalog: %d %s", catalogResponse.Code, catalogResponse.Body.String())
	}
	if len(catalog.Categories) != 3 || catalog.Categories[0].ID != categoryB || catalog.Categories[2].ID != subcategory ||
		catalog.Categories[2].Depth != 1 || len(catalog.Products) != 4 || catalog.Products[2].ID != grill ||
		catalog.Products[2].CategoryID != subcategory || catalog.Products[2].Status != "draft" || catalog.Products[3].CategoryID != "" {
		t.Fatalf("Katalog: %+v", catalog)
	}

	if _, err := db.ExecContext(ctx, `UPDATE mock_payments SET state = 'refunded' WHERE order_id = $1`, orderID); err != nil {
		t.Fatal(err)
	}
	if refunded := read("group_by=total&period=week"); refunded.TotalCents != 0 {
		t.Fatalf("Erstattung darf nicht als Umsatz zählen: %+v", refunded)
	}
}
