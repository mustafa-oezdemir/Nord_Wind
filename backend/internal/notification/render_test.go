package notification

import (
	"encoding/json"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

func testConfig(preview bool) Config {
	return Config{Enabled: true, PublicOrigin: "https://shop.nordwinds.de", Preview: preview,
		From: mail.Address{Name: "NordWinds", Address: "no-reply@nordwinds.de"}}
}

func sampleOrder() orders.Order {
	created := time.Date(2026, 10, 31, 9, 5, 0, 0, time.UTC)
	return orders.Order{
		ID: "order-id", OrderNumber: "NW-2026-100001", Status: orders.Created, CustomerEmail: "erika@example.org",
		Address: orders.Address{FirstName: "Erika", LastName: "Musterfrau", Street: "Heidestraße", HouseNumber: "17",
			AddressExtra: "Hinterhaus", PostalCode: "51147", City: "Köln", Country: "DE"},
		Items: []orders.Item{
			{Position: 1, ProductName: "Gusseisenpfanne", VariantLabel: "28 cm", Quantity: 2, LineTotalCents: 5998},
			{Position: 2, ProductName: "Kochbuch Nordküste", VariantLabel: "Gebunden", Quantity: 1, LineTotalCents: 1999},
		},
		Pricing: orders.Pricing{GoodsCents: 7997, DiscountCents: 800, ShippingCents: 0, TotalCents: 7197, Currency: "EUR",
			Taxes: []orders.TaxLine{{TaxClass: "standard", RateBP: 1900, VATCents: 862}, {TaxClass: "reduced", RateBP: 700, VATCents: 118}}},
		Discount:  &orders.Discount{Code: "HERBST10", PercentBP: 1000},
		Shipment:  &orders.Shipment{Carrier: "MANUAL", TrackingNumber: "NW-PAKET-4711", CreatedAt: created.Add(48 * time.Hour)},
		CreatedAt: created,
	}
}

func TestComposeAndRenderEveryKind(t *testing.T) {
	order := sampleOrder()
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	expectations := map[Kind][]string{
		KindWelcome:        {"Willkommen bei NordWinds", "https://shop.nordwinds.de/konto"},
		KindAccountClosed:  {"Ihr Kundenkonto wurde geschlossen", "Geschlossen am"},
		KindLoginLocked:    {"mehrere fehlgeschlagene Anmeldeversuche", "15 Minuten"},
		KindOrderCreated:   {"Bestellbestätigung NW-2026-100001", "Gusseisenpfanne", "79,97", "Rabatt HERBST10", "71,97", "kostenlos", "8,62", "51147 Köln", "rechtliches/widerruf"},
		KindOrderPaid:      {"Zahlung erhalten", "71,97"},
		KindPaymentFailed:  {"Zahlung fehlgeschlagen", "kein Betrag eingezogen", "/warenkorb"},
		KindOrderProcess:   {"wird vorbereitet"},
		KindOrderShipped:   {"ist unterwegs", "NW-PAKET-4711", "Sendung verfolgen"},
		KindOrderDelivered: {"wurde zugestellt", "rechtliches/widerruf"},
		KindOrderCancelled: {"storniert", "automatisch storniert"},
		KindOrderRefunded:  {"Erstattung", "12,34"},
	}
	for _, kind := range Kinds {
		t.Run(string(kind), func(t *testing.T) {
			n := Notification{ID: 1, Kind: kind, CreatedAt: now}
			var source *orders.Order
			if strings.HasPrefix(string(kind), "order.") {
				n.OrderNumber = order.OrderNumber
				source = &order
				payload := map[string]any{"actor": "system"}
				if kind == KindOrderRefunded {
					payload["amount_cents"] = 1234
				}
				n.Payload, _ = json.Marshal(payload)
			}
			view, err := Compose(testConfig(true), n, source, now)
			if err != nil {
				t.Fatal(err)
			}
			subject, text, html, err := Render(view)
			if err != nil {
				t.Fatal(err)
			}
			if subject == "" || !strings.Contains(html, "<html lang=\"de\">") || !strings.Contains(text, view.Heading) {
				t.Fatalf("Grundgerüst fehlt: %q", subject)
			}
			for _, want := range expectations[kind] {
				if !strings.Contains(subject+html, want) || (!strings.Contains(subject+text, want) && !strings.Contains(text, strings.ReplaceAll(want, " ", " "))) {
					t.Errorf("%q fehlt in HTML oder Text", want)
				}
			}
			for _, part := range []string{html, text} {
				if !strings.Contains(part, "Vorschau: Dieser Shop ist im Testbetrieb") || !strings.Contains(part, "Impressum") ||
					!strings.Contains(part, "Antworten an diese Adresse werden nicht gelesen") {
					t.Errorf("Pflichtangaben im Fuß fehlen")
				}
				// Weder Passwörter noch Tokens oder interne Kennungen gehören in E-Mails.
				for _, secret := range []string{"Passwort:", "token", "order-id", "csrf"} {
					if strings.Contains(strings.ToLower(part), strings.ToLower(secret)) {
						t.Errorf("unerwarteter Inhalt %q", secret)
					}
				}
			}
		})
	}

	// Ohne Vorschau und mit Antwortadresse entfallen beide Hinweise.
	config := testConfig(false)
	config.ReplyTo = &mail.Address{Address: "service@nordwinds.de"}
	view, _ := Compose(config, Notification{Kind: KindWelcome, CreatedAt: now}, nil, now)
	_, text, html, _ := Render(view)
	if strings.Contains(html+text, "Testbetrieb") || strings.Contains(html+text, "nicht gelesen") {
		t.Fatal("Vorschau- oder No-Reply-Hinweis trotz Produktion und Antwortadresse")
	}
}

func TestCancelledWordingFollowsActor(t *testing.T) {
	order := sampleOrder()
	for actor, want := range map[string]string{
		"customer": "wie gewünscht haben wir Ihre Bestellung storniert",
		"system":   "weil die Zahlung nicht rechtzeitig abgeschlossen wurde",
		"admin":    "Ihre Bestellung wurde storniert.",
	} {
		payload, _ := json.Marshal(map[string]string{"actor": actor})
		view, err := Compose(testConfig(true), Notification{Kind: KindOrderCancelled, OrderNumber: order.OrderNumber, Payload: payload}, &order, time.Now())
		if err != nil || !strings.Contains(strings.Join(view.Paragraphs, " "), want) {
			t.Fatalf("%s: %v %v", actor, view.Paragraphs, err)
		}
	}
}

func TestRenderEscapesCustomerData(t *testing.T) {
	order := sampleOrder()
	order.Address.FirstName = `"><img src=x onerror=alert(1)>`
	order.Items[0].ProductName = "<script>alert('pfanne')</script>"
	order.Address.City = "Köln & Umgebung"
	view, err := Compose(testConfig(true), Notification{Kind: KindOrderCreated, OrderNumber: order.OrderNumber}, &order, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, _, html, err := Render(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"<script>alert", `<img src=x`, "Köln & Umgebung"} {
		if strings.Contains(html, raw) {
			t.Fatalf("ungeschützte Ausgabe %q", raw)
		}
	}
	if !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, "Köln &amp; Umgebung") {
		t.Fatal("erwartete Maskierung fehlt")
	}

	// Ein Link mit fremdem Schema wird von html/template entschärft.
	view.Action = &Action{Label: "Klick", URL: "javascript:alert(1)"}
	_, _, html, _ = Render(view)
	if strings.Contains(html, `href="javascript:`) || !strings.Contains(html, `href="#ZgotmplZ"`) {
		t.Fatal("javascript:-Link nicht entschärft")
	}
}

func TestComposeRejectsUnknownOrMissingData(t *testing.T) {
	if _, err := Compose(testConfig(true), Notification{Kind: "marketing.newsletter"}, nil, time.Now()); err == nil {
		t.Fatal("unbekannte Art akzeptiert")
	}
	if _, err := Compose(testConfig(true), Notification{Kind: KindOrderPaid, OrderNumber: "NW-2026-100001"}, nil, time.Now()); err == nil {
		t.Fatal("Bestellart ohne Bestellung akzeptiert")
	}
}

func TestFormatting(t *testing.T) {
	for cents, want := range map[int64]string{0: "0,00 €", 5: "0,05 €", 129999: "1.299,99 €", 123456789: "1.234.567,89 €", -490: "−4,90 €"} {
		if got := FormatEuro(cents); got != want {
			t.Errorf("FormatEuro(%d) = %q, erwartet %q", cents, got, want)
		}
	}
	if got := FormatDateTime(time.Date(2026, 10, 31, 10, 30, 0, 0, time.UTC)); got != "31. Oktober 2026, 11:30 Uhr" {
		t.Errorf("Winterzeit: %q", got)
	}
	if got := FormatDateTime(time.Date(2026, 7, 1, 10, 30, 0, 0, time.UTC)); got != "1. Juli 2026, 12:30 Uhr" {
		t.Errorf("Sommerzeit: %q", got)
	}
	if formatPercent(1000) != "10 %" || formatPercent(1250) != "12,50 %" {
		t.Errorf("Prozent: %q %q", formatPercent(1000), formatPercent(1250))
	}
}
