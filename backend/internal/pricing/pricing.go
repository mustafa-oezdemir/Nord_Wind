// Package pricing berechnet Warenkorb- und Bestellsummen nach ADR-007.
//
// Alle Beträge sind ganzzahlige Euro-Cent und Bruttobeträge (inklusive Umsatzsteuer).
// Das Paket hat keine Abhängigkeiten außer der Standardbibliothek und ist deterministisch,
// damit API, Bestellung und Oberfläche dieselben Summen zeigen.
package pricing

import (
	"errors"
	"math"
	"math/bits"
	"sort"
)

// TaxClass ordnet ein Produkt einem Umsatzsteuersatz zu.
type TaxClass string

const (
	// TaxStandard ist der Regelsatz nach § 12 Abs. 1 UStG.
	TaxStandard TaxClass = "standard"
	// TaxReduced ist der ermäßigte Satz nach § 12 Abs. 2 UStG (z. B. Bücher, Lebensmittel).
	TaxReduced TaxClass = "reduced"
)

// Steuersätze in Basispunkten (100 bp = 1 %), Stand 2026.
var taxRates = map[TaxClass]int64{TaxStandard: 1900, TaxReduced: 700}

// RateBP liefert den Steuersatz einer Steuerklasse in Basispunkten.
func RateBP(class TaxClass) (int64, bool) {
	rate, ok := taxRates[class]
	return rate, ok
}

// ValidTaxClass prüft, ob eine Steuerklasse bekannt ist.
func ValidTaxClass(class TaxClass) bool {
	_, ok := taxRates[class]
	return ok
}

// ShippingPolicy beschreibt die Versandkosten innerhalb Deutschlands.
type ShippingPolicy struct {
	// FlatCents sind die Versandkosten (brutto) unterhalb der Freigrenze.
	FlatCents int64
	// FreeFromCents ist der rabattierte Warenwert (brutto), ab dem der Versand kostenlos ist.
	// 0 bedeutet: keine Freigrenze.
	FreeFromCents int64
}

// DefaultShipping ist die vom Projektinhaber am 30. Oktober 2026 festgelegte Regel:
// 4,90 €, kostenlos ab 50,00 € Warenwert nach Rabatt.
var DefaultShipping = ShippingPolicy{FlatCents: 490, FreeFromCents: 5000}

// MaxDiscountBP ist der höchste zulässige Rabatt (100 %).
const MaxDiscountBP = 10000

// Line ist eine Warenkorb- oder Bestellposition mit Bruttostückpreis.
type Line struct {
	UnitGrossCents int64
	Quantity       int64
	TaxClass       TaxClass
}

// TaxLine ist die Aufschlüsselung je Steuersatz, wie sie auf der Rechnung erscheint.
type TaxLine struct {
	Class      TaxClass
	RateBP     int64
	GrossCents int64
	NetCents   int64
	VATCents   int64
}

// Quote ist das vollständige Rechenergebnis.
type Quote struct {
	GoodsCents    int64
	DiscountCents int64
	ShippingCents int64
	TotalCents    int64
	// MissingForFreeShippingCents ist der fehlende Warenwert bis zum kostenlosen Versand (0, wenn erreicht).
	MissingForFreeShippingCents int64
	Taxes                       []TaxLine
}

var (
	ErrInvalidLine     = errors.New("ungültige Preisposition")
	ErrInvalidDiscount = errors.New("ungültiger Rabatt")
	ErrInvalidShipping = errors.New("ungültige Versandregel")
	ErrOverflow        = errors.New("Betrag überschreitet den zulässigen Bereich")
)

// maxAmount begrenzt Zwischenwerte, damit alle Rechenschritte in int64 bleiben.
const maxAmount = math.MaxInt64 / 4

