package orders

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Address ist die Liefer- und Rechnungsadresse einer Bestellung (ADR-008: eine Adresse, nur
// Deutschland). Sie wird beim Kauf als Snapshot gespeichert und danach nicht mehr verändert.
type Address struct {
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Company      string `json:"company"`
	Street       string `json:"street"`
	HouseNumber  string `json:"house_number"`
	AddressExtra string `json:"address_extra"`
	PostalCode   string `json:"postal_code"`
	City         string `json:"city"`
	Country      string `json:"country"`
}

// FieldError benennt ein ungültiges Feld mit einem stabilen Code, ohne den Wert zu wiederholen.
type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

var (
	postalCodePattern  = regexp.MustCompile(`^(0[1-9]|[1-9][0-9])[0-9]{3}$`)
	houseNumberPattern = regexp.MustCompile(`^[0-9]{1,4} ?[A-Za-z]?( ?[-/] ?[0-9]{1,4} ?[A-Za-z]?)?$`)
)

// Normalize entfernt Leerraum am Rand und schreibt das Länderkürzel groß.
func (a Address) Normalize() Address {
	trim := strings.TrimSpace
	return Address{
		FirstName: trim(a.FirstName), LastName: trim(a.LastName), Company: trim(a.Company),
		Street: trim(a.Street), HouseNumber: trim(a.HouseNumber), AddressExtra: trim(a.AddressExtra),
		PostalCode: trim(a.PostalCode), City: trim(a.City), Country: strings.ToUpper(trim(a.Country)),
	}
}

// Validate prüft eine normalisierte Adresse und liefert alle Fehler in Feldreihenfolge.
func (a Address) Validate() []FieldError {
	errs := []FieldError{}
	text := func(field, value string, required bool, minimum, maximum int) {
		length := utf8.RuneCountInString(value)
		switch {
		case value == "" && required:
			errs = append(errs, FieldError{field, "required"})
		case value == "":
		case !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0:
			errs = append(errs, FieldError{field, "invalid_format"})
		case length > maximum:
			errs = append(errs, FieldError{field, "too_long"})
		case length < minimum:
			errs = append(errs, FieldError{field, "too_short"})
		}
	}
	pattern := func(field, value string, re *regexp.Regexp) {
		switch {
		case value == "":
			errs = append(errs, FieldError{field, "required"})
		case !re.MatchString(value):
			errs = append(errs, FieldError{field, "invalid_format"})
		}
	}
	text("first_name", a.FirstName, true, 1, 100)
	text("last_name", a.LastName, true, 1, 100)
	text("company", a.Company, false, 1, 100)
	text("street", a.Street, true, 2, 100)
	pattern("house_number", a.HouseNumber, houseNumberPattern)
	text("address_extra", a.AddressExtra, false, 1, 100)
	pattern("postal_code", a.PostalCode, postalCodePattern)
	text("city", a.City, true, 2, 100)
	switch a.Country {
	case "DE":
	case "":
		errs = append(errs, FieldError{"country", "required"})
	default:
		errs = append(errs, FieldError{"country", "unsupported_country"})
	}
	return errs
}
