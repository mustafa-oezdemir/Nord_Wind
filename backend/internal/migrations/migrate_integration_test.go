package migrations

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
)

func TestUpDownUpOnIsolatedPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_MIGRATION_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_MIGRATION_TEST_URL für eine leere, separate Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("Testdatenbank konnte nicht geöffnet werden")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("Separate Testdatenbank ist nicht erreichbar")
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(databaseName, "nordwinds_test_") {
		t.Fatalf("Down-Test verweigert Datenbank %q", databaseName)
	}
	var existingTables int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public'`).Scan(&existingTables); err != nil {
		t.Fatal(err)
	}
	if existingTables != 0 {
		t.Fatal("Down-Test verlangt eine leere, separate Datenbank")
	}

	if err := Up(ctx, db); err != nil {
		t.Fatalf("Erstes Up: %v", err)
	}
	if err := Check(ctx, db); err != nil {
		t.Fatalf("Status nach Up: %v", err)
	}
	assertSearchIndexCount(t, ctx, db, 5)
	if err := Up(ctx, db); err != nil {
		t.Fatalf("Wiederholtes Up muss idempotent sein: %v", err)
	}

	categoryID := insertID(t, ctx, db, `INSERT INTO categories (name, slug) VALUES ('Elektronik', 'elektronik') RETURNING id`)
	childID := insertID(t, ctx, db, `INSERT INTO categories (parent_id, name, slug) VALUES ($1, 'Telefone', 'telefone') RETURNING id`, categoryID)
	productID := insertID(t, ctx, db, `INSERT INTO products (name, slug) VALUES ('Telefon', 'telefon') RETURNING id`)
	mustExec(t, ctx, db, `INSERT INTO product_categories (product_id, category_id) VALUES ($1, $2)`, productID, childID)
	variantID := insertID(t, ctx, db, `INSERT INTO product_variants (product_id, sku, label, price_cents, stock_quantity) VALUES ($1, 'NW-SKU-1', 'Standard', 100, 2) RETURNING id`, productID)
	customerID := insertID(t, ctx, db, `INSERT INTO customers (email) VALUES ('test@example.invalid') RETURNING id`)
	orderID := insertID(t, ctx, db, `INSERT INTO orders (order_number, customer_id, total_cents, shipping_snapshot) VALUES ('TEST-1', $1, 100, '{}'::jsonb) RETURNING id`, customerID)
	mustExec(t, ctx, db, `INSERT INTO order_items (order_id, variant_id, name_snapshot, sku_snapshot, quantity, unit_price_cents, tax_snapshot) VALUES ($1, $2, 'Telefon', 'NW-SKU-1', 1, 100, '{}'::jsonb)`, orderID, variantID)
	mustExec(t, ctx, db, `UPDATE product_variants SET price_cents = 150 WHERE id = $1`, variantID)
	var historicalPrice int64
	if err := db.QueryRowContext(ctx, `SELECT unit_price_cents FROM order_items WHERE order_id = $1`, orderID).Scan(&historicalPrice); err != nil || historicalPrice != 100 {
		t.Fatalf("Bestell-Snapshot änderte sich nach Katalogpreisänderung: %d, %v", historicalPrice, err)
	}

	assertSQLState(t, ctx, db, `INSERT INTO product_variants (product_id, sku, label, price_cents) VALUES ($1, 'nw-sku-1', 'Duplikat', 100)`, "23505", productID)
	assertSQLState(t, ctx, db, `INSERT INTO product_variants (product_id, sku, label, price_cents) VALUES ($1, 'NW-NEG-P', 'Negativ', -1)`, "23514", productID)
	assertSQLState(t, ctx, db, `INSERT INTO product_variants (product_id, sku, label, price_cents, stock_quantity) VALUES ($1, 'NW-NEG-S', 'Negativ', 100, -1)`, "23514", productID)
	assertSQLState(t, ctx, db, `INSERT INTO product_variants (product_id, sku, label, price_cents) VALUES ('00000000-0000-0000-0000-000000000000', 'NW-BAD-FK', 'Ungültig', 100)`, "23503")
	assertSQLState(t, ctx, db, `INSERT INTO products (name, slug) VALUES ('Duplikat', 'TELEFON')`, "23505")
	assertSQLState(t, ctx, db, `UPDATE categories SET parent_id = $1 WHERE id = $2`, "23514", childID, categoryID)
	assertSQLState(t, ctx, db, `INSERT INTO order_items (order_id, variant_id, name_snapshot, sku_snapshot, quantity, unit_price_cents, tax_snapshot) VALUES ($1, $2, 'Telefon', 'NW-SKU-1', 0, 100, '{}'::jsonb)`, "23514", orderID, variantID)
	assertSQLState(t, ctx, db, `INSERT INTO order_items (order_id, variant_id, name_snapshot, sku_snapshot, quantity, unit_price_cents, tax_snapshot) VALUES ('00000000-0000-0000-0000-000000000000', $1, 'Telefon', 'NW-SKU-1', 1, 100, '{}'::jsonb)`, "23503", variantID)
	mustExec(t, ctx, db, `INSERT INTO product_images (product_id, alt_text, width, height, image_bytes) VALUES ($1, 'Testbild', 1, 1, $2)`, productID, []byte{1})
	if err := Down(ctx, db); err == nil {
		t.Fatal("Bild-Down darf vorhandene Bilder nicht entfernen")
	}
	mustExec(t, ctx, db, `DELETE FROM product_images`)
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 6 mit Domain-Daten: %v", err)
	}

	mustExec(t, ctx, db, `INSERT INTO admin_audit_events (actor_customer_id, entity_type, entity_id, action)
		VALUES ($1, 'category', $2, 'update')`, customerID, categoryID)
	if err := Down(ctx, db); err == nil {
		t.Fatal("Audit-Down darf vorhandene Ereignisse nicht entfernen")
	}
	mustExec(t, ctx, db, `DELETE FROM admin_audit_events`)
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 5 mit Domain-Daten: %v", err)
	}
	mustExec(t, ctx, db, `UPDATE customers SET password_hash = $1 WHERE id = $2`, strings.Repeat("x", 60), customerID)
	if err := Down(ctx, db); err == nil {
		t.Fatal("Identity-Down darf vorhandene Passwort-Hashes nicht entfernen")
	}
	mustExec(t, ctx, db, `UPDATE customers SET password_hash = NULL WHERE id = $1`, customerID)
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 4 mit Domain-Daten: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down der Suchindizes mit Domain-Daten: %v", err)
	}
	assertSearchIndexCount(t, ctx, db, 0)
	if err := Down(ctx, db); err == nil {
		t.Fatal("Down darf vorhandene Domain-Daten nicht löschen")
	}
	if applied, err := appliedVersions(ctx, db); err != nil || applied != 2 {
		t.Fatalf("Fehlgeschlagenes Domain-Down änderte den Versionsstand: %d, %v", applied, err)
	}
	if err := Up(ctx, db); err != nil {
		t.Fatalf("Suchindizes erneut anlegen: %v", err)
	}
	assertSearchIndexCount(t, ctx, db, 5)
	if err := Check(ctx, db); err != nil {
		t.Fatalf("Status nach Wiederherstellung der Suchindizes: %v", err)
	}

	for _, table := range []string{"order_items", "orders", "customers", "product_variants", "product_categories", "products", "categories"} {
		mustExec(t, ctx, db, "DELETE FROM "+table)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 6: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 5: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 4: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 3: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 2: %v", err)
	}
	if err := Check(ctx, db); err == nil {
		t.Fatal("Check muss die fehlende Version 2 erkennen")
	}
	if err := Up(ctx, db); err != nil {
		t.Fatalf("Zweites Up: %v", err)
	}
	if err := Check(ctx, db); err != nil {
		t.Fatalf("Status nach Up/Down/Up: %v", err)
	}

	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 6 vor Bootstrap-Test: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 5 vor Bootstrap-Test: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 4 vor Bootstrap-Test: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 3 vor Bootstrap-Test: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 2 vor Bootstrap-Test: %v", err)
	}
	mustExec(t, ctx, db, `INSERT INTO app_settings (key, value) VALUES ('test', 'wert')`)
	if err := Down(ctx, db); err == nil {
		t.Fatal("Bootstrap-Down darf app_settings-Daten nicht löschen")
	}
	mustExec(t, ctx, db, `DELETE FROM app_settings`)
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 1: %v", err)
	}
	legacy, err := files.ReadFile("sql/0001_bootstrap.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, ctx, db, string(legacy))
	mustExec(t, ctx, db, `INSERT INTO schema_migrations (version) VALUES (1)`)
	mustExec(t, ctx, db, `INSERT INTO app_settings (key, value) VALUES ('legacy', 'bleibt')`)
	if err := Up(ctx, db); err != nil {
		t.Fatalf("Upgrade von Version 1: %v", err)
	}
	if err := Check(ctx, db); err != nil {
		t.Fatalf("Status nach Upgrade: %v", err)
	}
	var legacyValue string
	if err := db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'legacy'`).Scan(&legacyValue); err != nil || legacyValue != "bleibt" {
		t.Fatalf("Bestehender Wert ging beim Upgrade verloren: %q, %v", legacyValue, err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 6 mit bestehendem app_settings: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 5 mit bestehendem app_settings: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 4 mit bestehendem app_settings: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 3 mit bestehendem app_settings: %v", err)
	}
	if err := Down(ctx, db); err != nil {
		t.Fatalf("Down von Version 2 mit bestehendem app_settings: %v", err)
	}
	if err := Down(ctx, db); err == nil {
		t.Fatal("Bootstrap-Down darf bestehende Einstellungen nicht löschen")
	}
	mustExec(t, ctx, db, `DELETE FROM app_settings`)
	if err := Down(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Up(ctx, db); err != nil {
		t.Fatalf("Up nach vollständigem Down: %v", err)
	}
	if err := Check(ctx, db); err != nil {
		t.Fatalf("Endstatus: %v", err)
	}
}

func insertID(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var id string
	if err := db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func mustExec(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(ctx, query, args...); err != nil {
		t.Fatal(err)
	}
}

func assertSQLState(t *testing.T, ctx context.Context, db *sql.DB, query, wantCode string, args ...any) {
	t.Helper()
	_, err := db.ExecContext(ctx, query, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != wantCode {
		t.Fatalf("SQLSTATE %s erwartet, erhalten: %v", wantCode, err)
	}
}

func assertSearchIndexCount(t *testing.T, ctx context.Context, db *sql.DB, want int) {
	t.Helper()
	var count int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname = 'public'
		AND indexname IN ('categories_name_trgm_idx', 'categories_description_trgm_idx',
		'products_name_trgm_idx', 'products_description_trgm_idx', 'product_variants_sku_trgm_idx')`).Scan(&count)
	if err != nil || count != want {
		t.Fatalf("Suchindizes: %d statt %d, Fehler %v", count, want, err)
	}
}