// Calculate berechnet Warenwert, Rabatt, Versand, Gesamtsumme und Umsatzsteuer.
//
// Regeln (ADR-007):
//  1. Positionssumme = Stückpreis × Menge, exakt in Cent.
//  2. Rabatt = Warenwert × Prozentsatz, kaufmännisch auf Cent gerundet und nach
//     Bruttowerten auf die Steuersätze verteilt (größter Rest).
//  3. Versand fällt an, sobald eine Position vorhanden ist und der rabattierte Warenwert
//     unter der Freigrenze liegt. Er wird als Nebenleistung im Verhältnis der rabattierten
//     Warenwerte auf die Steuersätze verteilt.
//  4. Umsatzsteuer je Satz = Brutto × Satz ÷ (100 % + Satz), kaufmännisch gerundet;
//     Netto = Brutto − Umsatzsteuer.
func Calculate(lines []Line, discountBP int64, shipping ShippingPolicy) (Quote, error) {
	quote := Quote{Taxes: []TaxLine{}}
	if discountBP < 0 || discountBP > MaxDiscountBP {
		return quote, ErrInvalidDiscount
	}
	if shipping.FlatCents < 0 || shipping.FreeFromCents < 0 || shipping.FlatCents > maxAmount || shipping.FreeFromCents > maxAmount {
		return quote, ErrInvalidShipping
	}
	if len(lines) == 0 {
		return quote, nil
	}

	groups := map[TaxClass]*group{}
	for _, line := range lines {
		rate, ok := RateBP(line.TaxClass)
		if !ok || line.Quantity < 1 || line.UnitGrossCents < 0 {
			return Quote{Taxes: []TaxLine{}}, ErrInvalidLine
		}
		hi, lineTotal := bits.Mul64(uint64(line.UnitGrossCents), uint64(line.Quantity))
		if hi != 0 || lineTotal > maxAmount {
			return Quote{Taxes: []TaxLine{}}, ErrOverflow
		}
		g := groups[line.TaxClass]
		if g == nil {
			g = &group{class: line.TaxClass, rate: rate}
			groups[line.TaxClass] = g
		}
		g.goods += int64(lineTotal)
		g.quantity += line.Quantity
		quote.GoodsCents += int64(lineTotal)
		if g.goods > maxAmount || quote.GoodsCents > maxAmount || g.quantity > maxAmount {
			return Quote{Taxes: []TaxLine{}}, ErrOverflow
		}
	}

	ordered := make([]*group, 0, len(groups))
	for _, g := range groups {
		ordered = append(ordered, g)
	}
	// Höherer Satz zuerst; so ist auch die Reihenfolge der Rechnungszeilen festgelegt.
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].rate > ordered[j].rate })

	quote.DiscountCents = roundedMulDiv(quote.GoodsCents, discountBP, 10000)
	discounts := allocate(quote.DiscountCents, weights(ordered, func(g *group) int64 { return g.goods }))
	discountedGoods := quote.GoodsCents - quote.DiscountCents
	for index, g := range ordered {
		g.discount = discounts[index]
	}

	if shipping.FreeFromCents > 0 && discountedGoods >= shipping.FreeFromCents {
		quote.ShippingCents = 0
	} else {
		quote.ShippingCents = shipping.FlatCents
		if shipping.FreeFromCents > 0 {
			quote.MissingForFreeShippingCents = shipping.FreeFromCents - discountedGoods
		}
	}
	shippingWeights := weights(ordered, func(g *group) int64 { return g.goods - g.discount })
	if sum(shippingWeights) == 0 {
		// Nur kostenlose oder vollständig rabattierte Waren: nach Stückzahl verteilen.
		shippingWeights = weights(ordered, func(g *group) int64 { return g.quantity })
	}
	shares := allocate(quote.ShippingCents, shippingWeights)

	for index, g := range ordered {
		gross := g.goods - g.discount + shares[index]
		vat := roundedMulDiv(gross, g.rate, 10000+g.rate)
		quote.Taxes = append(quote.Taxes, TaxLine{
			Class: g.class, RateBP: g.rate, GrossCents: gross, NetCents: gross - vat, VATCents: vat,
		})
	}
	quote.TotalCents = discountedGoods + quote.ShippingCents
	return quote, nil
}

type group struct {
	class    TaxClass
	rate     int64
	goods    int64
	quantity int64
	discount int64
}

func weights(groups []*group, value func(*group) int64) []int64 {
	result := make([]int64, len(groups))
	for index, g := range groups {
		result[index] = value(g)
	}
	return result
}

func sum(values []int64) int64 {
	var total int64
	for _, value := range values {
		total += value
	}
	return total
}

// allocate verteilt total proportional zu weights nach dem Verfahren des größten Rests.
// Die Teile sind nicht negativ und ergeben exakt total; bei gleichem Rest gewinnt der
// frühere Eintrag (höherer Steuersatz).
func allocate(total int64, weights []int64) []int64 {
	parts := make([]int64, len(weights))
	weightSum := sum(weights)
	if total == 0 || weightSum == 0 {
		if total > 0 && len(parts) > 0 {
			parts[0] = total
		}
		return parts
	}
	remainders := make([]uint64, len(weights))
	var assigned int64
	for index, weight := range weights {
		quotient, remainder := mulDiv(total, weight, weightSum)
		parts[index] = quotient
		remainders[index] = remainder
		assigned += quotient
	}
	order := make([]int, len(weights))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(i, j int) bool { return remainders[order[i]] > remainders[order[j]] })
	for left, position := total-assigned, 0; left > 0; left, position = left-1, position+1 {
		parts[order[position%len(order)]]++
	}
	return parts
}

// mulDiv rechnet a × b ÷ c mit 128-Bit-Zwischenergebnis; a, b ≥ 0 und c > 0.
func mulDiv(a, b, c int64) (int64, uint64) {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	quotient, remainder := bits.Div64(hi, lo, uint64(c))
	return int64(quotient), remainder
}

// roundedMulDiv rechnet a × b ÷ c, kaufmännisch gerundet (ab ,5 aufrunden).
func roundedMulDiv(a, b, c int64) int64 {
	quotient, remainder := mulDiv(a, b, c)
	if remainder*2 >= uint64(c) {
		quotient++
	}
	return quotient
}
