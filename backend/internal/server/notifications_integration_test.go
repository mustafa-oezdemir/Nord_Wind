package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/notification"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/notification/smtptest"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/payment"
)

type capturedMail struct {
	To, Subject, Text, HTML string
	Header                  mail.Header
}

func parseCapturedMail(t *testing.T, received smtptest.Received) capturedMail {
	t.Helper()
	message, err := mail.ReadMessage(strings.NewReader(received.Data))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	result := capturedMail{To: received.To[0], Subject: subject, Header: message.Header}
	_, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(part))
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(part.Header.Get("Content-Type"), "text/plain") {
			result.Text = string(body)
		} else {
			result.HTML = string(body)
		}
	}
	return result
}

// Prüft den Weg vom fachlichen Ereignis über Trigger, Outbox und Dispatcher bis zur
// SMTP-Zustellung an ein lokales Testpostfach – mit echten HTTP-Abläufen.
func TestNotificationFlowOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_NOTIFICATION_FLOW_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_NOTIFICATION_FLOW_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
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
	var existing int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM customers`).Scan(&existing); err != nil || existing != 0 {
		t.Fatalf("Leere Testdatenbank erforderlich: %d, %v", existing, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`TRUNCATE notification_outbox, mock_payment_events, mock_payments, order_reservations,
			order_shipments, shipment_attempts, order_status_events, order_items, orders, cart_discounts, cart_items,
			customer_addresses, product_variants, products, auth_sessions, auth_rate_limits, customers CASCADE`); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
		_ = db.Close()
	})

	products := catalog.NewRepository(db)
	pan, err := products.CreateProduct(ctx, catalog.Product{Name: "Pfanne", Slug: "pfanne", Status: catalog.ProductPublished})
	if err != nil {
		t.Fatal(err)
	}
	variant, err := products.CreateVariant(ctx, catalog.ProductVariant{ProductID: pan.ID, SKU: "NW-PFANNE", Label: "28 cm", PriceCents: 2500, StockQuantity: 50, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}

	inbox := smtptest.Start(t, smtptest.Options{})
	config := notification.Config{Enabled: true, Environment: "development", Host: "127.0.0.1", Port: inbox.Port(),
		TLS: notification.TLSNone, From: mail.Address{Name: "NordWinds", Address: "no-reply@nordwinds.local"},
		PublicOrigin: "https://shop.example.invalid", Preview: true}
	store := notification.NewStore(db)
	orderRepository := orders.NewRepository(db)
	dispatcher := notification.NewDispatcher(store, orderRepository, notification.NewSMTPTransport(config), config, discardLogger())

	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	service.SetSecurityNotifier(store, func(err error) { t.Errorf("Sicherheitshinweis: %v", err) })
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	RegisterCartRoutes(router, auth, cart.NewRepository(db))
	RegisterOrderRoutes(router, auth, orderRepository, true)
	RegisterAdminOrderRoutes(router, auth, orderRepository, discardLogger())
	RegisterAccountRoutes(router, auth, orderRepository)
	RegisterMockPaymentRoutes(router, auth, payment.NewRepository(db), bytes.Repeat([]byte("s"), 32))
	RegisterAdminNotificationRoutes(router, auth, store, discardLogger())

	delivered := 0
	flush := func() []capturedMail {
		t.Helper()
		for {
			processed, err := dispatcher.Tick(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if processed == 0 {
				break
			}
		}
		messages := inbox.Messages()
		result := []capturedMail{}
		for _, received := range messages[delivered:] {
			result = append(result, parseCapturedMail(t, received))
		}
		delivered = len(messages)
		return result
	}
	expectMails := func(label string, want int, subjectPart string) []capturedMail {
		t.Helper()
		mails := flush()
		if len(mails) != want {
			subjects := []string{}
			for _, m := range mails {
				subjects = append(subjects, m.Subject)
			}
			t.Fatalf("%s: %d E-Mails statt %d: %v", label, len(mails), want, subjects)
		}
		for _, m := range mails {
			if !strings.Contains(m.Subject, subjectPart) {
				t.Fatalf("%s: Betreff %q enthält nicht %q", label, m.Subject, subjectPart)
			}
		}
		return mails
	}

	password := "Synthetic-Notification-Passphrase-2026"
	register := func(email string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		if response := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); response.Code != http.StatusCreated {
			t.Fatalf("Registrierung: %d", response.Code)
		}
	}
	login := func(email string) (*http.Cookie, string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login", body, nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("Login %s: %d", email, response.Code)
		}
		return sessionCookie(t, response), csrfFromResponse(t, response)
	}

	t.Run("Registrierung: Begrüßung mit korrekten Kopfzeilen", func(t *testing.T) {
		register("kunde@example.invalid")
		register("admin@example.invalid")
		mails := expectMails("Begrüßung", 2, "Willkommen bei NordWinds")
		m := mails[0]
		if m.To != "kunde@example.invalid" || m.Header.Get("Auto-Submitted") != "auto-generated" ||
			!strings.Contains(m.Header.Get("From"), "no-reply@nordwinds.local") || !strings.Contains(m.Text, "https://shop.example.invalid/konto") ||
			!strings.Contains(m.HTML, "Vorschau") || strings.Contains(m.Text+m.HTML, password) {
			t.Fatalf("Begrüßung: %+v", m)
		}
	})
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE email = 'admin@example.invalid'`); err != nil {
		t.Fatal(err)
	}

	t.Run("Gesperrte Anmeldung: ein Hinweis, gleiche Antwort für unbekannte Adressen", func(t *testing.T) {
		codes := map[string][]int{}
		for _, email := range []string{"kunde@example.invalid", "unbekannt@example.invalid"} {
			for range 7 {
				body := fmt.Sprintf(`{"email":%q,"password":"falsches-passwort-1234"}`, email)
				codes[email] = append(codes[email], authRequest(router, http.MethodPost, "/api/v1/auth/login", body, nil, "").Code)
			}
		}
		if fmt.Sprint(codes["kunde@example.invalid"]) != fmt.Sprint(codes["unbekannt@example.invalid"]) ||
			codes["kunde@example.invalid"][5] != http.StatusTooManyRequests {
			t.Fatalf("Antworten unterscheiden sich: %v", codes)
		}
		mails := expectMails("Sicherheitshinweis", 1, "Sicherheitshinweis")
		if mails[0].To != "kunde@example.invalid" || !strings.Contains(mails[0].Text, "15 Minuten") {
			t.Fatalf("Sicherheitshinweis: %+v", mails[0])
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM auth_rate_limits`); err != nil {
			t.Fatal(err)
		}
	})

	customer, csrf := login("kunde@example.invalid")
	admin, adminCSRF := login("admin@example.invalid")
	placeOrder := func(key string) orders.Order {
		t.Helper()
		if response := authRequest(router, http.MethodPost, "/api/v1/cart/items",
			fmt.Sprintf(`{"variant_id":%q,"quantity":2}`, variant.ID), customer, csrf); response.Code != http.StatusOK {
			t.Fatalf("Warenkorb: %d %s", response.Code, response.Body.String())
		}
		var snapshot cart.Snapshot
		response := authRequest(router, http.MethodGet, "/api/v1/cart", "", customer, "")
		if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		return decodeOrder(t, orderRequest(router, orderBody(snapshot.Pricing.TotalCents), customer, csrf, "benachrichtigung-"+key), http.StatusCreated)
	}
	initiate := func(order orders.Order) payment.Payment {
		t.Helper()
		response := authRequest(router, http.MethodPost, "/api/v1/mock-payments",
			fmt.Sprintf(`{"order_id":%q,"method":"paypal"}`, order.ID), customer, csrf)
		var created payment.Payment
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &created) != nil {
			t.Fatalf("Zahlung: %d %s", response.Code, response.Body.String())
		}
		return created
	}
	simulate := func(p payment.Payment, outcome string) int {
		return authRequest(router, http.MethodPost, "/api/v1/mock-payments/"+p.ID+"/simulate",
			fmt.Sprintf(`{"outcome":%q}`, outcome), customer, csrf).Code
	}
	transition := func(number, status, tracking string) {
		t.Helper()
		body := fmt.Sprintf(`{"status":%q,"tracking_number":%q}`, status, tracking)
		if response := authRequest(router, http.MethodPatch, "/api/v1/admin/orders/"+number+"/status", body, admin, adminCSRF); response.Code != http.StatusOK {
			t.Fatalf("Status %s: %d %s", status, response.Code, response.Body.String())
		}
	}

	var first orders.Order
	t.Run("Bestellung, Zahlung, Versand, Zustellung und Erstattung", func(t *testing.T) {
		first = placeOrder("a")
		confirmation := expectMails("Bestellbestätigung", 1, "Bestellbestätigung "+first.OrderNumber)[0]
		for _, want := range []string{"Hallo Erika,", "2 × Pfanne (28 cm): 50,00 €", "Gesamtsumme: 50,00 €", "51147 Köln",
			"https://shop.example.invalid/bestellung/" + first.OrderNumber, "rechtliches/widerruf"} {
			if !strings.Contains(strings.ReplaceAll(confirmation.Text, "\u00a0", " "), want) {
				t.Fatalf("Bestellbestätigung ohne %q:\n%s", want, confirmation.Text)
			}
		}
		if !strings.Contains(confirmation.HTML, "Heidestraße 17") || !strings.Contains(confirmation.HTML, "Bestellung ansehen") {
			t.Fatal("HTML-Fassung unvollständig")
		}

		pay := initiate(first)
		expectMails("Zahlung gestartet", 0, "")
		if code := simulate(pay, "succeeded"); code != http.StatusOK {
			t.Fatalf("Zahlung: %d", code)
		}
		expectMails("Zahlung erhalten", 1, "Zahlung erhalten – Bestellung "+first.OrderNumber)
		if code := simulate(pay, "succeeded"); code != http.StatusConflict {
			t.Fatalf("doppelte Zahlung: %d", code)
		}
		expectMails("doppelte Zahlung", 0, "")

		transition(first.OrderNumber, "PROCESSING", "")
		expectMails("In Bearbeitung", 1, "wird vorbereitet")
		transition(first.OrderNumber, "SHIPPED", "NW-PAKET-4711")
		shipped := expectMails("Versand", 1, "ist unterwegs")[0]
		if !strings.Contains(shipped.Text, "Sendungsnummer: NW-PAKET-4711") {
			t.Fatalf("Versand ohne Sendungsnummer:\n%s", shipped.Text)
		}
		transition(first.OrderNumber, "DELIVERED", "")
		expectMails("Zustellung", 1, "wurde zugestellt")
		if code := simulate(pay, "refunded"); code != http.StatusOK {
			t.Fatalf("Erstattung: %d", code)
		}
		refund := expectMails("Erstattung", 1, "Erstattung für Bestellung "+first.OrderNumber)[0]
		if !strings.Contains(strings.ReplaceAll(refund.Text, "\u00a0", " "), "Erstattet: 50,00 €") {
			t.Fatalf("Erstattung ohne Betrag:\n%s", refund.Text)
		}
		if code := simulate(pay, "refunded"); code == http.StatusOK {
			expectMails("doppelte Erstattung", 0, "")
		}
	})

	t.Run("Fehlgeschlagene Zahlung und Stornierung", func(t *testing.T) {
		failed := placeOrder("b")
		expectMails("Bestellbestätigung B", 1, "Bestellbestätigung")
		if code := simulate(initiate(failed), "failed"); code != http.StatusOK {
			t.Fatalf("Fehlschlag: %d", code)
		}
		mail := expectMails("Zahlung fehlgeschlagen", 1, "Zahlung fehlgeschlagen")[0]
		if !strings.Contains(mail.Text, "kein Betrag eingezogen") {
			t.Fatalf("Fehlschlag-Text:\n%s", mail.Text)
		}

		cancelled := placeOrder("c")
		expectMails("Bestellbestätigung C", 1, "Bestellbestätigung")
		if response := authRequest(router, http.MethodPost, "/api/v1/orders/"+cancelled.OrderNumber+"/cancel", "", customer, csrf); response.Code != http.StatusOK {
			t.Fatalf("Stornierung: %d %s", response.Code, response.Body.String())
		}
		storno := expectMails("Stornierung", 1, "storniert")[0]
		if !strings.Contains(storno.Text, "wie gewünscht") {
			t.Fatalf("Stornotext:\n%s", storno.Text)
		}
		if response := authRequest(router, http.MethodPost, "/api/v1/orders/"+cancelled.OrderNumber+"/cancel", "", customer, csrf); response.Code != http.StatusOK {
			t.Fatalf("zweite Stornierung: %d", response.Code)
		}
		expectMails("zweite Stornierung", 0, "")
	})

	t.Run("Zustellfehler blockieren die Bestellung nicht und werden nachgeholt", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		closedPort := listener.Addr().(*net.TCPAddr).Port
		_ = listener.Close()
		broken := config
		broken.Port = closedPort
		brokenDispatcher := notification.NewDispatcher(store, orderRepository, notification.NewSMTPTransport(broken), broken, discardLogger())

		order := placeOrder("d")
		if _, err := brokenDispatcher.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		var state, lastError string
		var attempts int
		if err := db.QueryRowContext(ctx, `SELECT state, last_error, attempts FROM notification_outbox
			WHERE order_number = $1`, order.OrderNumber).Scan(&state, &lastError, &attempts); err != nil {
			t.Fatal(err)
		}
		if state != "pending" || lastError != "connect" || attempts != 1 {
			t.Fatalf("nach Fehler: %s %s %d", state, lastError, attempts)
		}
		if own := decodeOrder(t, authRequest(router, http.MethodGet, "/api/v1/orders/"+order.OrderNumber, "", customer, ""), http.StatusOK); own.Status != orders.Created {
			t.Fatalf("Bestellung durch Mailfehler verändert: %s", own.Status)
		}
		expectMails("vor Ablauf des Backoffs", 0, "")
		if _, err := db.ExecContext(ctx, `UPDATE notification_outbox SET next_attempt_at = now() WHERE order_number = $1`, order.OrderNumber); err != nil {
			t.Fatal(err)
		}
		expectMails("nachgeholt", 1, "Bestellbestätigung "+order.OrderNumber)
	})

	t.Run("Verwaltung: nur Admins, keine Adressen, erneuter Versand mit CSRF", func(t *testing.T) {
		assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/notifications", "", customer, ""), http.StatusForbidden, "FORBIDDEN")
		response := authRequest(router, http.MethodGet, "/api/v1/admin/notifications?state=sent&limit=100", "", admin, "")
		var listed struct {
			Data   []notification.AdminEntry `json:"data"`
			Counts map[string]int64          `json:"counts"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &listed) != nil || len(listed.Data) < 10 ||
			listed.Counts["sent"] != int64(len(inbox.Messages())) || strings.Contains(response.Body.String(), "@example.invalid") {
			t.Fatalf("Liste: %d %s", response.Code, response.Body.String())
		}
		assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/notifications?state=kaputt", "", admin, ""), http.StatusBadRequest, "INVALID_REQUEST")

		var id int64
		if err := db.QueryRowContext(ctx, `UPDATE notification_outbox SET state = 'failed', sent_at = NULL, last_error = 'connect'
			WHERE id = (SELECT id FROM notification_outbox WHERE kind = 'order.paid') RETURNING id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		path := fmt.Sprintf("/api/v1/admin/notifications/%d/retry", id)
		assertProblem(t, authRequest(router, http.MethodPost, path, "", admin, ""), http.StatusForbidden, "FORBIDDEN")
		if response := authRequest(router, http.MethodPost, path, "", admin, adminCSRF); response.Code != http.StatusNoContent {
			t.Fatalf("Retry: %d", response.Code)
		}
		assertProblem(t, authRequest(router, http.MethodPost, path, "", admin, adminCSRF), http.StatusConflict, "CONFLICT")
		assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/notifications/999999/retry", "", admin, adminCSRF), http.StatusNotFound, "NOT_FOUND")
		expectMails("erneuter Versand", 1, "Zahlung erhalten")
	})

	t.Run("Kontoschließung: Bestätigung an die bisherige Adresse, danach keine Post", func(t *testing.T) {
		register("schliessen@example.invalid")
		expectMails("Begrüßung", 1, "Willkommen")
		owner, ownerCSRF := login("schliessen@example.invalid")
		if response := authRequest(router, http.MethodDelete, "/api/v1/customer/me", "", owner, ownerCSRF); response.Code != http.StatusNoContent && response.Code != http.StatusOK {
			t.Fatalf("Schließung: %d %s", response.Code, response.Body.String())
		}
		closed := expectMails("Schließung", 1, "Kundenkonto wurde geschlossen")[0]
		if closed.To != "schliessen@example.invalid" {
			t.Fatalf("Empfänger: %s", closed.To)
		}
		var remaining int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notification_outbox WHERE recipient IS NOT NULL`).Scan(&remaining); err != nil || remaining != 0 {
			t.Fatalf("gespeicherte Adressen nach Versand: %d, %v", remaining, err)
		}
	})
}
