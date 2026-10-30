package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/pricing"
)

const orderAddress = `{"first_name":" Erika ","last_name":"Musterfrau","company":"","street":"Heidestraße",
	"house_number":"17","address_extra":"","postal_code":" 51147 ","city":"Köln","country":"de"}`

func orderRequest(router http.Handler, body string, cookie *http.Cookie, csrf, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
	request.Host = "shop.example.invalid"
	request.Header.Set("Origin", "https://shop.example.invalid")
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func orderBody(total int64) string {
	return fmt.Sprintf(`{"address":%s,"expected_total_cents":%d,"preview_notice_acknowledged":true}`, orderAddress, total)
}

type orderProblem struct {
	Code   string              `json:"code"`
	Reason string              `json:"reason"`
	Errors []orders.FieldError `json:"errors"`
	Cart   *cart.Snapshot      `json:"cart"`
}

func decodeOrderProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) orderProblem {
	t.Helper()
	assertProblem(t, response, status, code)
	var problem orderProblem
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	return problem
}

func decodeOrder(t *testing.T, response *httptest.ResponseRecorder, status int) orders.Order {
	t.Helper()
	if response.Code != status {
		t.Fatalf("Bestellung: %d %s", response.Code, response.Body.String())
	}
	var order orders.Order
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	return order
}

