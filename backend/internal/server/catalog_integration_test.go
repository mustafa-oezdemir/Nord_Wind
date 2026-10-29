package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestCatalogAPIOnPostgres(t *testing.T) {
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
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("Testdatenbank ist nicht erreichbar")
	}
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

	t.Run("Filter, Suche, Sortierung und Seitennummerierung", func(t *testing.T) {
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
		root := testCategory(t, ctx, repo, catalog.Category{Name: "Technik", Slug: "technik"})
		cameras := testCategory(t, ctx, repo, catalog.Category{Name: "Kameras", Slug: "kameras", ParentID: &root.ID})
		coffee := testCategory(t, ctx, repo, catalog.Category{Name: "Kaffee", Slug: "kaffee", ParentID: &root.ID})
		testCategory(t, ctx, repo, catalog.Category{Name: "Käse", Slug: "kaese"})
		testCategory(t, ctx, repo, catalog.Category{Name: "Archiv", Slug: "archiv", ParentID: &root.ID, Status: catalog.CategoryInactive})
		izmir := testProduct(t, ctx, repo, catalog.Product{Name: "İzmir Kamera", Slug: "izmir-kamera", Status: catalog.ProductPublished})
		cheese := testProduct(t, ctx, repo, catalog.Product{Name: "Käse-Kamera", Slug: "kaese-kamera", Status: catalog.ProductPublished})
		turkish := testProduct(t, ctx, repo, catalog.Product{Name: "Türkischer Kaffee", Slug: "tuerkischer-kaffee", Status: catalog.ProductPublished})
		draft := testProduct(t, ctx, repo, catalog.Product{Name: "Entwurf", Slug: "entwurf"})
		testProduct(t, ctx, repo, catalog.Product{Name: "Ohne Variante", Slug: "ohne-variante", Status: catalog.ProductPublished})
		hidden := testProduct(t, ctx, repo, catalog.Product{Name: "Unsichtbar", Slug: "unsichtbar", Status: catalog.ProductPublished})
		for _, pair := range [][2]string{{izmir.ID, cameras.ID}, {cheese.ID, root.ID}, {turkish.ID, coffee.ID}, {draft.ID, root.ID}, {hidden.ID, root.ID}} {
			if err := repo.AddProductCategory(ctx, pair[0], pair[1]); err != nil {
				t.Fatal(err)
			}
		}
		for _, variant := range []catalog.ProductVariant{
			{ProductID: izmir.ID, SKU: "NW-IZMIR", Label: "Standard", PriceCents: 25000, StockQuantity: 2, IsActive: true},
			{ProductID: cheese.ID, SKU: "NW-KAESE", Label: "Standard", PriceCents: 15000, StockQuantity: 1, IsActive: true},
			{ProductID: turkish.ID, SKU: "NW-CAFE", Label: "Standard", PriceCents: 8000, StockQuantity: 0, IsActive: true},
			{ProductID: draft.ID, SKU: "NW-DRAFT", Label: "Standard", PriceCents: 100, IsActive: true},
			{ProductID: hidden.ID, SKU: "NW-HIDDEN", Label: "Standard", PriceCents: 100, IsActive: false},
		} {
			if _, err := repo.CreateVariant(ctx, variant); err != nil {
				t.Fatal(err)
			}
		}
		router, err := New("NordWinds", db)
		if err != nil {
			t.Fatal(err)
		}
		RegisterCatalogRoutes(router, repo)

		for _, path := range []string{"/api/v1/products", "/api/v1/products?q=", "/api/v1/products?q=+++"} {
			response := getCatalog(t, router, path)
			page := decodeProducts(t, response, 200)
			if page.Pagination.Total != 3 || page.Pagination.TotalPages != 1 || len(page.Data) != 3 {
				t.Fatalf("Leere Suche %s: %+v", path, page)
			}
		}
		for _, tc := range []struct{ query, slug string }{
			{"İzmir", "izmir-kamera"}, {"izmir", "izmir-kamera"}, {"käse", "kaese-kamera"}, {"Türk", "tuerkischer-kaffee"}, {"NW-IZMIR", "izmir-kamera"},
		} {
			response := getCatalog(t, router, "/api/v1/products?q="+url.QueryEscape(tc.query))
			page := decodeProducts(t, response, 200)
			if page.Pagination.Total != 1 || len(page.Data) != 1 || page.Data[0].Slug != tc.slug {
				t.Fatalf("Suche %q: %+v", tc.query, page)
			}
		}
		filtered := decodeProducts(t, getCatalog(t, router, "/api/v1/products?category=technik&sort=price_asc&page=2&limit=2"), 200)
		if filtered.Pagination.Total != 3 || filtered.Pagination.TotalPages != 2 || filtered.Pagination.Page != 2 ||
			len(filtered.Data) != 1 || filtered.Data[0].Slug != "izmir-kamera" || filtered.Data[0].PriceCents != 25000 {
			t.Fatalf("Kategorie-/Preisfilter und zweite Seite: %+v", filtered)
		}
		firstPage := decodeProducts(t, getCatalog(t, router, "/api/v1/products?category=technik&sort=price_asc&limit=2"), 200)
		if len(firstPage.Data) != 2 || firstPage.Data[0].Slug != "tuerkischer-kaffee" || firstPage.Data[0].Available ||
			firstPage.Data[1].Slug != "kaese-kamera" {
			t.Fatalf("Preisreihenfolge oder Verfügbarkeit: %+v", firstPage)
		}
		outOfRange := decodeProducts(t, getCatalog(t, router, "/api/v1/products?page=9&limit=2"), 200)
		if outOfRange.Pagination.Total != 3 || len(outOfRange.Data) != 0 || outOfRange.Data == nil {
			t.Fatalf("Leere Seite: %+v", outOfRange)
		}
		literalWildcard := decodeProducts(t, getCatalog(t, router, "/api/v1/products?q=%25"), 200)
		if literalWildcard.Pagination.Total != 0 || len(literalWildcard.Data) != 0 {
			t.Fatalf("Prozentzeichen wurde als Wildcard behandelt: %+v", literalWildcard)
		}
		children := decodeCategories(t, getCatalog(t, router, "/api/v1/categories?category=technik&sort=name_asc"), 200)
		if children.Pagination.Total != 2 || len(children.Data) != 2 ||
			children.Data[0].Slug != "kaffee" || children.Data[1].Slug != "kameras" {
			t.Fatalf("Direkte Unterkategorien: %+v", children)
		}
		searchedCategory := decodeCategories(t, getCatalog(t, router, "/api/v1/categories?q="+url.QueryEscape("Kä")), 200)
		if searchedCategory.Pagination.Total != 1 || len(searchedCategory.Data) != 1 || searchedCategory.Data[0].Slug != "kaese" {
			t.Fatalf("Kategorie-Suche filterte nicht: %+v", searchedCategory)
		}
		allCategories := decodeCategories(t, getCatalog(t, router, "/api/v1/categories"), 200)
		if allCategories.Pagination.Total != 4 || len(allCategories.Data) != 4 {
			t.Fatalf("Inaktive Kategorie veröffentlicht: %+v", allCategories)
		}
		for _, path := range []string{"/api/v1/products?category=unbekannt", "/api/v1/categories?category=unbekannt", "/api/v1/products?category=archiv"} {
			assertProblem(t, getCatalog(t, router, path), 404, "NOT_FOUND")
		}
		for _, path := range []string{
			"/api/v1/products?limit=0", "/api/v1/products?limit=101", "/api/v1/products?limit=abc",
			"/api/v1/products?page=0", "/api/v1/products?page=10001",
			"/api/v1/products?sort=" + url.QueryEscape("price_asc;DROP TABLE products"),
			"/api/v1/categories?sort=" + url.QueryEscape("name_asc;DROP TABLE categories"),
			"/api/v1/products?limit=2&limit=3", "/api/v1/products?category=", "/api/v1/products?unknown=x",
			"/api/v1/products?q=%00", "/api/v1/products?q=" + strings.Repeat("a", 101),
		} {
			assertProblem(t, getCatalog(t, router, path), 400, "INVALID_REQUEST")
		}
		if page := decodeProducts(t, getCatalog(t, router, "/api/v1/products?sort=price_desc"), 200); page.Pagination.Total != 3 {
			t.Fatalf("Abgewiesene Sortier-Eingabe beschädigte Katalog: %+v", page)
		}
	})
	assertCatalogEmpty(t, ctx, db)

	t.Run("Datenbankfehler bleiben intern", func(t *testing.T) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		router, err := New("NordWinds", db)
		if err != nil {
			t.Fatal(err)
		}
		RegisterCatalogRoutes(router, catalog.NewRepository(tx))
		response := getCatalog(t, router, "/api/v1/products")
		assertProblem(t, response, 500, "INTERNAL_ERROR")
		if strings.Contains(strings.ToLower(response.Body.String()), "sql") || strings.Contains(strings.ToLower(response.Body.String()), "transaction") {
			t.Fatal("Interner Datenbankfehler wurde im HTTP-Body veröffentlicht")
		}
	})
}

