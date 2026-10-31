package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

// TestBusinessMetricsOnPostgres prüft die aus PostgreSQL gelesenen Kennzahlen gegen
// eine leere Testdatenbank: Konten, Outbox, Bestand und Pool-Werte.
func TestBusinessMetricsOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_METRICS_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_METRICS_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	gin.SetMode(gin.TestMode)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var existing int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM customers`).Scan(&existing); err != nil || existing != 0 {
		t.Fatalf("Leere Testdatenbank erforderlich: %d, %v", existing, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`TRUNCATE notification_outbox, product_variants, products, auth_sessions, auth_rate_limits, customers CASCADE`); err != nil {
			t.Errorf("Cleanup: %v", err)
		}
		_ = db.Close()
	})

	token := strings.Repeat("b", 32)
	router, err := New("NordWinds", db, discardLogger(), token)
	if err != nil {
		t.Fatal(err)
	}
	empty := scrape(t, router, token)
	for series, want := range map[string]float64{
		`nordwinds_business_metrics_up`:                            1,
		`nordwinds_orders{status="PAID"}`:                          0,
		`nordwinds_order_value_euros{status="DELIVERED"}`:          0,
		`nordwinds_customer_accounts{state="active"}`:              0,
		`nordwinds_notification_outbox_entries{state="failed"}`:    0,
		`nordwinds_notification_outbox_oldest_pending_age_seconds`: 0,
		`nordwinds_catalog_variants_out_of_stock`:                  0,
	} {
		if got, ok := empty[series]; !ok || got != want {
			t.Errorf("leer: %s = %v (%v), erwartet %v", series, got, ok, want)
		}
	}

	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	// Die Registrierung reiht über den Trigger eine Begrüßung ein (ADR-011).
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	// Zur Laufzeit erzeugt, damit kein Passwortwert im Repository steht.
	if _, err := service.Register(ctx, "metrik-kundin@example.invalid", base64.RawURLEncoding.EncodeToString(raw), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE notification_outbox SET created_at = now() - interval '10 minutes'`); err != nil {
		t.Fatal(err)
	}
	products := catalog.NewRepository(db)
	product, err := products.CreateProduct(ctx, catalog.Product{Name: "Leuchtturm", Slug: "leuchtturm", Status: catalog.ProductPublished})
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []catalog.ProductVariant{
		{ProductID: product.ID, SKU: "NW-LT-1", Label: "Klein", PriceCents: 1999, StockQuantity: 0, IsActive: true},
		{ProductID: product.ID, SKU: "NW-LT-2", Label: "Groß", PriceCents: 2999, StockQuantity: 4, IsActive: true},
		{ProductID: product.ID, SKU: "NW-LT-3", Label: "Alt", PriceCents: 999, StockQuantity: 0, IsActive: false},
	} {
		if _, err := products.CreateVariant(ctx, variant); err != nil {
			t.Fatal(err)
		}
	}

	filled := scrape(t, router, token)
	for series, want := range map[string]float64{
		`nordwinds_customer_accounts{state="active"}`:            1,
		`nordwinds_notification_outbox_entries{state="pending"}`: 1,
		`nordwinds_catalog_variants_out_of_stock`:                1,
	} {
		if got := filled[series]; got != want {
			t.Errorf("gefüllt: %s = %v, erwartet %v", series, got, want)
		}
	}
	if age := filled[`nordwinds_notification_outbox_oldest_pending_age_seconds`]; age < 590 || age > 3600 {
		t.Errorf("Alter der ältesten E-Mail: %v", age)
	}
	if filled[`nordwinds_db_pool_open_connections`] < 1 {
		t.Error("Pool-Werte fehlen")
	}
	for series := range filled {
		if strings.Contains(series, "metrik-kundin") || strings.Contains(series, "NW-LT") || strings.Contains(series, "leuchtturm") {
			t.Fatalf("Kunden- oder Produktdaten in Metrik: %s", series)
		}
	}

	// Ohne erreichbare Datenbank bleibt der Abruf erfolgreich und meldet up = 0.
	broken, err := sql.Open("pgx", "postgres://nordwinds:falsch@127.0.0.1:1/nordwinds_test_offline?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	offlineRouter, err := New("NordWinds", broken, discardLogger(), token)
	if err != nil {
		t.Fatal(err)
	}
	offline := scrape(t, offlineRouter, token)
	if offline[`nordwinds_business_metrics_up`] != 0 {
		t.Fatal("Datenbankausfall nicht als up = 0 gemeldet")
	}
	if _, ok := offline[`nordwinds_orders{status="PAID"}`]; ok {
		t.Fatal("Geschäftswerte trotz Ausfall gemeldet")
	}
}