// Prüft die transaktionale Bestellanlage (ADR-008) gegen PostgreSQL.
func TestOrdersOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_ORDERS_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_ORDERS_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
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
	for _, table := range []string{"orders", "cart_items", "products", "customers"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("Bestelltest benötigt leere Tabelle %s: %d, %v", table, count, err)
		}
	}
	t.Cleanup(func() {
		// TRUNCATE umgeht die Zeilentrigger, die Bestellungen gegen Löschen schützen.
		if _, err := db.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS test_fail_status_event ON order_status_events;
			DROP FUNCTION IF EXISTS test_fail_status_event();
			TRUNCATE order_status_events, order_items, orders, cart_discounts, cart_items, discount_codes,
				admin_audit_events, product_variants, products, auth_sessions, auth_rate_limits, customers CASCADE`); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
		_ = db.Close()
	})

	products := catalog.NewRepository(db)
	pan, err := products.CreateProduct(ctx, catalog.Product{Name: "Pfanne", Slug: "pfanne", Status: catalog.ProductPublished})
	if err != nil {
		t.Fatal(err)
	}
	book, err := products.CreateProduct(ctx, catalog.Product{Name: "Kochbuch", Slug: "kochbuch", Status: catalog.ProductPublished, TaxClass: pricing.TaxReduced})
	if err != nil {
		t.Fatal(err)
	}
	panVariant, err := products.CreateVariant(ctx, catalog.ProductVariant{ProductID: pan.ID, SKU: "NW-PFANNE", Label: "28 cm", PriceCents: 3000, StockQuantity: 50, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	bookVariant, err := products.CreateVariant(ctx, catalog.ProductVariant{ProductID: book.ID, SKU: "NW-BUCH", Label: "Gebunden", PriceCents: 1000, StockQuantity: 50, IsActive: true})
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
	RegisterCartRoutes(router, auth, cart.NewRepository(db))
	RegisterAdminRoutes(router, auth, catalog.NewAdminRepository(db))
	RegisterOrderRoutes(router, auth, orders.NewRepository(db), true)
	password := "Synthetic-Orders-Passphrase-2026"
	login := func(email string, admin bool) (*http.Cookie, string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		if response := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); response.Code != http.StatusCreated {
			t.Fatalf("Registrierung: %d", response.Code)
		}
		if admin {
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
	customer, csrf := login("bestell-kunde@example.invalid", false)
	other, otherCSRF := login("bestell-andere@example.invalid", false)
	admin, adminCSRF := login("bestell-admin@example.invalid", true)

	getCart := func() cart.Snapshot {
		t.Helper()
		response := authRequest(router, http.MethodGet, "/api/v1/cart", "", customer, "")
		var snapshot cart.Snapshot
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &snapshot) != nil {
			t.Fatalf("Warenkorb: %d %s", response.Code, response.Body.String())
		}
		return snapshot
	}
	fillCart := func() cart.Snapshot {
		t.Helper()
		for _, item := range []struct {
			id       string
			quantity int
		}{{panVariant.ID, 1}, {bookVariant.ID, 1}} {
			response := authRequest(router, http.MethodPost, "/api/v1/cart/items",
				fmt.Sprintf(`{"variant_id":%q,"quantity":%d}`, item.id, item.quantity), customer, csrf)
			if response.Code != http.StatusOK {
				t.Fatalf("In den Warenkorb: %d %s", response.Code, response.Body.String())
			}
		}
		return getCart()
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var value int
		if err := db.QueryRowContext(ctx, query, args...).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	orderCount := func() int { return count(`SELECT count(*) FROM orders`) }
	key := func(suffix string) string { return "test-schluessel-" + suffix }

	t.Run("Produktionssperre verhindert Bestellungen", func(t *testing.T) {
		blockedRouter, err := New("NordWinds", db, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		RegisterOrderRoutes(blockedRouter, auth, orders.NewRepository(db), false)
		assertProblem(t, orderRequest(blockedRouter, orderBody(0), customer, csrf, key("gesperrt")), http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE")
		if orderCount() != 0 {
			t.Fatal("Bestellung trotz Produktionssperre")
		}
	})

	t.Run("Anmeldung, Rolle, CSRF und Schlüssel sind Pflicht", func(t *testing.T) {
		assertProblem(t, orderRequest(router, orderBody(0), nil, "", key("a")), http.StatusUnauthorized, "UNAUTHENTICATED")
		assertProblem(t, orderRequest(router, orderBody(0), customer, "", key("a")), http.StatusForbidden, "FORBIDDEN")
		assertProblem(t, orderRequest(router, orderBody(0), admin, adminCSRF, key("a")), http.StatusForbidden, "FORBIDDEN")
		assertProblem(t, orderRequest(router, orderBody(0), customer, csrf, ""), http.StatusBadRequest, "INVALID_REQUEST")
		assertProblem(t, orderRequest(router, orderBody(0), customer, csrf, "kurz"), http.StatusBadRequest, "INVALID_REQUEST")
		assertProblem(t, orderRequest(router, `{"address":`+orderAddress+`}`, customer, csrf, key("a")), http.StatusBadRequest, "INVALID_REQUEST")
		assertProblem(t, orderRequest(router, `{"address":`+orderAddress+`,"expected_total_cents":-1}`, customer, csrf, key("a")), http.StatusBadRequest, "INVALID_REQUEST")
		assertProblem(t, orderRequest(router, `{"address":`+orderAddress+`,"expected_total_cents":0,"total_cents":0}`, customer, csrf, key("a")), http.StatusBadRequest, "INVALID_REQUEST")
		assertProblem(t, orderRequest(router, `{"address":`+orderAddress+`,"expected_total_cents":0}`, customer, csrf, key("hinweis")), http.StatusBadRequest, "INVALID_REQUEST")
		if orderCount() != 0 {
			t.Fatal("Bestellung trotz abgewiesener Anfrage")
		}
	})

	t.Run("Leerer Warenkorb", func(t *testing.T) {
		decodeOrderProblem(t, orderRequest(router, orderBody(0), customer, csrf, key("leer")), http.StatusConflict, "CART_EMPTY")
		if orderCount() != 0 {
			t.Fatal("Bestellung aus leerem Warenkorb")
		}
	})

	t.Run("Fehlende oder ungültige Adresse", func(t *testing.T) {
		snapshot := fillCart()
		problem := decodeOrderProblem(t, orderRequest(router, fmt.Sprintf(`{"expected_total_cents":%d,"preview_notice_acknowledged":true}`, snapshot.Pricing.TotalCents),
			customer, csrf, key("adresse")), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
		if len(problem.Errors) != 7 {
			t.Fatalf("Feldfehler ohne Adresse: %+v", problem.Errors)
		}
		partial := `{"address":{"first_name":"Erika","last_name":"","street":"Heidestraße","house_number":"17",
			"postal_code":"5114","city":"Köln","country":"AT"},"expected_total_cents":` + fmt.Sprint(snapshot.Pricing.TotalCents) + `,"preview_notice_acknowledged":true}`
		problem = decodeOrderProblem(t, orderRequest(router, partial, customer, csrf, key("adresse")), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
		want := []orders.FieldError{{Field: "last_name", Code: "required"}, {Field: "postal_code", Code: "invalid_format"},
			{Field: "country", Code: "unsupported_country"}}
		if len(problem.Errors) != len(want) || problem.Errors[0] != want[0] || problem.Errors[1] != want[1] || problem.Errors[2] != want[2] {
			t.Fatalf("Feldfehler: %+v", problem.Errors)
		}
		if strings.Contains(orderRequest(router, partial, customer, csrf, key("adresse")).Body.String(), "Heidestraße") {
			t.Fatal("Problemantwort wiederholt Eingabewerte")
		}
		if orderCount() != 0 || len(getCart().Items) != 2 {
			t.Fatal("Adressfehler hat Bestellung oder Warenkorb verändert")
		}
	})

	var first orders.Order
	t.Run("Preisänderung verlangt neue Bestätigung", func(t *testing.T) {
		shown := getCart().Pricing.TotalCents
		if shown != 4490 {
			t.Fatalf("angezeigte Summe %d", shown)
		}
		if _, err := db.ExecContext(ctx, `UPDATE product_variants SET price_cents = 3200 WHERE id = $1`, panVariant.ID); err != nil {
			t.Fatal(err)
		}
		problem := decodeOrderProblem(t, orderRequest(router, orderBody(shown), customer, csrf, key("erste")), http.StatusConflict, "CART_CHANGED")
		if problem.Reason != orders.ReasonTotalChanged || problem.Cart == nil || problem.Cart.Pricing.TotalCents != 4690 {
			t.Fatalf("Konflikt ohne neue Summe: %+v", problem)
		}
		if orderCount() != 0 || len(getCart().Items) != 2 {
			t.Fatal("Preisänderung hat Bestellung oder Warenkorb verändert")
		}
		// Der Schlüssel ist durch den Konflikt nicht verbraucht.
		first = decodeOrder(t, orderRequest(router, orderBody(4690), customer, csrf, key("erste")), http.StatusCreated)
	})

	t.Run("Bestellung speichert Positionen, Preise, Adresse und Status", func(t *testing.T) {
		if !orders.ValidOrderNumber(first.OrderNumber) || !strings.HasPrefix(first.OrderNumber, "NW-20") || first.Status != orders.Created ||
			first.CustomerEmail != "bestell-kunde@example.invalid" {
			t.Fatalf("Kopf: %+v", first)
		}
		if first.Address.FirstName != "Erika" || first.Address.PostalCode != "51147" || first.Address.Country != "DE" {
			t.Fatalf("Adresse nicht normalisiert: %+v", first.Address)
		}
		if len(first.Items) != 2 || first.Items[0] != (orders.Item{Position: 1, VariantID: panVariant.ID, ProductName: "Pfanne",
			VariantLabel: "28 cm", SKU: "NW-PFANNE", Quantity: 1, UnitPriceCents: 3200, LineTotalCents: 3200, TaxClass: "standard", TaxRateBP: 1900}) ||
			first.Items[1].TaxRateBP != 700 || first.Items[1].Position != 2 {
			t.Fatalf("Positionen: %+v", first.Items)
		}
		p := first.Pricing
		if p.GoodsCents != 4200 || p.DiscountCents != 0 || p.ShippingCents != 490 || p.TotalCents != 4690 || len(p.Taxes) != 2 ||
			p.Taxes[0].GrossCents+p.Taxes[1].GrossCents != 4690 || first.Discount != nil {
			t.Fatalf("Preisaufstellung: %+v", p)
		}
		if count(`SELECT count(*) FROM order_items WHERE order_id = $1`, first.ID) != 2 ||
			count(`SELECT count(*) FROM order_status_events WHERE order_id = $1 AND from_status IS NULL AND to_status = 'CREATED' AND actor = 'customer'`, first.ID) != 1 {
			t.Fatal("Positionen oder Statuseintrag fehlen")
		}
		if len(getCart().Items) != 0 {
			t.Fatal("Warenkorb nach der Bestellung nicht leer")
		}
	})

	t.Run("Wiederholung mit demselben Schlüssel legt keine zweite Bestellung an", func(t *testing.T) {
		response := orderRequest(router, orderBody(4690), customer, csrf, key("erste"))
		replay := decodeOrder(t, response, http.StatusOK)
		if replay.OrderNumber != first.OrderNumber || response.Header().Get("Idempotent-Replayed") != "true" ||
			response.Header().Get("Location") != "/api/v1/orders/"+first.OrderNumber {
			t.Fatalf("Wiederholung: %+v %v", replay, response.Header())
		}
		changed := strings.Replace(orderBody(4690), "Köln", "Bonn", 1)
		decodeOrderProblem(t, orderRequest(router, changed, customer, csrf, key("erste")), http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED")
		if orderCount() != 1 {
			t.Fatalf("%d Bestellungen statt 1", orderCount())
		}
	})

	t.Run("Gleichzeitige Doppelklicks ergeben genau eine Bestellung", func(t *testing.T) {
		total := fillCart().Pricing.TotalCents
		statuses := make(chan int, 12)
		numbers := make(chan string, 12)
		var wait sync.WaitGroup
		for index := 0; index < 12; index++ {
			wait.Add(1)
			go func(index int) {
				defer wait.Done()
				requestKey := key("doppelklick")
				if index >= 8 {
					requestKey = key(fmt.Sprintf("anderer-tab-%d", index))
				}
				response := orderRequest(router, orderBody(total), customer, csrf, requestKey)
				statuses <- response.Code
				if response.Code < 300 {
					var order orders.Order
					_ = json.Unmarshal(response.Body.Bytes(), &order)
					numbers <- order.OrderNumber
				}
			}(index)
		}
		wait.Wait()
		close(statuses)
		close(numbers)
		created, replayed, empty := 0, 0, 0
		for status := range statuses {
			switch status {
			case http.StatusCreated:
				created++
			case http.StatusOK:
				replayed++
			case http.StatusConflict:
				empty++
			default:
				t.Errorf("unerwarteter Status %d", status)
			}
		}
		distinct := map[string]bool{}
		for number := range numbers {
			distinct[number] = true
		}
		// Ein Aufruf legt an; alle mit demselben Schlüssel erhalten diese Bestellung, die
		// mit eigenem Schlüssel finden einen leeren Warenkorb – es sei denn, einer von ihnen war zuerst.
		if created != 1 || len(distinct) != 1 || created+replayed+empty != 12 || orderCount() != 2 {
			t.Fatalf("angelegt %d, wiederholt %d, leer %d, Nummern %v, Bestellungen %d", created, replayed, empty, distinct, orderCount())
		}
	})

	t.Run("Rabatt wird mitgespeichert; deaktivierter Rabatt verlangt Bestätigung", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `INSERT INTO discount_codes (code, percent_bp) VALUES ('BESTELL10', 1000)`); err != nil {
			t.Fatal(err)
		}
		fillCart()
		if response := authRequest(router, http.MethodPut, "/api/v1/cart/discount", `{"code":"bestell10"}`, customer, csrf); response.Code != http.StatusOK {
			t.Fatalf("Rabatt: %d", response.Code)
		}
		shown := getCart().Pricing.TotalCents
		if _, err := db.ExecContext(ctx, `UPDATE discount_codes SET is_active = false`); err != nil {
			t.Fatal(err)
		}
		problem := decodeOrderProblem(t, orderRequest(router, orderBody(shown), customer, csrf, key("rabatt")), http.StatusConflict, "CART_CHANGED")
		if problem.Reason != orders.ReasonTotalChanged || problem.Cart.Discount == nil || problem.Cart.Discount.Applied {
			t.Fatalf("deaktivierter Rabatt: %+v", problem)
		}
		if _, err := db.ExecContext(ctx, `UPDATE discount_codes SET is_active = true`); err != nil {
			t.Fatal(err)
		}
		order := decodeOrder(t, orderRequest(router, orderBody(shown), customer, csrf, key("rabatt")), http.StatusCreated)
		if order.Discount == nil || order.Discount.Code != "BESTELL10" || order.Discount.PercentBP != 1000 ||
			order.Pricing.DiscountCents != 420 || order.Pricing.TotalCents != shown {
			t.Fatalf("Rabatt-Snapshot: %+v %+v", order.Discount, order.Pricing)
		}
		if count(`SELECT count(*) FROM cart_discounts`) != 0 {
			t.Fatal("Rabattcode bleibt nach der Bestellung im Warenkorb")
		}
	})

	t.Run("Nicht lieferbare Position verhindert die Bestellung", func(t *testing.T) {
		total := fillCart().Pricing.TotalCents
		if _, err := db.ExecContext(ctx, `UPDATE product_variants SET stock_quantity = 0 WHERE id = $1`, bookVariant.ID); err != nil {
			t.Fatal(err)
		}
		before := orderCount()
		problem := decodeOrderProblem(t, orderRequest(router, orderBody(total), customer, csrf, key("ausverkauft")), http.StatusConflict, "CART_CHANGED")
		if problem.Reason != orders.ReasonItemUnavailable || orderCount() != before {
			t.Fatalf("ausverkauft: %+v", problem)
		}
		if _, err := db.ExecContext(ctx, `UPDATE product_variants SET stock_quantity = 50 WHERE id = $1`, bookVariant.ID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Fehler in der Transaktion hinterlässt keine Teildaten", func(t *testing.T) {
		total := getCart().Pricing.TotalCents
		before := []int{orderCount(), count(`SELECT count(*) FROM order_items`), count(`SELECT count(*) FROM order_status_events`)}
		if _, err := db.ExecContext(ctx, `CREATE FUNCTION test_fail_status_event() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'Testfehler nach Bestellung und Positionen'; END $$;
			CREATE TRIGGER test_fail_status_event BEFORE INSERT ON order_status_events
			FOR EACH ROW EXECUTE FUNCTION test_fail_status_event()`); err != nil {
			t.Fatal(err)
		}
		assertProblem(t, orderRequest(router, orderBody(total), customer, csrf, key("rollback")), http.StatusInternalServerError, "INTERNAL_ERROR")
		after := []int{orderCount(), count(`SELECT count(*) FROM order_items`), count(`SELECT count(*) FROM order_status_events`)}
		if after[0] != before[0] || after[1] != before[1] || after[2] != before[2] || len(getCart().Items) != 2 {
			t.Fatalf("Teildaten nach Rollback: vorher %v, nachher %v", before, after)
		}
		if _, err := db.ExecContext(ctx, `DROP TRIGGER test_fail_status_event ON order_status_events;
			DROP FUNCTION test_fail_status_event()`); err != nil {
			t.Fatal(err)
		}
		decodeOrder(t, orderRequest(router, orderBody(total), customer, csrf, key("rollback")), http.StatusCreated)
	})

	t.Run("Nur der eigene Kunde sieht die Bestellung", func(t *testing.T) {
		path := "/api/v1/orders/" + first.OrderNumber
		if own := decodeOrder(t, authRequest(router, http.MethodGet, path, "", customer, ""), http.StatusOK); own.ID != first.ID || len(own.Items) != 2 {
			t.Fatalf("eigene Bestellung: %+v", own)
		}
		assertProblem(t, authRequest(router, http.MethodGet, path, "", other, ""), http.StatusNotFound, "NOT_FOUND")
		assertProblem(t, authRequest(router, http.MethodGet, path, "", nil, ""), http.StatusUnauthorized, "UNAUTHENTICATED")
		assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/orders/'%20OR%201=1", "", customer, ""), http.StatusNotFound, "NOT_FOUND")
	})

	t.Run("Snapshots und Status sind in der Datenbank geschützt", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `UPDATE product_variants SET price_cents = 9900 WHERE id = $1`, panVariant.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE products SET name = 'Neue Pfanne' WHERE id = $1`, pan.ID); err != nil {
			t.Fatal(err)
		}
		again := decodeOrder(t, authRequest(router, http.MethodGet, "/api/v1/orders/"+first.OrderNumber, "", customer, ""), http.StatusOK)
		if again.Items[0].UnitPriceCents != 3200 || again.Items[0].ProductName != "Pfanne" || again.Pricing.TotalCents != 4690 {
			t.Fatalf("Katalogänderung wirkt auf Bestellung: %+v", again.Items[0])
		}
		sqlState := func(query string, args ...any) string {
			t.Helper()
			_, err := db.ExecContext(ctx, query, args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("%s: kein Datenbankfehler: %v", query, err)
			}
			return pgErr.Code
		}
		for query, want := range map[string]string{
			`UPDATE order_items SET unit_price_cents = 1 WHERE order_id = $1`:                          "23000",
			`DELETE FROM order_items WHERE order_id = $1`:                                              "23000",
			`UPDATE orders SET total_cents = 1, goods_cents = 511, shipping_cents = 490 WHERE id = $1`: "23000",
			`UPDATE orders SET shipping_snapshot = '{"address":{}}' WHERE id = $1`:                     "23000",
			`DELETE FROM orders WHERE id = $1`:                                                         "23000",
			`UPDATE orders SET status = 'SHIPPED' WHERE id = $1`:                                       "23514",
			`UPDATE order_status_events SET to_status = 'PAID' WHERE order_id = $1`:                    "23000",
		} {
			if got := sqlState(query, first.ID); got != want {
				t.Errorf("%s: SQLSTATE %s, erwartet %s", query, got, want)
			}
		}
		if _, err := db.ExecContext(ctx, `UPDATE orders SET status = 'PAYMENT_PENDING', updated_at = now() WHERE id = $1`, first.ID); err != nil {
			t.Fatalf("erlaubter Statuswechsel: %v", err)
		}
		if got := sqlState(`UPDATE orders SET status = 'CREATED' WHERE id = $1`, first.ID); got != "23514" {
			t.Fatalf("Rückwärtswechsel: %s", got)
		}
		assertProblem(t, authRequest(router, http.MethodDelete, "/api/v1/admin/variants/"+panVariant.ID, "", admin, adminCSRF), http.StatusConflict, "CONFLICT")
	})

	t.Run("Zwei Kunden reservieren das letzte Stück nur einmal", func(t *testing.T) {
		last, err := products.CreateProduct(ctx, catalog.Product{
			Name: "Letztes Stück", Slug: "letztes-stueck", Status: catalog.ProductPublished,
		})
		if err != nil {
			t.Fatal(err)
		}
		variant, err := products.CreateVariant(ctx, catalog.ProductVariant{
			ProductID: last.ID, SKU: "NW-LAST-ONE", Label: "Einzelstück",
			PriceCents: 1000, StockQuantity: 1, IsActive: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		users := []struct {
			cookie *http.Cookie
			csrf   string
		}{
			{customer, csrf}, {other, otherCSRF},
		}
		for _, user := range users {
			response := authRequest(router, http.MethodPost, "/api/v1/cart/items",
				fmt.Sprintf(`{"variant_id":%q,"quantity":1}`, variant.ID), user.cookie, user.csrf)
			if response.Code != http.StatusOK {
				t.Fatalf("Warenkorb vor Wettlauf: %d %s", response.Code, response.Body.String())
			}
		}
		before := orderCount()
		results := make([]*httptest.ResponseRecorder, 2)
		start := make(chan struct{})
		var concurrent sync.WaitGroup
		for index, user := range users {
			concurrent.Add(1)
			go func(index int, user struct {
				cookie *http.Cookie
				csrf   string
			}) {
				defer concurrent.Done()
				<-start
				results[index] = orderRequest(router, orderBody(1490), user.cookie, user.csrf,
					key(fmt.Sprintf("letztes-%d", index)))
			}(index, user)
		}
		close(start)
		concurrent.Wait()
		winner, created := -1, 0
		for index, response := range results {
			switch response.Code {
			case http.StatusCreated:
				winner = index
				created++
			case http.StatusConflict:
				decodeOrderProblem(t, response, http.StatusConflict, "CART_CHANGED")
			default:
				t.Fatalf("Unerwartetes Ergebnis %d: %s", response.Code, response.Body.String())
			}
		}
		if created != 1 || winner < 0 || orderCount() != before+1 ||
			count(`SELECT stock_quantity FROM product_variants WHERE id = $1`, variant.ID) != 0 ||
			count(`SELECT count(*) FROM order_reservations WHERE variant_id = $1 AND state = 'active'`, variant.ID) != 1 {
			t.Fatalf("Überverkauf: Ergebnisse %d/%d, Bestellungen %d", results[0].Code, results[1].Code, orderCount()-before)
		}
		var winnerOrder orders.Order
		if err := json.Unmarshal(results[winner].Body.Bytes(), &winnerOrder); err != nil {
			t.Fatal(err)
		}
		cancelPath := "/api/v1/orders/" + winnerOrder.OrderNumber + "/cancel"
		assertProblem(t, authRequest(router, http.MethodPost, cancelPath, "", users[1-winner].cookie,
			users[1-winner].csrf), http.StatusNotFound, "NOT_FOUND")
		for attempt := 0; attempt < 2; attempt++ {
			response := authRequest(router, http.MethodPost, cancelPath, "", users[winner].cookie,
				users[winner].csrf)
			if response.Code != http.StatusOK {
				t.Fatalf("Stornierung %d: %d %s", attempt, response.Code, response.Body.String())
			}
		}
		if count(`SELECT stock_quantity FROM product_variants WHERE id = $1`, variant.ID) != 1 ||
			count(`SELECT count(*) FROM order_reservations WHERE variant_id = $1 AND state = 'released'`, variant.ID) != 1 ||
			count(`SELECT count(*) FROM order_status_events WHERE order_id = $1 AND to_status = 'CANCELLED'`, winnerOrder.ID) != 1 {
			t.Fatal("Stornierung gab Bestand nicht genau einmal frei")
		}
		loser := 1 - winner
		retry := orderRequest(router, orderBody(1490), users[loser].cookie, users[loser].csrf,
			key(fmt.Sprintf("letztes-%d", loser)))
		decodeOrder(t, retry, http.StatusCreated)
		if count(`SELECT stock_quantity FROM product_variants WHERE id = $1`, variant.ID) != 0 ||
			count(`SELECT count(*) FROM order_reservations WHERE variant_id = $1 AND state = 'active'`, variant.ID) != 1 {
			t.Fatal("Wiederholung nach Stornierung reservierte Bestand nicht erneut")
		}
	})
}
