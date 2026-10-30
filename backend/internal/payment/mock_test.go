package payment

import (
	"bytes"
	"testing"
	"time"
)

func TestMockMethodsAndEvents(t *testing.T) {
	for _, method := range []Method{DebitCard, Klarna, PayPal, BankTransfer} {
		if !method.Valid() {
			t.Fatalf("Zahlungsart %q fehlt", method)
		}
	}
	if Method("card_number").Valid() || Outcome("pending").Valid() {
		t.Fatal("Unbekannte Mock-Werte akzeptiert")
	}
	first, err := NewID("pay_")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewID("pay_")
	if err != nil || first == second || len(first) != 36 {
		t.Fatalf("Zufällige Zahlungs-IDs: %q %q %v", first, second, err)
	}
}

func TestMockWebhookSignature(t *testing.T) {
	secret := bytes.Repeat([]byte("s"), 32)
	now := time.Unix(1_800_000_000, 0)
	body := []byte(`{"id":"evt_1","outcome":"succeeded"}`)
	header := Sign(secret, body, now)
	if err := Verify(secret, header, body, now); err != nil {
		t.Fatalf("Gültige Signatur: %v", err)
	}
	for _, candidate := range []struct {
		name   string
		header string
		body   []byte
		now    time.Time
	}{
		{"fehlend", "", body, now},
		{"verändert", header, []byte(`{"id":"evt_1","outcome":"failed"}`), now},
		{"abgelaufen", header, body, now.Add(6 * time.Minute)},
		{"Zukunft", header, body, now.Add(-6 * time.Minute)},
		{"falsches Format", "t=abc,v1=xyz", body, now},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			if err := Verify(secret, candidate.header, candidate.body, candidate.now); err != ErrInvalidSignature {
				t.Fatalf("Erwarteter Signaturfehler, erhalten %v", err)
			}
		})
	}
}
