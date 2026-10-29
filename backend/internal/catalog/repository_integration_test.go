package catalog

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestRepositoryPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_CATALOG_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_CATALOG_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("PostgreSQL-Testdatenbank konnte nicht geöffnet werden")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("PostgreSQL-Testdatenbank ist nicht erreichbar")
	}
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Repository-Test verweigert Datenbank %q", name)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatalf("Migrationen auf separater Testdatenbank: %v", err)
	}
	assertEmptyCatalog(t, ctx, db)

	t.Run("CRUD und Beziehungen", func(t *testing.T) {
		repo := transactionalRepository(t, ctx, db)
		parent := mustCategory(t, ctx, repo, Category{Name: "Technik", Slug: "technik"})
		child := mustCategory(t, ctx, repo, Category{
			ParentID: &parent.ID, Name: "Kameras", Slug: "kameras", SortOrder: 2,
		})
		unlinkedCategory := mustCategory(t, ctx, repo, Category{Name: "Bücher", Slug: "buecher"})
		loadedChild, err := repo.GetCategory(ctx, child.ID)
		if err != nil || loadedChild.ParentID == nil || *loadedChild.ParentID != parent.ID ||
			loadedChild.Status != CategoryActive {
			t.Fatalf("Kategoriebeziehung falsch: %+v, %v", loadedChild, err)
		}
		child.Name = "Digitalkameras"
		child.Status = CategoryInactive
		child, err = repo.UpdateCategory(ctx, child)
		if err != nil {
			t.Fatal(err)
		}
		loadedChild, err = repo.GetCategory(ctx, child.ID)
		if err != nil || loadedChild.Name != "Digitalkameras" || loadedChild.Status != CategoryInactive {
			t.Fatalf("Kategorie-Update nicht gespeichert: %+v, %v", loadedChild, err)
		}

		product := mustProduct(t, ctx, repo, Product{Name: "Kamera", Slug: "kamera"})
		otherProduct := mustProduct(t, ctx, repo, Product{Name: "Buch", Slug: "buch"})
		if product.Status != ProductDraft {
			t.Fatalf("Produktstatus: %q", product.Status)
		}
		product.Description = "Testgerät"
		product.Status = ProductPublished
		product, err = repo.UpdateProduct(ctx, product)
		if err != nil {
			t.Fatal(err)
		}
		loadedProduct, err := repo.GetProduct(ctx, product.ID)
		if err != nil || loadedProduct.Description != "Testgerät" || loadedProduct.Status != ProductPublished {
			t.Fatalf("Produkt-Update nicht gespeichert: %+v, %v", loadedProduct, err)
		}
		if err := repo.AddProductCategory(ctx, product.ID, parent.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.AddProductCategory(ctx, product.ID, child.ID); err != nil {
			t.Fatal(err)
		}
		categories, err := repo.ListProductCategories(ctx, product.ID)
		if err != nil || len(categories) != 2 || categories[0].ID != parent.ID || categories[1].ID != child.ID {
			t.Fatalf("Produktkategorien falsch: %+v, %v", categories, err)
		}

		first := mustVariant(t, ctx, repo, ProductVariant{
			ProductID: product.ID, SKU: "NW-CAM-1", Label: "Standard",
			PriceCents: 12999, StockQuantity: 3, IsActive: true,
		})
		second := mustVariant(t, ctx, repo, ProductVariant{
			ProductID: product.ID, SKU: "NW-CAM-2", Label: "Premium",
			PriceCents: 16999, StockQuantity: 1, IsActive: true,
		})
		otherVariant := mustVariant(t, ctx, repo, ProductVariant{
			ProductID: otherProduct.ID, SKU: "NW-BOOK-1", Label: "Ausgabe",
			PriceCents: 999, StockQuantity: 4, IsActive: true,
		})
		if first.Currency != "EUR" {
			t.Fatalf("Variantenwährung: %q", first.Currency)
		}
		first.PriceCents = 13999
		first.StockQuantity = 2
		first.IsActive = false
		first, err = repo.UpdateVariant(ctx, first)
		if err != nil {
			t.Fatal(err)
		}
		loadedVariant, err := repo.GetVariant(ctx, first.ID)
		if err != nil || loadedVariant.PriceCents != 13999 || loadedVariant.StockQuantity != 2 || loadedVariant.IsActive {
			t.Fatalf("Varianten-Update nicht gespeichert: %+v, %v", loadedVariant, err)
		}
		variants, err := repo.ListProductVariants(ctx, product.ID)
		if err != nil || len(variants) != 2 || !containsVariant(variants, first.ID) || !containsVariant(variants, second.ID) {
			t.Fatalf("Produktvarianten falsch: %+v, %v", variants, err)
		}

		if err := repo.DeleteVariant(ctx, first.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteVariant(ctx, second.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteVariant(ctx, otherVariant.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetVariant(ctx, first.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Gelöschte Variante: %v", err)
		}
		if _, err := repo.UpdateVariant(ctx, first); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Update einer gelöschten Variante: %v", err)
		}
		if err := repo.RemoveProductCategory(ctx, product.ID, child.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.RemoveProductCategory(ctx, product.ID, parent.ID); err != nil {
			t.Fatal(err)
		}
		categories, err = repo.ListProductCategories(ctx, product.ID)
		if err != nil || len(categories) != 0 {
			t.Fatalf("Entfernte Kategorien noch verbunden: %+v, %v", categories, err)
		}
		if err := repo.RemoveProductCategory(ctx, product.ID, child.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Erneutes Entfernen der Kategorie: %v", err)
		}
		if err := repo.DeleteProduct(ctx, product.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteProduct(ctx, otherProduct.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetProduct(ctx, product.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Gelöschtes Produkt: %v", err)
		}
		if _, err := repo.UpdateProduct(ctx, product); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Update eines gelöschten Produkts: %v", err)
		}
		if err := repo.DeleteCategory(ctx, child.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteCategory(ctx, parent.ID); err != nil {
			t.Fatal(err)
		}
		if err := repo.DeleteCategory(ctx, unlinkedCategory.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.GetCategory(ctx, parent.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Gelöschte Kategorie: %v", err)
		}
		if _, err := repo.UpdateCategory(ctx, parent); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Update einer gelöschten Kategorie: %v", err)
		}
		if err := repo.DeleteVariant(ctx, first.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Fehlendes Delete muss ErrNotFound liefern: %v", err)
		}
	})
	assertEmptyCatalog(t, ctx, db)

	t.Run("Datenbankfehler", func(t *testing.T) {
		cases := []struct {
			name string
			code string
			run  func(*testing.T, *Repository) error
		}{
			{"SKU ohne Beachtung der Großschreibung eindeutig", "23505", func(t *testing.T, r *Repository) error {
				p := mustProduct(t, ctx, r, Product{Name: "Kamera", Slug: "kamera"})
				mustVariant(t, ctx, r, ProductVariant{ProductID: p.ID, SKU: "NW-1", Label: "A", PriceCents: 100})
				_, err := r.CreateVariant(ctx, ProductVariant{ProductID: p.ID, SKU: "nw-1", Label: "B", PriceCents: 100})
				return err
			}},
			{"Negativer Preis", "23514", func(t *testing.T, r *Repository) error {
				p := mustProduct(t, ctx, r, Product{Name: "Kamera", Slug: "kamera"})
				_, err := r.CreateVariant(ctx, ProductVariant{ProductID: p.ID, SKU: "NW-NEG", Label: "A", PriceCents: -1})
				return err
			}},
			{"Negativer Bestand", "23514", func(t *testing.T, r *Repository) error {
				p := mustProduct(t, ctx, r, Product{Name: "Kamera", Slug: "kamera"})
				_, err := r.CreateVariant(ctx, ProductVariant{ProductID: p.ID, SKU: "NW-NEG", Label: "A", PriceCents: 100, StockQuantity: -1})
				return err
			}},
			{"Ungültiger Produkt-FK", "23503", func(t *testing.T, r *Repository) error {
				_, err := r.CreateVariant(ctx, ProductVariant{ProductID: "00000000-0000-0000-0000-000000000000", SKU: "NW-ORPHAN", Label: "A", PriceCents: 100})
				return err
			}},
			{"Doppelte Produktkategorie", "23505", func(t *testing.T, r *Repository) error {
				p := mustProduct(t, ctx, r, Product{Name: "Kamera", Slug: "kamera"})
				c := mustCategory(t, ctx, r, Category{Name: "Technik", Slug: "technik"})
				if err := r.AddProductCategory(ctx, p.ID, c.ID); err != nil {
					t.Fatal(err)
				}
				return r.AddProductCategory(ctx, p.ID, c.ID)
			}},
			{"Referenzierte Kategorie nicht löschbar", "23001", func(t *testing.T, r *Repository) error {
				p := mustProduct(t, ctx, r, Product{Name: "Kamera", Slug: "kamera"})
				c := mustCategory(t, ctx, r, Category{Name: "Technik", Slug: "technik"})
				if err := r.AddProductCategory(ctx, p.ID, c.ID); err != nil {
					t.Fatal(err)
				}
				return r.DeleteCategory(ctx, c.ID)
			}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo := transactionalRepository(t, ctx, db)
				assertSQLState(t, tc.run(t, repo), tc.code)
			})
			assertEmptyCatalog(t, ctx, db)
		}
	})
	assertEmptyCatalog(t, ctx, db)
}

func transactionalRepository(t *testing.T, ctx context.Context, db *sql.DB) *Repository {
	t.Helper()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("Fixture-Transaktion konnte nicht zurückgesetzt werden: %v", err)
		}
	})
	return NewRepository(tx)
}

func mustCategory(t *testing.T, ctx context.Context, r *Repository, input Category) Category {
	t.Helper()
	category, err := r.CreateCategory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return category
}

func mustProduct(t *testing.T, ctx context.Context, r *Repository, input Product) Product {
	t.Helper()
	product, err := r.CreateProduct(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return product
}

func mustVariant(t *testing.T, ctx context.Context, r *Repository, input ProductVariant) ProductVariant {
	t.Helper()
	variant, err := r.CreateVariant(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return variant
}

func containsVariant(variants []ProductVariant, id string) bool {
	for _, variant := range variants {
		if variant.ID == id {
			return true
		}
	}
	return false
}

func assertSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("SQLSTATE %s erwartet, erhalten: %v", want, err)
	}
}

func assertEmptyCatalog(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	for _, table := range []string{"categories", "products", "product_categories", "product_variants"} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("Testdatenbank enthält %d Datensätze in %s", count, table)
		}
	}
}
