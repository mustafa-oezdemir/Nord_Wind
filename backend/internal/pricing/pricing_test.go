package pricing

import (
	"errors"
	"math"
	"testing"
)

func std(unit, quantity int64) Line {
	return Line{UnitGrossCents: unit, Quantity: quantity, TaxClass: TaxStandard}
}
func red(unit, quantity int64) Line {
	return Line{UnitGrossCents: unit, Quantity: quantity, TaxClass: TaxReduced}
}

func TestCalculateExamples(t *testing.T) {
	for _, tc := range []struct {
		name                                      string
		lines                                     []Line
		discountBP                                int64
		goods, discount, shipping, total, missing int64
		taxes                                     []TaxLine
	}{
		{
			name: "leerer Warenkorb ohne Versand", lines: nil,
			taxes: []TaxLine{},
		},
		{
			name: "Regelsatz über der Freigrenze", lines: []Line{std(5990, 1)},
			goods: 5990, total: 5990,
			taxes: []TaxLine{{TaxStandard, 1900, 5990, 5034, 956}},
		},
		{
			name: "unter der Freigrenze mit Versand", lines: []Line{std(1999, 1)},
			goods: 1999, shipping: 490, total: 2489, missing: 3001,
			taxes: []TaxLine{{TaxStandard, 1900, 2489, 2092, 397}},
		},
		{
			name: "genau an der Freigrenze versandkostenfrei", lines: []Line{std(2500, 2)},
			goods: 5000, total: 5000,
			taxes: []TaxLine{{TaxStandard, 1900, 5000, 4202, 798}},
		},
		{
			name: "ein Cent unter der Freigrenze", lines: []Line{std(4999, 1)},
			goods: 4999, shipping: 490, total: 5489, missing: 1,
			taxes: []TaxLine{{TaxStandard, 1900, 5489, 4613, 876}},
		},
		{
			name: "gemischte Sätze: Versand anteilig, Gleichstand an den höheren Satz", lines: []Line{std(3000, 1), red(1000, 1)},
			goods: 4000, shipping: 490, total: 4490, missing: 1000,
			taxes: []TaxLine{{TaxStandard, 1900, 3368, 2830, 538}, {TaxReduced, 700, 1122, 1049, 73}},
		},
		{
			name: "10 % Rabatt auf gemischte Sätze erreicht Freigrenze", lines: []Line{std(2000, 2), red(1000, 2)}, discountBP: 1000,
			goods: 6000, discount: 600, total: 5400,
			taxes: []TaxLine{{TaxStandard, 1900, 3600, 3025, 575}, {TaxReduced, 700, 1800, 1682, 118}},
		},
		{
			name: "Rabatt senkt Warenwert unter die Freigrenze", lines: []Line{std(5500, 1)}, discountBP: 1000,
			goods: 5500, discount: 550, shipping: 490, total: 5440, missing: 50,
			taxes: []TaxLine{{TaxStandard, 1900, 5440, 4571, 869}},
		},
		{
			name: "Rabatt kaufmännisch gerundet (100,5 → 101)", lines: []Line{std(1005, 1)}, discountBP: 1000,
			goods: 1005, discount: 101, shipping: 490, total: 1394, missing: 4096,
			taxes: []TaxLine{{TaxStandard, 1900, 1394, 1171, 223}},
		},
		{
			name: "Menge und krummer Prozentsatz (15 % von 9,99 = 1,4985 → 1,50)", lines: []Line{std(333, 3)}, discountBP: 1500,
			goods: 999, discount: 150, shipping: 490, total: 1339, missing: 4151,
			taxes: []TaxLine{{TaxStandard, 1900, 1339, 1125, 214}},
		},
		{
			name: "100 % Rabatt: Versand bleibt", lines: []Line{std(1000, 2)}, discountBP: MaxDiscountBP,
			goods: 2000, discount: 2000, shipping: 490, total: 490, missing: 5000,
			taxes: []TaxLine{{TaxStandard, 1900, 490, 412, 78}},
		},
		{
			name: "kostenlose Ware verteilt Versand nach Stückzahl", lines: []Line{std(0, 1), red(0, 3)},
			goods: 0, shipping: 490, total: 490, missing: 5000,
			taxes: []TaxLine{{TaxStandard, 1900, 123, 103, 20}, {TaxReduced, 700, 367, 343, 24}},
		},
		{
			name: "mehrere Positionen desselben Satzes werden zusammengefasst", lines: []Line{red(1299, 2), red(899, 1), std(100, 1)},
			goods: 3597, shipping: 490, total: 4087, missing: 1403,
			taxes: []TaxLine{{TaxStandard, 1900, 114, 96, 18}, {TaxReduced, 700, 3973, 3713, 260}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quote, err := Calculate(tc.lines, tc.discountBP, DefaultShipping)
			if err != nil {
				t.Fatal(err)
			}
			if quote.GoodsCents != tc.goods || quote.DiscountCents != tc.discount || quote.ShippingCents != tc.shipping ||
				quote.TotalCents != tc.total || quote.MissingForFreeShippingCents != tc.missing {
				t.Fatalf("Summen: %+v", quote)
			}
			if len(quote.Taxes) != len(tc.taxes) {
				t.Fatalf("Steuerzeilen: %+v, erwartet %+v", quote.Taxes, tc.taxes)
			}
			for index, want := range tc.taxes {
				if quote.Taxes[index] != want {
					t.Fatalf("Steuerzeile %d: %+v, erwartet %+v", index, quote.Taxes[index], want)
				}
			}
			assertInvariants(t, quote)
		})
	}
}

func TestCalculateRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lines    []Line
		discount int64
		shipping ShippingPolicy
		want     error
	}{
		{"Menge 0", []Line{std(100, 0)}, 0, DefaultShipping, ErrInvalidLine},
		{"negative Menge", []Line{std(100, -1)}, 0, DefaultShipping, ErrInvalidLine},
		{"negativer Preis", []Line{std(-1, 1)}, 0, DefaultShipping, ErrInvalidLine},
		{"unbekannte Steuerklasse", []Line{{UnitGrossCents: 100, Quantity: 1, TaxClass: "luxus"}}, 0, DefaultShipping, ErrInvalidLine},
		{"Rabatt über 100 %", []Line{std(100, 1)}, MaxDiscountBP + 1, DefaultShipping, ErrInvalidDiscount},
		{"negativer Rabatt", []Line{std(100, 1)}, -1, DefaultShipping, ErrInvalidDiscount},
		{"negative Versandkosten", []Line{std(100, 1)}, 0, ShippingPolicy{FlatCents: -1}, ErrInvalidShipping},
		{"Positionsüberlauf", []Line{std(math.MaxInt64/2, 3)}, 0, DefaultShipping, ErrOverflow},
		{"Summenüberlauf", []Line{std(math.MaxInt64/8, 1), std(math.MaxInt64/8, 1), std(math.MaxInt64/8, 1)}, 0, DefaultShipping, ErrOverflow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Calculate(tc.lines, tc.discount, tc.shipping); !errors.Is(err, tc.want) {
				t.Fatalf("Fehler %v, erwartet %v", err, tc.want)
			}
		})
	}
}

// Über viele Kombinationen gilt: Summen gehen auf, Netto + Steuer = Brutto, keine negativen Teile.
func TestCalculateInvariantsAcrossCombinations(t *testing.T) {
	prices := []int64{0, 1, 5, 99, 333, 1005, 1999, 4999, 5000, 12345, 129900}
	quantities := []int64{1, 2, 3, 7, 99}
	discounts := []int64{0, 1, 333, 1000, 1500, 2500, 9999, MaxDiscountBP}
	policies := []ShippingPolicy{DefaultShipping, {FlatCents: 0}, {FlatCents: 599}, {FlatCents: 490, FreeFromCents: 1}}
	count := 0
	for _, a := range prices {
		for _, b := range prices {
			for _, qa := range quantities {
				for _, discount := range discounts {
					for _, policy := range policies {
						lines := []Line{std(a, qa), red(b, 3)}
						quote, err := Calculate(lines, discount, policy)
						if err != nil {
							t.Fatal(err)
						}
						assertInvariants(t, quote)
						if quote.DiscountCents > quote.GoodsCents {
							t.Fatalf("Rabatt größer als Warenwert: %+v", quote)
						}
						again, _ := Calculate(lines, discount, policy)
						if again.TotalCents != quote.TotalCents || again.Taxes[0] != quote.Taxes[0] {
							t.Fatal("Berechnung ist nicht deterministisch")
						}
						count++
					}
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("keine Kombination geprüft")
	}
}

func TestAllocateLargestRemainder(t *testing.T) {
	for _, tc := range []struct {
		total   int64
		weights []int64
		want    []int64
	}{
		{100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{2, []int64{1, 1, 1}, []int64{1, 1, 0}},
		{10, []int64{0, 5}, []int64{0, 10}},
		{7, []int64{0, 0}, []int64{7, 0}},
		{0, []int64{3, 4}, []int64{0, 0}},
	} {
		got := allocate(tc.total, tc.weights)
		for index := range tc.want {
			if got[index] != tc.want[index] {
				t.Fatalf("allocate(%d, %v) = %v, erwartet %v", tc.total, tc.weights, got, tc.want)
			}
		}
	}
}

func TestTaxRates(t *testing.T) {
	if rate, ok := RateBP(TaxStandard); !ok || rate != 1900 {
		t.Fatalf("Regelsatz %d", rate)
	}
	if rate, ok := RateBP(TaxReduced); !ok || rate != 700 {
		t.Fatalf("ermäßigter Satz %d", rate)
	}
	if ValidTaxClass("null") {
		t.Fatal("unbekannte Steuerklasse akzeptiert")
	}
}

func assertInvariants(t *testing.T, quote Quote) {
	t.Helper()
	var gross int64
	for index, tax := range quote.Taxes {
		if tax.NetCents+tax.VATCents != tax.GrossCents || tax.NetCents < 0 || tax.VATCents < 0 {
			t.Fatalf("Steuerzeile geht nicht auf: %+v", tax)
		}
		if index > 0 && quote.Taxes[index-1].RateBP <= tax.RateBP {
			t.Fatalf("Steuerzeilen nicht nach Satz absteigend: %+v", quote.Taxes)
		}
		gross += tax.GrossCents
	}
	if len(quote.Taxes) > 0 && gross != quote.TotalCents {
		t.Fatalf("Steuerzeilen %d ≠ Gesamtsumme %d", gross, quote.TotalCents)
	}
	if quote.TotalCents != quote.GoodsCents-quote.DiscountCents+quote.ShippingCents || quote.DiscountCents < 0 || quote.ShippingCents < 0 {
		t.Fatalf("Gesamtsumme geht nicht auf: %+v", quote)
	}
}
