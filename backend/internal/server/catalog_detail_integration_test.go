package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestProductDetailAPIOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_CATALOG_API_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_CATALOG_API_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("Testdatenbank konnte nicht geöffnet werden")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("API-Test verweigert Datenbank %q", name)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	assertCatalogEmpty(t, ctx, db)

	t.Run("Produktdetail mit Varianten, Bestand und Verkaufsstatus", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("Fixture-Rollback: %v", err)
			}
		})
		repo := catalog.NewRepository(tx)
		kitchen := testCategory(t, ctx, repo, catalog.Category{Name: "Küche", Slug: "kueche", SortOrder: 2})
		pans := testCategory(t, ctx, repo, catalog.Category{Name: "Pfannen", Slug: "pfannen", ParentID: &kitchen.ID, SortOrder: 1})
		hiddenCategory := testCategory(t, ctx, repo, catalog.Category{Name: "Alt", Slug: "alt", Status: catalog.CategoryInactive})

		pan := testProduct(t, ctx, repo, catalog.Product{Name: "Gusseisenpfanne", Slug: "Guss-Pfanne", Description: "Für Herd und Ofen.", Status: catalog.ProductPublished})
		soldOut := testProduct(t, ctx, repo, catalog.Product{Name: "Kupfertopf", Slug: "kupfertopf", Status: catalog.ProductPublished})
		testProduct(t, ctx, repo, catalog.Product{Name: "Ohne Variante", Slug: "ohne-variante", Status: catalog.ProductPublished})
		archived := testProduct(t, ctx, repo, catalog.Product{Name: "Alte Pfanne", Slug: "alte-pfanne", Status: catalog.ProductArchived})
		draft := testProduct(t, ctx, repo, catalog.Product{Name: "Entwurf", Slug: "entwurf"})
		for _, pair := range [][2]string{{pan.ID, pans.ID}, {pan.ID, kitchen.ID}, {pan.ID, hiddenCategory.ID}, {archived.ID, pans.ID}} {
			if err := repo.AddProductCategory(ctx, pair[0], pair[1]); err != nil {
				t.Fatal(err)
			}
		}
		for _, variant := range []catalog.ProductVariant{
			{ProductID: pan.ID, SKU: "NW-PFANNE-24", Label: "24 cm", PriceCents: 4990, StockQuantity: 0, IsActive: true},
			{ProductID: pan.ID, SKU: "NW-PFANNE-28", Label: "28 cm", PriceCents: 5990, StockQuantity: catalog.LowStockThreshold, IsActive: true},
			{ProductID: pan.ID, SKU: "NW-PFANNE-32", Label: "32 cm", PriceCents: 7490, StockQuantity: 40, IsActive: true},
			{ProductID: pan.ID, SKU: "NW-PFANNE-ALT", Label: "Alt", PriceCents: 100, StockQuantity: 9, IsActive: false},
			{ProductID: soldOut.ID, SKU: "NW-TOPF", Label: "Standard", PriceCents: 12900, StockQuantity: 0, IsActive: true},
			{ProductID: archived.ID, SKU: "NW-ALT", Label: "Standard", PriceCents: 1000, StockQuantity: 5, IsActive: true},
			{ProductID: draft.ID, SKU: "NW-ENTWURF", Label: "Standard", PriceCents: 1000, StockQuantity: 5, IsActive: true},
		} {
			if _, err := repo.CreateVariant(ctx, variant); err != nil {
				t.Fatal(err)
			}
		}
		router, err := New("NordWinds", db, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		RegisterCatalogRoutes(router, repo)

		detail := decodeProductDetail(t, getCatalog(t, router, "/api/v1/products/guss-pfanne"))
		if detail.Slug != "Guss-Pfanne" || detail.Name != "Gusseisenpfanne" || detail.SaleStatus != "available" {
			t.Fatalf("Produktkopf: %+v", detail)
		}
		if len(detail.Categories) != 2 || detail.Categories[0].Slug != "pfannen" || detail.Categories[1].Slug != "kueche" {
			t.Fatalf("Nur aktive Kategorien in Pflegereihenfolge erwartet: %+v", detail.Categories)
		}
		wantVariants := []variantOfferResponse{
			{SKU: "NW-PFANNE-24", Label: "24 cm", PriceCents: 4990, Currency: "EUR", StockStatus: "out_of_stock"},
			{SKU: "NW-PFANNE-28", Label: "28 cm", PriceCents: 5990, Currency: "EUR", StockStatus: "low_stock"},
			{SKU: "NW-PFANNE-32", Label: "32 cm", PriceCents: 7490, Currency: "EUR", StockStatus: "in_stock"},
		}
		if len(detail.Variants) != len(wantVariants) {
			t.Fatalf("Inaktive Variante veröffentlicht oder Variante fehlt: %+v", detail.Variants)
		}
		for index, want := range wantVariants {
			got := detail.Variants[index]
			want.ID = got.ID
			if got != want || got.ID == "" {
				t.Fatalf("Variante %d: %+v, erwartet %+v", index, got, want)
			}
		}
		raw := getCatalog(t, router, "/api/v1/products/guss-pfanne").Body.String()
		if strings.Contains(raw, "stock_quantity") {
			t.Fatalf("Exakte Lagermenge veröffentlicht: %s", raw)
		}

		for _, tc := range []struct{ slug, status string }{
			{"kupfertopf", "sold_out"}, {"ohne-variante", "unavailable"}, {"alte-pfanne", "discontinued"},
		} {
			result := decodeProductDetail(t, getCatalog(t, router, "/api/v1/products/"+tc.slug))
			if result.SaleStatus != tc.status {
				t.Fatalf("%s: Verkaufsstatus %q, erwartet %q", tc.slug, result.SaleStatus, tc.status)
			}
			if tc.status != "sold_out" && (result.Variants == nil || len(result.Variants) != 0) {
				t.Fatalf("%s: keine bestellbaren Varianten erwartet: %+v", tc.slug, result.Variants)
			}
		}
		if body := getCatalog(t, router, "/api/v1/products/ohne-variante").Body.String(); !strings.Contains(body, `"variants":[]`) {
			t.Fatalf("Leere Varianten müssen ein JSON-Array sein: %s", body)
		}

		for _, path := range []string{"/api/v1/products/entwurf", "/api/v1/products/unbekannt"} {
			assertProblem(t, getCatalog(t, router, path), 404, "NOT_FOUND")
		}
		for _, path := range []string{
			"/api/v1/products/guss-pfanne?preview=1",
			"/api/v1/products/" + strings.Repeat("a", 121),
			"/api/v1/products/guss%00pfanne",
			"/api/v1/products/%20",
		} {
			assertProblem(t, getCatalog(t, router, path), 400, "INVALID_REQUEST")
		}
	})
	assertCatalogEmpty(t, ctx, db)

	t.Run("Datenbankfehler im Produktdetail bleiben intern", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		router, err := New("NordWinds", db, discardLogger())
		if err != nil {
			t.Fatal(err)
		}
		RegisterCatalogRoutes(router, catalog.NewRepository(tx))
		response := getCatalog(t, router, "/api/v1/products/guss-pfanne")
		assertProblem(t, response, 500, "INTERNAL_ERROR")
		body := strings.ToLower(response.Body.String())
		if strings.Contains(body, "sql") || strings.Contains(body, "transaction") {
			t.Fatal("Interner Datenbankfehler wurde im HTTP-Body veröffentlicht")
		}
	})
}

func decodeProductDetail(t *testing.T, response *httptest.ResponseRecorder) productDetailResponse {
	t.Helper()
	if response.Code != 200 || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Produktdetail: Status %d, Content-Type %q, Body %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	var body productDetailResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
