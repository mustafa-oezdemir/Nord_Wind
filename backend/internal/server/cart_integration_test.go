package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestCartAPIOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_CART_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_CART_TEST_URL für separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM cart_items`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Warenkorbtest benötigt leere Tabelle: %d, %v", count, err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"cart_items", "product_variants", "products", "auth_sessions", "auth_rate_limits", "customers"} {
			if _, err := db.ExecContext(context.Background(), "DELETE FROM "+table); err != nil {
				t.Errorf("Cleanup %s: %v", table, err)
			}
		}
		_ = db.Close()
	})
	products := catalog.NewRepository(db)
	product, err := products.CreateProduct(ctx, catalog.Product{Name: "Warenkorbprodukt", Slug: "warenkorbprodukt", Status: catalog.ProductPublished})
	if err != nil {
		t.Fatal(err)
	}
	variant, err := products.CreateVariant(ctx, catalog.ProductVariant{ProductID: product.ID, SKU: "NW-CART-01", Label: "Standard", PriceCents: 1234, StockQuantity: 10, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	RegisterCartRoutes(router, auth, cart.NewRepository(db))
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/cart", "", nil, ""), http.StatusUnauthorized, "UNAUTHENTICATED")
	password := "Synthetic-Cart-Passphrase-2026"
	login := func(email string) (*http.Cookie, string) {
		t.Helper()
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		if response := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); response.Code != http.StatusCreated {
			t.Fatalf("Registrierung: %d", response.Code)
		}
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login", body, nil, "")
		if response.Code != http.StatusOK {
			t.Fatalf("Login: %d", response.Code)
		}
		return sessionCookie(t, response), csrfFromResponse(t, response)
	}
	cookieA, csrfA := login("cart-a@example.invalid")
	cookieB, _ := login("cart-b@example.invalid")
	itemPath := "/api/v1/cart/items/" + variant.ID
	addBody := func(quantity int) string { return fmt.Sprintf(`{"variant_id":%q,"quantity":%d}`, variant.ID, quantity) }
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/cart/items", addBody(1), cookieA, ""), http.StatusForbidden, "FORBIDDEN")
	for _, quantity := range []int{0, -1, 100} {
		assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/cart/items", addBody(quantity), cookieA, csrfA), http.StatusBadRequest, "INVALID_REQUEST")
	}
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/cart/items", `{"variant_id":"00000000-0000-0000-0000-000000000000","quantity":1}`, cookieA, csrfA), http.StatusNotFound, "NOT_FOUND")
	read := func(cookie *http.Cookie) cart.Snapshot {
		t.Helper()
		response := authRequest(router, http.MethodGet, "/api/v1/cart", "", cookie, "")
		if response.Code != http.StatusOK {
			t.Fatalf("Warenkorb lesen: %d", response.Code)
		}
		var snapshot cart.Snapshot
		if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	for _, quantity := range []int{2, 3} {
		if response := authRequest(router, http.MethodPost, "/api/v1/cart/items", addBody(quantity), cookieA, csrfA); response.Code != http.StatusOK {
			t.Fatalf("Hinzufügen %d: %d %s", quantity, response.Code, response.Body.String())
		}
	}
	first := read(cookieA)
	if len(first.Items) != 1 || first.Items[0].Quantity != 5 || first.SubtotalCents != 6170 || !first.Items[0].Available {
		t.Fatalf("Zusammengeführte Variante: %+v", first)
	}
	if other := read(cookieB); len(other.Items) != 0 {
		t.Fatalf("Sitzungstrennung verletzt: %+v", other)
	}
	if response := authRequest(router, http.MethodPost, "/api/v1/auth/logout", "", cookieA, csrfA); response.Code != http.StatusNoContent {
		t.Fatalf("Logout: %d", response.Code)
	}
	newLogin := authRequest(router, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":"cart-a@example.invalid","password":%q}`, password), nil, "")
	if newLogin.Code != http.StatusOK {
		t.Fatalf("Wiederanmeldung: %d", newLogin.Code)
	}
	cookieA, csrfA = sessionCookie(t, newLogin), csrfFromResponse(t, newLogin)
	if persisted := read(cookieA); len(persisted.Items) != 1 || persisted.Items[0].Quantity != 5 {
		t.Fatalf("Warenkorb nach neuer Session verloren: %+v", persisted)
	}
	for _, quantity := range []string{`{"quantity":0}`, `{"quantity":-2}`} {
		assertProblem(t, authRequest(router, http.MethodPut, itemPath, quantity, cookieA, csrfA), http.StatusBadRequest, "INVALID_REQUEST")
	}
	if response := authRequest(router, http.MethodPut, itemPath, `{"quantity":4}`, cookieA, csrfA); response.Code != http.StatusOK {
		t.Fatalf("Menge setzen: %d", response.Code)
	}
	variant.StockQuantity = 2
	if _, err := products.UpdateVariant(ctx, variant); err != nil {
		t.Fatal(err)
	}
	changed := read(cookieA)
	if changed.Items[0].Available || changed.Items[0].Unavailable != "insufficient_stock" || changed.SubtotalCents != 0 {
		t.Fatalf("Bestandsänderung nicht sichtbar: %+v", changed)
	}
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/cart/items", addBody(1), cookieA, csrfA), http.StatusConflict, "CONFLICT")
	variant.StockQuantity = 10
	if _, err := products.UpdateVariant(ctx, variant); err != nil {
		t.Fatal(err)
	}
	product.Status = catalog.ProductArchived
	if _, err := products.UpdateProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if archived := read(cookieA); archived.Items[0].Unavailable != "product_unavailable" {
		t.Fatalf("Archiviertes Produkt wird angeboten: %+v", archived)
	}
	product.Status = catalog.ProductPublished
	if _, err := products.UpdateProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if response := authRequest(router, http.MethodDelete, itemPath, "", cookieA, csrfA); response.Code != http.StatusNoContent {
		t.Fatalf("Entfernen: %d", response.Code)
	}
	assertProblem(t, authRequest(router, http.MethodDelete, itemPath, "", cookieA, csrfA), http.StatusNotFound, "NOT_FOUND")
	if response := authRequest(router, http.MethodPost, "/api/v1/cart/items", addBody(1), cookieA, csrfA); response.Code != http.StatusOK {
		t.Fatalf("Erneut hinzufügen: %d", response.Code)
	}
	if err := products.DeleteVariant(ctx, variant.ID); err != nil {
		t.Fatal(err)
	}
	if missing := read(cookieA); len(missing.Items) != 0 {
		t.Fatalf("Gelöschte Variante blieb im Warenkorb: %+v", missing)
	}
}
