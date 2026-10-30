package catalog

import "testing"

func TestStockStatusHidesExactQuantity(t *testing.T) {
	for _, tc := range []struct {
		quantity int32
		want     StockStatus
	}{
		{-1, StockOutOfStock},
		{0, StockOutOfStock},
		{1, StockLow},
		{LowStockThreshold, StockLow},
		{LowStockThreshold + 1, StockInStock},
		{10000, StockInStock},
	} {
		if got := stockStatus(tc.quantity); got != tc.want {
			t.Errorf("stockStatus(%d) = %q, erwartet %q", tc.quantity, got, tc.want)
		}
	}
}

func TestSaleStatus(t *testing.T) {
	inStock := VariantOffer{StockStatus: StockInStock}
	low := VariantOffer{StockStatus: StockLow}
	empty := VariantOffer{StockStatus: StockOutOfStock}
	for _, tc := range []struct {
		name     string
		status   ProductStatus
		variants []VariantOffer
		want     SaleStatus
	}{
		{"verfügbar", ProductPublished, []VariantOffer{empty, low}, SaleAvailable},
		{"ausverkauft", ProductPublished, []VariantOffer{empty, empty}, SaleSoldOut},
		{"ohne aktive Variante", ProductPublished, nil, SaleUnavailable},
		{"archiviert", ProductArchived, []VariantOffer{inStock}, SaleDiscontinued},
	} {
		if got := saleStatus(tc.status, tc.variants); got != tc.want {
			t.Errorf("%s: %q, erwartet %q", tc.name, got, tc.want)
		}
	}
}
