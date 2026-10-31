package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Europe/Berlin auch in minimalen Container-Images

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

// Kind ist die Art einer Benachrichtigung; die Liste entspricht dem CHECK in Migration 15.
type Kind string

const (
	KindWelcome        Kind = "account.welcome"
	KindAccountClosed  Kind = "account.closed"
	KindLoginLocked    Kind = "security.login_locked"
	KindOrderCreated   Kind = "order.created"
	KindOrderPaid      Kind = "order.paid"
	KindPaymentFailed  Kind = "order.payment_failed"
	KindOrderProcess   Kind = "order.processing"
	KindOrderShipped   Kind = "order.shipped"
	KindOrderDelivered Kind = "order.delivered"
	KindOrderCancelled Kind = "order.cancelled"
	KindOrderRefunded  Kind = "order.refunded"
)

// Kinds sind alle bekannten Arten. Alle sind transaktional (Vertrag, Sicherheit, Konto);
// Werbung gibt es nicht und würde eine eigene Einwilligung verlangen (ADR-011).
var Kinds = []Kind{KindWelcome, KindAccountClosed, KindLoginLocked, KindOrderCreated, KindOrderPaid,
	KindPaymentFailed, KindOrderProcess, KindOrderShipped, KindOrderDelivered, KindOrderCancelled, KindOrderRefunded}

// Fact ist eine Zeile „Bezeichnung: Wert“.
type Fact struct {
	Label  string
	Value  string
	Strong bool
}

// ItemLine ist eine Bestellposition in der E-Mail.
type ItemLine struct {
	Quantity int32
	Name     string
	Detail   string
	Total    string
}

// Action ist der Hauptknopf der E-Mail.
type Action struct {
	Label string
	URL   string
}

// View ist der Inhalt einer E-Mail; ein gemeinsames Layout rendert HTML und Text.
type View struct {
	Subject    string
	Preheader  string
	Heading    string
	Greeting   string
	Paragraphs []string
	Facts      []Fact
	Items      []ItemLine
	Totals     []Fact
	Address    []string
	Action     *Action
	Notes      []string

	ShopName    string
	Origin      string
	ImprintURL  string
	PrivacyURL  string
	Preview     bool
	NoReply     bool
	CurrentYear int
}

// Notification ist ein geholter Outbox-Eintrag.
type Notification struct {
	ID          int64
	Kind        Kind
	CustomerID  string
	OrderNumber string
	Recipient   string
	Payload     json.RawMessage
	Attempts    int
	CreatedAt   time.Time
}

var berlin = mustLocation("Europe/Berlin")

func mustLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

var months = [...]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August",
	"September", "Oktober", "November", "Dezember"}

// FormatDateTime schreibt z. B. „31. Oktober 2026, 11:30 Uhr“ in deutscher Zeit.
func FormatDateTime(value time.Time) string {
	local := value.In(berlin)
	return fmt.Sprintf("%d. %s %d, %02d:%02d Uhr", local.Day(), months[local.Month()-1], local.Year(), local.Hour(), local.Minute())
}

// FormatEuro schreibt Cent-Beträge deutsch, z. B. 129999 → „1.299,99 €“.
func FormatEuro(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "−", -cents
	}
	euros := fmt.Sprint(cents / 100)
	var grouped strings.Builder
	for index, digit := range euros {
		if index > 0 && (len(euros)-index)%3 == 0 {
			grouped.WriteByte('.')
		}
		grouped.WriteRune(digit)
	}
	return fmt.Sprintf("%s%s,%02d €", sign, grouped.String(), cents%100)
}

func formatPercent(basisPoints int64) string {
	if basisPoints%100 == 0 {
		return fmt.Sprintf("%d %%", basisPoints/100)
	}
	return strings.Replace(fmt.Sprintf("%.2f %%", float64(basisPoints)/100), ".", ",", 1)
}

var errUnknownKind = errors.New("unbekannte Benachrichtigungsart")

