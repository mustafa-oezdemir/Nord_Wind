// Package payment beschreibt den ausschließlich simulierten NordWinds-Zahlungsablauf.
package payment

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Method string

const (
	DebitCard    Method = "debit_card"
	Klarna       Method = "klarna"
	PayPal       Method = "paypal"
	BankTransfer Method = "bank_transfer"
)

func (method Method) Valid() bool {
	switch method {
	case DebitCard, Klarna, PayPal, BankTransfer:
		return true
	default:
		return false
	}
}

type Outcome string

const (
	Succeeded Outcome = "succeeded"
	Failed    Outcome = "failed"
	Refunded  Outcome = "refunded"
)

func (outcome Outcome) Valid() bool {
	switch outcome {
	case Succeeded, Failed, Refunded:
		return true
	default:
		return false
	}
}

var ErrInvalidSignature = errors.New("ungültige Mock-Webhook-Signatur")

var paymentIDPattern = regexp.MustCompile(`^pay_[0-9a-f]{32}$`)
var eventIDPattern = regexp.MustCompile(`^evt_[0-9a-f]{32}$`)
var orderIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// Event ist die fachliche Nutzlast eines simulierten Zahlungs-Webhooks.
type Event struct {
	ID          string  `json:"id"`
	PaymentID   string  `json:"payment_id"`
	OrderID     string  `json:"order_id"`
	Method      Method  `json:"method"`
	Outcome     Outcome `json:"outcome"`
	AmountCents int64   `json:"amount_cents"`
	Currency    string  `json:"currency"`
}

func (event Event) Valid() bool {
	return eventIDPattern.MatchString(event.ID) && paymentIDPattern.MatchString(event.PaymentID) &&
		orderIDPattern.MatchString(event.OrderID) &&
		event.Method.Valid() && event.Outcome.Valid() && event.AmountCents >= 0 && event.Currency == "EUR"
}

// NewID erzeugt eine zufällige ID ohne personenbezogene oder Zahlungsdaten.
func NewID(prefix string) (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(data[:]), nil
}

// Sign erzeugt eine zeitgebundene HMAC-SHA256-Signatur über die unveränderten JSON-Bytes.
func Sign(secret []byte, body []byte, now time.Time) string {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	return fmt.Sprintf("t=%s,v1=%s", timestamp, hex.EncodeToString(signature(secret, timestamp, body)))
}

func signature(secret []byte, timestamp string, body []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}

// Verify akzeptiert nur Signaturen innerhalb von fünf Minuten und vergleicht konstant.
func Verify(secret []byte, header string, body []byte, now time.Time) error {
	if len(secret) < 32 || len(header) > 160 || len(body) == 0 {
		return ErrInvalidSignature
	}
	parts := strings.Split(header, ",")
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "t=") || !strings.HasPrefix(parts[1], "v1=") {
		return ErrInvalidSignature
	}
	timestampText := strings.TrimPrefix(parts[0], "t=")
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil || timestamp < now.Add(-5*time.Minute).Unix() || timestamp > now.Add(5*time.Minute).Unix() {
		return ErrInvalidSignature
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(parts[1], "v1="))
	if err != nil || len(provided) != sha256.Size {
		return ErrInvalidSignature
	}
	if !hmac.Equal(provided, signature(secret, timestampText, body)) {
		return ErrInvalidSignature
	}
	return nil
}