func testCategory(t *testing.T, ctx context.Context, repo *catalog.Repository, category catalog.Category) catalog.Category {
	t.Helper()
	result, err := repo.CreateCategory(ctx, category)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func testProduct(t *testing.T, ctx context.Context, repo *catalog.Repository, product catalog.Product) catalog.Product {
	t.Helper()
	result, err := repo.CreateProduct(ctx, product)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func getCatalog(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func decodeProducts(t *testing.T, response *httptest.ResponseRecorder, status int) listResponse[productResponse] {
	t.Helper()
	if response.Code != status || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Produktantwort: Status %d, Content-Type %q, Body %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	var body listResponse[productResponse]
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func decodeCategories(t *testing.T, response *httptest.ResponseRecorder, status int) listResponse[categoryResponse] {
	t.Helper()
	if response.Code != status || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("Kategorieantwort: Status %d, Content-Type %q, Body %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	var body listResponse[categoryResponse]
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertProblem(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatalf("Problemantwort: Status %d, Content-Type %q, Body %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Status != status || problem.Code != code {
		t.Fatalf("Problemformat: %+v", problem)
	}
}

func assertCatalogEmpty(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, table := range []string{"categories", "products", "product_variants", "product_categories"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("Testdatenbank enthält %d Datensätze in %s", count, table)
		}
	}
}