// Compose baut den Inhalt. order ist nur für Bestellarten gesetzt.
func Compose(config Config, n Notification, order *orders.Order, now time.Time) (View, error) {
	view := View{
		ShopName: "NordWinds", Origin: config.PublicOrigin,
		ImprintURL: config.PublicOrigin + "/rechtliches/impressum", PrivacyURL: config.PublicOrigin + "/rechtliches/datenschutz",
		Preview: config.Preview, NoReply: config.ReplyTo == nil, CurrentYear: now.In(berlin).Year(),
	}
	contact := "Bei Fragen erreichen Sie uns über die Kontaktangaben im Impressum."
	switch n.Kind {
	case KindWelcome:
		view.Subject = "Willkommen bei NordWinds"
		view.Preheader = "Ihr Kundenkonto ist eingerichtet."
		view.Heading = "Willkommen bei NordWinds"
		view.Greeting = "Hallo,"
		view.Paragraphs = []string{
			"Ihr Kundenkonto ist eingerichtet. Ab jetzt bleibt Ihr Warenkorb gespeichert, und Sie sehen alle Bestellungen an einem Ort.",
		}
		view.Action = &Action{"Zum Kundenkonto", config.PublicOrigin + "/konto"}
		view.Notes = []string{"Sie haben sich nicht bei NordWinds registriert? Dann wenden Sie sich bitte über die Kontaktangaben im Impressum an uns."}
	case KindAccountClosed:
		view.Subject = "Ihr Kundenkonto wurde geschlossen"
		view.Preheader = "Anmeldung, Warenkorb und Adressbuch sind entfernt."
		view.Heading = "Ihr Kundenkonto ist geschlossen"
		view.Greeting = "Hallo,"
		view.Paragraphs = []string{
			"Ihr NordWinds-Kundenkonto wurde auf Ihren Wunsch geschlossen. Anmeldung, Warenkorb und Adressbuch sind entfernt.",
			"Bestellungen bewahren wir auf, soweit gesetzliche Aufbewahrungspflichten das verlangen.",
		}
		view.Facts = []Fact{{Label: "Geschlossen am", Value: FormatDateTime(n.CreatedAt)}}
		view.Notes = []string{"Sie haben das nicht veranlasst? Dann wenden Sie sich bitte umgehend über die Kontaktangaben im Impressum an uns."}
	case KindLoginLocked:
		view.Subject = "Sicherheitshinweis: mehrere fehlgeschlagene Anmeldeversuche"
		view.Preheader = "Weitere Anmeldeversuche sind für 15 Minuten gesperrt."
		view.Heading = "Mehrere fehlgeschlagene Anmeldeversuche"
		view.Greeting = "Hallo,"
		view.Paragraphs = []string{
			"für Ihr Kundenkonto gab es mehrere Anmeldeversuche mit falschem Passwort. Zum Schutz sind weitere Versuche für 15 Minuten gesperrt.",
			"Waren Sie das selbst, können Sie sich danach wie gewohnt anmelden. Waren Sie es nicht, bleibt Ihr Konto geschützt, solange niemand Ihr Passwort kennt. Verwenden Sie für NordWinds ein Passwort, das Sie nirgendwo sonst nutzen.",
		}
		view.Facts = []Fact{{Label: "Zeitpunkt", Value: FormatDateTime(n.CreatedAt)}}
		view.Notes = []string{"Wir fragen nie per E-Mail nach Ihrem Passwort. " + contact}
	default:
		if !strings.HasPrefix(string(n.Kind), "order.") {
			return View{}, errUnknownKind
		}
		if order == nil {
			return View{}, errors.New("Bestellung fehlt")
		}
		if err := composeOrder(&view, config, n, *order); err != nil {
			return View{}, err
		}
		view.Notes = append(view.Notes, contact)
	}
	return view, nil
}

