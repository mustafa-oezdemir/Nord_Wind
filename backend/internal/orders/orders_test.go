package orders

import (
	"bytes"
	"strings"
	"testing"
)

func validAddress() Address {
	return Address{FirstName: "Erika", LastName: "Musterfrau", Street: "Heidestraße", HouseNumber: "17",
		PostalCode: "51147", City: "Köln", Country: "DE"}
}

func TestAddressNormalizeAndValid(t *testing.T) {
	raw := Address{FirstName: "  Erika ", LastName: "Musterfrau\t", Street: " Heidestraße", HouseNumber: "17 a",
		PostalCode: " 01067 ", City: "Dresden ", Country: " de "}
	normalized := raw.Normalize()
	if normalized.FirstName != "Erika" || normalized.Country != "DE" || normalized.PostalCode != "01067" || normalized.City != "Dresden" {
		t.Fatalf("Normalisierung: %+v", normalized)
	}
	if errs := normalized.Validate(); len(errs) != 0 {
		t.Fatalf("gültige Adresse abgelehnt: %+v", errs)
	}
	for _, house := range []string{"1", "17a", "17 a", "5-7", "10/12", "2b - 4"} {
		address := validAddress()
		address.HouseNumber = house
		if errs := address.Validate(); len(errs) != 0 {
			t.Fatalf("Hausnummer %q abgelehnt: %+v", house, errs)
		}
	}
}

func TestAddressValidationErrors(t *testing.T) {
	if errs := (Address{}).Validate(); len(errs) != 7 {
		t.Fatalf("leere Adresse: %+v", errs)
	}
	for _, tc := range []struct {
		name   string
		change func(*Address)
		field  string
		code   string
	}{
		{"Vorname fehlt", func(a *Address) { a.FirstName = "" }, "first_name", "required"},
		{"Nachname zu lang", func(a *Address) { a.LastName = strings.Repeat("ä", 101) }, "last_name", "too_long"},
		{"Firma zu lang", func(a *Address) { a.Company = strings.Repeat("x", 101) }, "company", "too_long"},
		{"Straße zu kurz", func(a *Address) { a.Street = "X" }, "street", "too_short"},
		{"Hausnummer ohne Ziffer", func(a *Address) { a.HouseNumber = "a" }, "house_number", "invalid_format"},
		{"Hausnummer zu lang", func(a *Address) { a.HouseNumber = "12345" }, "house_number", "invalid_format"},
		{"Steuerzeichen", func(a *Address) { a.City = "Köln\nBonn" }, "city", "invalid_format"},
		{"PLZ vierstellig", func(a *Address) { a.PostalCode = "1234" }, "postal_code", "invalid_format"},
		{"PLZ beginnt mit 00", func(a *Address) { a.PostalCode = "00123" }, "postal_code", "invalid_format"},
		{"PLZ mit Buchstaben", func(a *Address) { a.PostalCode = "5114A" }, "postal_code", "invalid_format"},
		{"Land fehlt", func(a *Address) { a.Country = "" }, "country", "required"},
		{"Österreich", func(a *Address) { a.Country = "AT" }, "country", "unsupported_country"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			address := validAddress()
			tc.change(&address)
			errs := address.Validate()
			if len(errs) != 1 || errs[0].Field != tc.field || errs[0].Code != tc.code {
				t.Fatalf("Fehler %+v, erwartet %s/%s", errs, tc.field, tc.code)
			}
		})
	}
}

func TestStatusTransitions(t *testing.T) {
	allowed := map[[2]Status]bool{
		{Created, PaymentPending}: true, {Created, Cancelled}: true,
		{PaymentPending, Paid}: true, {PaymentPending, PaymentFailed}: true, {PaymentPending, Cancelled}: true,
		{PaymentFailed, PaymentPending}: true, {PaymentFailed, Cancelled}: true,
		{Paid, Processing}: true, {Processing, Shipped}: true, {Shipped, Delivered}: true,
	}
	all := []Status{Created, PaymentPending, Paid, Processing, Shipped, Delivered, Cancelled, PaymentFailed}
	for _, from := range all {
		for _, to := range all {
			if CanTransition(from, to) != allowed[[2]Status{from, to}] {
				t.Errorf("CanTransition(%s, %s) = %v", from, to, CanTransition(from, to))
			}
		}
	}
}

func TestKeysAndNumbers(t *testing.T) {
	for key, want := range map[string]bool{
		"6f1c2d3e-4b5a-4c6d-8e9f-0a1b2c3d4e5f": true, "abcdefghijklmnop": true,
		"kurz": false, "": false, strings.Repeat("a", 65): false, "mit leerzeichen-1234": false, "ümlaut-1234567890": false,
	} {
		if ValidIdempotencyKey(key) != want {
			t.Errorf("ValidIdempotencyKey(%q) != %v", key, want)
		}
	}
	for number, want := range map[string]bool{"NW-2026-100001": true, "NW-2026-1": false, "nw-2026-100001": false, "NW-2026-100001x": false} {
		if ValidOrderNumber(number) != want {
			t.Errorf("ValidOrderNumber(%q) != %v", number, want)
		}
	}
}

func TestRequestHash(t *testing.T) {
	base := CreateInput{CustomerID: "a", IdempotencyKey: "k1-0000000000000", Address: validAddress(), ExpectedTotalCents: 4490}
	same := base
	same.CustomerID, same.IdempotencyKey = "b", "k2-0000000000000"
	if !bytes.Equal(base.RequestHash(), same.RequestHash()) || len(base.RequestHash()) != 32 {
		t.Fatal("Fingerabdruck hängt nur von Adresse und Summe ab")
	}
	otherTotal := base
	otherTotal.ExpectedTotalCents = 4491
	otherAddress := base
	otherAddress.Address.City = "Bonn"
	if bytes.Equal(base.RequestHash(), otherTotal.RequestHash()) || bytes.Equal(base.RequestHash(), otherAddress.RequestHash()) {
		t.Fatal("geänderte Anfrage hat denselben Fingerabdruck")
	}
}
