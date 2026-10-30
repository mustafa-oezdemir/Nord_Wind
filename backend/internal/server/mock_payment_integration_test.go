package server

import (
	"bytes"
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

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/inventory"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/payment"
)

func TestMockPaymentLifecycleOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_MOCK_PAYMENT_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_MOCK_PAYMENT_TEST_URL für separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM orders`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Leere Testdatenbank erforderlich: orders=%d, %v", count, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`TRUNCATE mock_payment_events, mock_payments, order_reservations,
			order_status_events, order_items, orders, product_variants, products,
			auth_sessions, auth_rate_limits, customers CASCADE`); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
	})
	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	secret := bytes.Repeat([]byte("s"), 32)
	repository := payment.NewRepository(db)
	RegisterMockPaymentRoutes(router, auth, repository, secret)
	methodsResponse := authRequest(router, http.MethodGet, "/api/v1/mock-payments/methods", "", nil, "")
	if methodsResponse.Code != http.StatusOK || !bytes.Contains(methodsResponse.Body.Bytes(), []byte(`"bank_transfer"`)) {
		t.Fatalf("Mock-Zahlungsarten: %d %s", methodsResponse.Code, methodsResponse.Body.String())
	}
	password := "Synthetic-Mock-Passphrase-2026"
	login := func(email string) (*http.Cookie, string, string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		registered := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, "")
		if registered.Code != http.StatusCreated {
			t.Fatalf("Registrierung: %d %s", registered.Code, registered.Body.String())
		}
		var account struct {
			Customer struct {
				ID string `json:"id"`
			} `json:"customer"`
		}
		if err := json.Unmarshal(registered.Body.Bytes(), &account); err != nil {
			t.Fatal(err)
		}
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login", body, nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("Login: %d %s", response.Code, response.Body.String())
		}
		return sessionCookie(t, response), csrfFromResponse(t, response), account.Customer.ID
	}
	cookie, csrf, customerID := login("mock-kunde@example.invalid")
	otherCookie, otherCSRF, _ := login("mock-fremd@example.invalid")
	newOrder := func(number, status string) string {
		t.Helper()
		var id string
		if err := db.QueryRowContext(ctx, `INSERT INTO orders
			(order_number, customer_id, status, total_cents, shipping_snapshot,
			idempotency_key, request_hash, customer_email, goods_cents, discount_cents,
			shipping_cents, tax_lines)
			VALUES ($1, $2, $3, 100, '{"address":{}}', $4,
			decode(repeat('ab', 32), 'hex'), 'mock-kunde@example.invalid', 100, 0, 0, '[{}]')
			RETURNING id`, number, customerID, status, number+"-checkout-key").Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO order_status_events
			(order_id, from_status, to_status, actor) VALUES ($1, NULL, $2, 'customer')`, id, status); err != nil {
			t.Fatal(err)
		}
		return id
	}
	var productID, variantID string
	if err := db.QueryRowContext(ctx, `INSERT INTO products (name, slug, status)
		VALUES ('Mock-Ware', 'mock-ware', 'published') RETURNING id`).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO product_variants
		(product_id, sku, label, price_cents, stock_quantity)
		VALUES ($1, 'NW-MOCK', 'Standard', 100, 4) RETURNING id`, productID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	methods := []payment.Method{payment.DebitCard, payment.Klarna, payment.PayPal, payment.BankTransfer}
	payments := make(map[payment.Method]payment.Payment)
	for index, method := range methods {
		orderID := newOrder(fmt.Sprintf("NW-MOCK-%d", index), "PAYMENT_PENDING")
		if err := inventory.WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
			return inventory.Reserve(ctx, tx, orderID, []inventory.Line{{VariantID: variantID, Quantity: 1}}, time.Now().Add(time.Minute))
		}); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf(`{"order_id":%q,"method":%q}`, orderID, method)
		response := authRequest(router, http.MethodPost, "/api/v1/mock-payments", body, cookie, csrf)
		if response.Code != http.StatusOK {
			t.Fatalf("Initiierung %s: %d %s", method, response.Code, response.Body.String())
		}
		var created payment.Payment
		if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.Method != method || created.State != "pending" {
			t.Fatalf("Initiierung %s: %+v, %v", method, created, err)
		}
		payments[method] = created
		if repeated := authRequest(router, http.MethodPost, "/api/v1/mock-payments", body, cookie, csrf); repeated.Code != http.StatusOK || !bytes.Contains(repeated.Body.Bytes(), []byte(created.ID)) {
			t.Fatalf("Doppelte Initiierung %s: %d", method, repeated.Code)
		}
	}
	if response := authRequest(router, http.MethodPost, "/api/v1/mock-payments", fmt.Sprintf(`{"order_id":%q,"method":"paypal"}`, payments[payment.DebitCard].OrderID), cookie, csrf); response.Code != http.StatusConflict {
		t.Fatalf("Methodenwechsel: %d", response.Code)
	}
	if response := authRequest(router, http.MethodGet, "/api/v1/mock-payments/"+payments[payment.DebitCard].ID, "", otherCookie, otherCSRF); response.Code != http.StatusNotFound {
		t.Fatalf("Fremde Zahlung lesbar: %d", response.Code)
	}
	byOrderPath := "/api/v1/mock-payments/by-order/" + payments[payment.DebitCard].OrderID
	if response := authRequest(router, http.MethodGet, byOrderPath, "", cookie, ""); response.Code != http.StatusOK {
		t.Fatalf("Eigene Zahlung über Bestellung: %d", response.Code)
	}
	if response := authRequest(router, http.MethodGet, byOrderPath, "", otherCookie, ""); response.Code != http.StatusNotFound {
		t.Fatalf("Fremde Zahlung über Bestellung: %d", response.Code)
	}
	if response := authRequest(router, http.MethodPost, "/api/v1/mock-payments", fmt.Sprintf(`{"order_id":%q,"method":"paypal"}`, payments[payment.DebitCard].OrderID), otherCookie, otherCSRF); response.Code != http.StatusNotFound {
		t.Fatalf("Fremde Bestellung zahlbar: %d", response.Code)
	}
	badWebhook := httptest.NewRequest(http.MethodPost, "/api/v1/mock-payments/webhook", strings.NewReader(`{}`))
	badWebhook.Header.Set("Content-Type", "application/json")
	badResponse := httptest.NewRecorder()
	router.ServeHTTP(badResponse, badWebhook)
	assertProblem(t, badResponse, http.StatusUnauthorized, "UNAUTHENTICATED")
	simulate := func(method payment.Method, outcome payment.Outcome) (payment.Payment, string) {
		t.Helper()
		response := authRequest(router, http.MethodPost, "/api/v1/mock-payments/"+payments[method].ID+"/simulate",
			fmt.Sprintf(`{"outcome":%q}`, outcome), cookie, csrf)
		if response.Code != http.StatusOK {
			t.Fatalf("Simulation %s/%s: %d %s", method, outcome, response.Code, response.Body.String())
		}
		var result struct {
			EventID string          `json:"event_id"`
			Payment payment.Payment `json:"payment"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Payment, result.EventID
	}
	debit, debitEvent := simulate(payment.DebitCard, payment.Succeeded)
	if debit.State != "succeeded" {
		t.Fatalf("Debit-Erfolg: %+v", debit)
	}
	event := payment.Event{ID: debitEvent, PaymentID: debit.ID, OrderID: debit.OrderID, Method: debit.Method,
		Outcome: payment.Succeeded, AmountCents: debit.AmountCents, Currency: debit.Currency}
	raw, err := payment.MarshalEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/mock-payments/webhook", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Nordwinds-Mock-Signature", payment.Sign(secret, raw, time.Now()))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("Webhook-Wiederholung %d: %d %s", attempt, response.Code, response.Body.String())
		}
	}
	var eventCount int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM mock_payment_events WHERE id = $1`, debitEvent).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("Event nicht idempotent: %d, %v", eventCount, err)
	}
	refunded, _ := simulate(payment.DebitCard, payment.Refunded)
	if refunded.State != "refunded" {
		t.Fatalf("Debit-Erstattung: %+v", refunded)
	}
	failed, _ := simulate(payment.Klarna, payment.Failed)
	if failed.State != "failed" {
		t.Fatalf("Klarna-Fehler: %+v", failed)
	}
	paypal, _ := simulate(payment.PayPal, payment.Succeeded)
	if paypal.State != "succeeded" {
		t.Fatalf("PayPal-Erfolg: %+v", paypal)
	}
	if _, err := db.ExecContext(ctx, `UPDATE order_reservations SET reserved_at = now() - interval '2 minutes',
		expires_at = now() - interval '1 second'
		WHERE order_id = $1`, payments[payment.BankTransfer].OrderID); err != nil {
		t.Fatal(err)
	}
	if released, err := inventory.ReleaseExpired(ctx, db, time.Now(), 10); err != nil || released != 1 {
		t.Fatalf("Überweisung-Timeout: %d, %v", released, err)
	}
	if err := repository.SyncExpired(ctx); err != nil {
		t.Fatal(err)
	}
	transfer := authRequest(router, http.MethodGet, "/api/v1/mock-payments/"+payments[payment.BankTransfer].ID, "", cookie, "")
	if transfer.Code != http.StatusOK || !bytes.Contains(transfer.Body.Bytes(), []byte(`"state":"failed"`)) {
		t.Fatalf("Überweisung nach Timeout: %d %s", transfer.Code, transfer.Body.String())
	}
	var stock int
	if err := db.QueryRowContext(ctx, `SELECT stock_quantity FROM product_variants WHERE id = $1`, variantID).Scan(&stock); err != nil || stock != 2 {
		t.Fatalf("Bestand nach Erfolg, Fehler, Erstattung, Timeout: %d statt 2, %v", stock, err)
	}
	cancelledOrderID := newOrder("NW-MOCK-CANCELLED", "CREATED")
	if err := inventory.WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		return inventory.Reserve(ctx, tx, cancelledOrderID,
			[]inventory.Line{{VariantID: variantID, Quantity: 1}}, time.Now().Add(time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	response := authRequest(router, http.MethodPost, "/api/v1/mock-payments",
		fmt.Sprintf(`{"order_id":%q,"method":"bank_transfer"}`, cancelledOrderID), cookie, csrf)
	if response.Code != http.StatusOK {
		t.Fatalf("Zahlung vor Stornierung: %d %s", response.Code, response.Body.String())
	}
	var cancelledPayment payment.Payment
	if err := json.Unmarshal(response.Body.Bytes(), &cancelledPayment); err != nil {
		t.Fatal(err)
	}
	if err := inventory.Cancel(ctx, db, cancelledOrderID); err != nil {
		t.Fatal(err)
	}
	response = authRequest(router, http.MethodPost, "/api/v1/mock-payments/"+cancelledPayment.ID+"/simulate",
		`{"outcome":"failed"}`, cookie, csrf)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"state":"failed"`)) {
		t.Fatalf("Verspäteter Fehler nach Stornierung: %d %s", response.Code, response.Body.String())
	}
	var cancelledStatus string
	if err := db.QueryRowContext(ctx, `SELECT status FROM orders WHERE id = $1`, cancelledOrderID).Scan(&cancelledStatus); err != nil || cancelledStatus != "CANCELLED" {
		t.Fatalf("Stornierung überschrieben: %s, %v", cancelledStatus, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT stock_quantity FROM product_variants WHERE id = $1`, variantID).Scan(&stock); err != nil || stock != 2 {
		t.Fatalf("Bestand nach Stornierung und spätem Fehler: %d statt 2, %v", stock, err)
	}
	lateOrderID := newOrder("NW-MOCK-LATE", "CREATED")
	if err := inventory.WithDeadlockRetry(ctx, db, func(tx *sql.Tx) error {
		return inventory.Reserve(ctx, tx, lateOrderID,
			[]inventory.Line{{VariantID: variantID, Quantity: 1}}, time.Now().Add(time.Minute))
	}); err != nil {
		t.Fatal(err)
	}
	response = authRequest(router, http.MethodPost, "/api/v1/mock-payments",
		fmt.Sprintf(`{"order_id":%q,"method":"debit_card"}`, lateOrderID), cookie, csrf)
	if response.Code != http.StatusOK {
		t.Fatalf("Zahlung vor Ablauf: %d %s", response.Code, response.Body.String())
	}
	var latePayment payment.Payment
	if err := json.Unmarshal(response.Body.Bytes(), &latePayment); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE order_reservations SET reserved_at = now() - interval '2 minutes',
		expires_at = now() - interval '1 second' WHERE order_id = $1`, lateOrderID); err != nil {
		t.Fatal(err)
	}
	response = authRequest(router, http.MethodPost, "/api/v1/mock-payments/"+latePayment.ID+"/simulate",
		`{"outcome":"succeeded"}`, cookie, csrf)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"state":"pending"`)) {
		t.Fatalf("Später Erfolg wurde als bezahlt behandelt: %d %s", response.Code, response.Body.String())
	}
	var lateDisposition string
	if err := db.QueryRowContext(ctx, `SELECT disposition FROM mock_payment_events WHERE payment_id = $1`, latePayment.ID).Scan(&lateDisposition); err != nil || lateDisposition != "manual_review" {
		t.Fatalf("Später Erfolg ohne Abgleichfall: %s, %v", lateDisposition, err)
	}
	if released, err := inventory.ReleaseExpired(ctx, db, time.Now(), 10); err != nil || released != 1 {
		t.Fatalf("Reservierung nach spätem Erfolg: %d, %v", released, err)
	}
	if err := repository.SyncExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT stock_quantity FROM product_variants WHERE id = $1`, variantID).Scan(&stock); err != nil || stock != 2 {
		t.Fatalf("Bestand nach spätem Erfolg: %d statt 2, %v", stock, err)
	}
}