func composeOrder(view *View, config Config, n Notification, order orders.Order) error {
	number := order.OrderNumber
	view.Greeting = "Hallo " + order.Address.FirstName + ","
	view.Action = &Action{"Bestellung ansehen", config.PublicOrigin + "/bestellung/" + number}
	view.Facts = []Fact{{Label: "Bestellnummer", Value: number}}
	var payload struct {
		Actor       string `json:"actor"`
		AmountCents int64  `json:"amount_cents"`
	}
	if len(n.Payload) > 0 {
		if err := json.Unmarshal(n.Payload, &payload); err != nil {
			return err
		}
	}
	switch n.Kind {
	case KindOrderCreated:
		view.Subject = "Bestellbestätigung " + number
		view.Preheader = "Wir haben Ihre Bestellung erhalten."
		view.Heading = "Danke für Ihre Bestellung"
		view.Paragraphs = []string{"wir haben Ihre Bestellung erhalten. Hier ist Ihre Übersicht. Die Zahlung ist noch offen; sobald sie eingegangen ist, bekommen Sie eine weitere E-Mail."}
		view.Facts = append(view.Facts, Fact{Label: "Bestellt am", Value: FormatDateTime(order.CreatedAt)})
		for _, item := range order.Items {
			view.Items = append(view.Items, ItemLine{Quantity: item.Quantity, Name: item.ProductName,
				Detail: item.VariantLabel, Total: FormatEuro(item.LineTotalCents)})
		}
		p := order.Pricing
		view.Totals = []Fact{{Label: "Warenwert", Value: FormatEuro(p.GoodsCents)}}
		if p.DiscountCents > 0 && order.Discount != nil {
			view.Totals = append(view.Totals, Fact{Label: "Rabatt " + order.Discount.Code + " (" + formatPercent(order.Discount.PercentBP) + ")",
				Value: "−" + FormatEuro(p.DiscountCents)})
		}
		shipping := "kostenlos"
		if p.ShippingCents > 0 {
			shipping = FormatEuro(p.ShippingCents)
		}
		view.Totals = append(view.Totals, Fact{Label: "Versand innerhalb Deutschlands", Value: shipping},
			Fact{Label: "Gesamtsumme", Value: FormatEuro(p.TotalCents), Strong: true})
		for _, tax := range p.Taxes {
			view.Totals = append(view.Totals, Fact{Label: "enthaltene MwSt. " + formatPercent(tax.RateBP), Value: FormatEuro(tax.VATCents)})
		}
		a := order.Address
		view.Address = []string{a.FirstName + " " + a.LastName}
		if a.Company != "" {
			view.Address = append(view.Address, a.Company)
		}
		view.Address = append(view.Address, a.Street+" "+a.HouseNumber)
		if a.AddressExtra != "" {
			view.Address = append(view.Address, a.AddressExtra)
		}
		view.Address = append(view.Address, a.PostalCode+" "+a.City, "Deutschland")
		view.Notes = []string{"Informationen zu Ihrem Widerrufsrecht finden Sie unter " + config.PublicOrigin + "/rechtliches/widerruf."}
	case KindOrderPaid:
		view.Subject = "Zahlung erhalten – Bestellung " + number
		view.Preheader = "Ihre Zahlung ist eingegangen."
		view.Heading = "Zahlung erhalten"
		view.Paragraphs = []string{"Ihre Zahlung ist eingegangen. Wir bereiten Ihre Bestellung jetzt für den Versand vor."}
		view.Facts = append(view.Facts, Fact{Label: "Betrag", Value: FormatEuro(order.Pricing.TotalCents), Strong: true})
	case KindPaymentFailed:
		view.Subject = "Zahlung fehlgeschlagen – Bestellung " + number
		view.Preheader = "Die Zahlung konnte nicht abgeschlossen werden."
		view.Heading = "Die Zahlung hat nicht geklappt"
		view.Paragraphs = []string{
			"die Zahlung für Ihre Bestellung konnte nicht abgeschlossen werden. Es wurde kein Betrag eingezogen.",
			"Die reservierten Artikel sind wieder freigegeben. Wenn Sie sie weiterhin möchten, legen Sie sie bitte erneut in den Warenkorb.",
		}
		view.Action = &Action{"Zum Warenkorb", config.PublicOrigin + "/warenkorb"}
	case KindOrderProcess:
		view.Subject = "Ihre Bestellung " + number + " wird vorbereitet"
		view.Preheader = "Wir packen Ihre Bestellung."
		view.Heading = "Ihre Bestellung wird vorbereitet"
		view.Paragraphs = []string{"wir packen gerade Ihre Bestellung. Sobald das Paket unterwegs ist, schicken wir Ihnen die Sendungsnummer."}
	case KindOrderShipped:
		view.Subject = "Ihre Bestellung " + number + " ist unterwegs"
		view.Preheader = "Ihr Paket wurde versendet."
		view.Heading = "Ihre Bestellung ist unterwegs"
		view.Paragraphs = []string{"Ihr Paket hat unser Lager verlassen. Den aktuellen Stand sehen Sie in Ihrer Bestellung."}
		if order.Shipment != nil {
			view.Facts = append(view.Facts, Fact{Label: "Sendungsnummer", Value: order.Shipment.TrackingNumber, Strong: true},
				Fact{Label: "Versendet am", Value: FormatDateTime(order.Shipment.CreatedAt)})
		}
		view.Action = &Action{"Sendung verfolgen", config.PublicOrigin + "/bestellung/" + number}
	case KindOrderDelivered:
		view.Subject = "Ihre Bestellung " + number + " wurde zugestellt"
		view.Preheader = "Ihr Paket ist angekommen."
		view.Heading = "Ihre Bestellung ist angekommen"
		view.Paragraphs = []string{"laut Versandstatus wurde Ihre Bestellung zugestellt. Wir wünschen Ihnen viel Freude damit."}
		view.Notes = []string{"Informationen zu Ihrem Widerrufsrecht finden Sie unter " + config.PublicOrigin + "/rechtliches/widerruf."}
	case KindOrderCancelled:
		view.Subject = "Bestellung " + number + " storniert"
		view.Preheader = "Ihre Bestellung wurde storniert."
		view.Heading = "Ihre Bestellung ist storniert"
		reason := "Ihre Bestellung wurde storniert."
		switch payload.Actor {
		case "customer":
			reason = "wie gewünscht haben wir Ihre Bestellung storniert."
		case "system":
			reason = "Ihre Bestellung wurde automatisch storniert, weil die Zahlung nicht rechtzeitig abgeschlossen wurde."
		}
		view.Paragraphs = []string{reason, "Es wurde kein Betrag eingezogen, und die reservierten Artikel sind wieder freigegeben."}
	case KindOrderRefunded:
		view.Subject = "Erstattung für Bestellung " + number
		view.Preheader = "Wir haben Ihnen den Betrag erstattet."
		view.Heading = "Ihre Erstattung ist veranlasst"
		amount := payload.AmountCents
		if amount == 0 {
			amount = order.Pricing.TotalCents
		}
		view.Paragraphs = []string{"wir haben Ihnen den Betrag für Ihre Bestellung erstattet. Je nach Zahlungsart kann die Gutschrift einige Tage dauern."}
		view.Facts = append(view.Facts, Fact{Label: "Erstattet", Value: FormatEuro(amount), Strong: true})
	default:
		return errUnknownKind
	}
	return nil
}
