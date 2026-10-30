package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestAdminAPIOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_ADMIN_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_ADMIN_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Admin-Test verweigert Datenbank %q", name)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM customers`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Admin-Test benötigt leere Datenbank: %d, %v", count, err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"admin_audit_events", "product_categories", "product_variants", "products", "categories", "auth_sessions", "auth_rate_limits", "customers"} {
			if _, err := db.ExecContext(context.Background(), "DELETE FROM "+table); err != nil {
				t.Errorf("Cleanup %s: %v", table, err)
			}
		}
		_ = db.Close()
	})
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	RegisterAdminRoutes(router, auth, catalog.NewAdminRepository(db))
	RegisterCatalogRoutes(router, catalog.NewRepository(db))
	if got := authRequest(router, http.MethodGet, "/api/v1/admin/categories", "", nil, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Anonymer Adminzugriff: %d", got.Code)
	}
	password := "Admin-Test-Passphrase-2026"
	for _, email := range []string{"admin@example.invalid", "customer@example.invalid"} {
		body := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
		if got := authRequest(router, http.MethodPost, "/api/v1/auth/register", body, nil, ""); got.Code != http.StatusCreated {
			t.Fatalf("Registrierung %s: %d", email, got.Code)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE email = 'admin@example.invalid'`); err != nil {
		t.Fatal(err)
	}
	var adminID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM customers WHERE email = 'admin@example.invalid'`).Scan(&adminID); err != nil {
		t.Fatal(err)
	}
	customerLogin := authRequest(router, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":"customer@example.invalid","password":%q}`, password), nil, "")
	if customerLogin.Code != http.StatusOK {
		t.Fatalf("Kundenlogin: %d", customerLogin.Code)
	}
	customerCookie := sessionCookie(t, customerLogin)
	if got := authRequest(router, http.MethodGet, "/api/v1/admin/categories", "", customerCookie, ""); got.Code != http.StatusForbidden {
		t.Fatalf("Kundenrolle im Admin-API: %d", got.Code)
	}
	adminLogin := authRequest(router, http.MethodPost, "/api/v1/auth/login", fmt.Sprintf(`{"email":"admin@example.invalid","password":%q}`, password), nil, "")
	if adminLogin.Code != http.StatusOK {
		t.Fatalf("Adminlogin: %d", adminLogin.Code)
	}
	adminCookie := sessionCookie(t, adminLogin)
	csrf := csrfFromResponse(t, adminLogin)
	categoryBody := `{"name":"Technik","slug":"technik","status":"active","sort_order":0}`
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/categories", categoryBody, adminCookie, ""), http.StatusForbidden, "FORBIDDEN")
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/categories", `{"name":"","slug":"bad slug"}`, adminCookie, csrf), http.StatusBadRequest, "INVALID_REQUEST")
	category := authRequest(router, http.MethodPost, "/api/v1/admin/categories", categoryBody, adminCookie, csrf)
	if category.Code != http.StatusCreated {
		t.Fatalf("Kategorie: %d %s", category.Code, category.Body.String())
	}
	categoryID := idFromResponse(t, category)
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/categories", categoryBody, adminCookie, csrf), http.StatusConflict, "CONFLICT")
	child := authRequest(router, http.MethodPost, "/api/v1/admin/categories", fmt.Sprintf(`{"name":"Kameras","slug":"kameras","parent_id":%q}`, categoryID), adminCookie, csrf)
	if child.Code != http.StatusCreated {
		t.Fatalf("Unterkategorie: %d %s", child.Code, child.Body.String())
	}
	if !strings.Contains(authRequest(router, http.MethodGet, "/api/v1/admin/categories", "", adminCookie, "").Body.String(), `"parent_id":"`+categoryID+`"`) {
		t.Fatal("Kategoriebaum fehlt")
	}
	productBody := `{"name":"Kamera","slug":"kamera","status":"draft"}`
	product := authRequest(router, http.MethodPost, "/api/v1/admin/products", productBody, adminCookie, csrf)
	if product.Code != http.StatusCreated {
		t.Fatalf("Produkt: %d %s", product.Code, product.Body.String())
	}
	productID := idFromResponse(t, product)
	attach := authRequest(router, http.MethodPost, "/api/v1/admin/products/"+productID+"/categories", fmt.Sprintf(`{"category_id":%q}`, categoryID), adminCookie, csrf)
	if attach.Code != http.StatusNoContent {
		t.Fatalf("Kategoriezuordnung: %d", attach.Code)
	}
	invalidCategory := "00000000-0000-0000-0000-000000000000"
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/products/"+productID+"/categories", fmt.Sprintf(`{"category_id":%q}`, invalidCategory), adminCookie, csrf), http.StatusConflict, "CONFLICT")
	variantBody := `{"sku":"NW-CAMERA","label":"Standard","price_cents":12900,"stock_quantity":2,"is_active":true}`
	variant := authRequest(router, http.MethodPost, "/api/v1/admin/products/"+productID+"/variants", variantBody, adminCookie, csrf)
	if variant.Code != http.StatusCreated {
		t.Fatalf("Variante: %d %s", variant.Code, variant.Body.String())
	}
	variantID := idFromResponse(t, variant)
	assertProblem(t, authRequest(router, http.MethodPost, "/api/v1/admin/products/"+productID+"/variants", variantBody, adminCookie, csrf), http.StatusConflict, "CONFLICT")
	assertProblem(t, authRequest(router, http.MethodPut, "/api/v1/admin/variants/"+variantID, `{"sku":"NW-CAMERA","label":"Standard","price_cents":-1}`, adminCookie, csrf), http.StatusBadRequest, "INVALID_REQUEST")
	zero := authRequest(router, http.MethodPut, "/api/v1/admin/variants/"+variantID, `{"sku":"NW-CAMERA","label":"Standard","price_cents":0,"stock_quantity":0,"is_active":false}`, adminCookie, csrf)
	if zero.Code != http.StatusOK || !strings.Contains(zero.Body.String(), `"price_cents":0`) || !strings.Contains(zero.Body.String(), `"is_active":false`) {
		t.Fatalf("Nullwert-Update: %d %s", zero.Code, zero.Body.String())
	}
	active := authRequest(router, http.MethodPut, "/api/v1/admin/variants/"+variantID, `{"sku":"NW-CAMERA","label":"Standard","price_cents":0,"stock_quantity":0,"is_active":true}`, adminCookie, csrf)
	if active.Code != http.StatusOK {
		t.Fatalf("Variante aktivieren: %d", active.Code)
	}
	if got := authRequest(router, http.MethodPut, "/api/v1/admin/products/"+productID, `{"name":"Kamera","slug":"kamera","status":"published"}`, adminCookie, csrf); got.Code != http.StatusOK {
		t.Fatalf("Produkt veröffentlichen: %d %s", got.Code, got.Body.String())
	}
	if got := authRequest(router, http.MethodGet, "/api/v1/products", "", nil, ""); !strings.Contains(got.Body.String(), `"slug":"kamera"`) {
		t.Fatalf("Veröffentlichung fehlt im öffentlichen Katalog: %d %s", got.Code, got.Body.String())
	}
	deleteParent := authRequest(router, http.MethodDelete, "/api/v1/admin/categories/"+categoryID, "", adminCookie, csrf)
	assertProblem(t, deleteParent, http.StatusConflict, "CONFLICT")
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/products/invalid", "", adminCookie, ""), http.StatusBadRequest, "INVALID_REQUEST")
	assertProblem(t, authRequest(router, http.MethodGet, "/api/v1/admin/products/"+invalidCategory, "", adminCookie, ""), http.StatusNotFound, "NOT_FOUND")
	audit := authRequest(router, http.MethodGet, "/api/v1/admin/audit", "", adminCookie, "")
	if audit.Code != http.StatusOK {
		t.Fatalf("Audit: %d", audit.Code)
	}
	var auditBody struct {
		Data []catalog.AuditEvent `json:"data"`
	}
	if err := json.Unmarshal(audit.Body.Bytes(), &auditBody); err != nil {
		t.Fatal(err)
	}
	if len(auditBody.Data) != 8 {
		t.Fatalf("Audit enthält %d statt 8 erfolgreiche Änderungen", len(auditBody.Data))
	}
	for _, event := range auditBody.Data {
		if event.ActorCustomerID != adminID {
			t.Fatal("Falscher Audit-Akteur")
		}
		if event.EntityType == "product_category" && (event.RelatedID == nil || *event.RelatedID != categoryID) {
			t.Fatal("Audit-Zuordnung ohne Kategorie-ID")
		}
	}
	for _, path := range []string{
		"/api/v1/admin/variants/" + variantID,
		"/api/v1/admin/products/" + productID + "/categories/" + categoryID,
		"/api/v1/admin/products/" + productID,
		"/api/v1/admin/categories/" + idFromResponse(t, child),
		"/api/v1/admin/categories/" + categoryID,
	} {
		response := authRequest(router, http.MethodDelete, path, "", adminCookie, csrf)
		if response.Code != http.StatusNoContent {
			t.Fatalf("Löschen %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{
		"/api/v1/admin/variants/" + variantID,
		"/api/v1/admin/products/" + productID,
		"/api/v1/admin/categories/" + categoryID,
	} {
		assertProblem(t, authRequest(router, http.MethodGet, path, "", adminCookie, ""), http.StatusNotFound, "NOT_FOUND")
	}
	finalAudit := authRequest(router, http.MethodGet, "/api/v1/admin/audit", "", adminCookie, "")
	if err := json.Unmarshal(finalAudit.Body.Bytes(), &auditBody); err != nil || len(auditBody.Data) != 13 {
		t.Fatalf("Audit nach CRUD-Löschungen: %d Ereignisse, %v", len(auditBody.Data), err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'customer' WHERE id = $1`, adminID); err != nil {
		t.Fatal(err)
	}
	if got := authRequest(router, http.MethodGet, "/api/v1/admin/audit", "", adminCookie, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Rollenentzug ließ Admin-Session bestehen: %d", got.Code)
	}
}

func idFromResponse(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil || item.ID == "" {
		t.Fatalf("Antwort ohne ID: %v", err)
	}
	return item.ID
}
