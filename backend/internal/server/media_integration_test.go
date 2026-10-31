package server

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestProductMediaOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_MEDIA_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_MEDIA_TEST_URL für separate PostgreSQL-Testdatenbank setzen")
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
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM product_images`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Bildtest benötigt leere Tabelle: %d, %v", count, err)
	}
	t.Cleanup(func() {
		for _, table := range []string{"admin_audit_events", "product_images", "product_variants", "products", "auth_sessions", "auth_rate_limits", "customers"} {
			if _, err := db.ExecContext(context.Background(), "DELETE FROM "+table); err != nil {
				t.Errorf("Cleanup %s: %v", table, err)
			}
		}
		_ = db.Close()
	})
	repository := catalog.NewRepository(db)
	product, err := repository.CreateProduct(ctx, catalog.Product{Name: "Testbildprodukt", Slug: "testbildprodukt", Status: catalog.ProductDraft})
	if err != nil {
		t.Fatal(err)
	}
	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Register(ctx, "media-admin@example.invalid", "Synthetic-Media-Passphrase-2026", "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE email = 'media-admin@example.invalid'`); err != nil {
		t.Fatal(err)
	}
	_, token, session, err := service.Login(ctx, "media-admin@example.invalid", "Synthetic-Media-Passphrase-2026", "test", "")
	if err != nil {
		t.Fatal(err)
	}
	cookie := &http.Cookie{Name: "nordwinds_session", Value: token}
	router, err := New("NordWinds", db, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	auth := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid"})
	RegisterAdminRoutes(router, auth, catalog.NewAdminRepository(db))
	RegisterCatalogRoutes(router, repository)
	RegisterMediaRoutes(router, repository)
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/admin/products/" + product.ID + "/images"
	requestImage := func(data []byte, declared string, authenticated bool, csrf string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreatePart(textproto.MIMEHeader{
			"Content-Disposition": {`form-data; name="image"; filename="../../evil.php"`},
			"Content-Type":        {declared},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.WriteField("alt", "Produkt von vorne"); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, path, &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("X-CSRF-Token", csrf)
		if authenticated {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	assertProblem(t, requestImage(source.Bytes(), "image/png", false, session.CSRFToken), http.StatusUnauthorized, "UNAUTHENTICATED")
	assertProblem(t, requestImage(source.Bytes(), "image/png", true, ""), http.StatusForbidden, "FORBIDDEN")
	assertProblem(t, requestImage([]byte("<script>evil()</script>"), "image/png", true, session.CSRFToken), http.StatusBadRequest, "INVALID_REQUEST")
	assertProblem(t, requestImage(source.Bytes(), "image/jpeg", true, session.CSRFToken), http.StatusBadRequest, "INVALID_REQUEST")
	assertProblem(t, requestImage(make([]byte, maxImageBytes+1), "image/png", true, session.CSRFToken), http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE")
	upload := requestImage(source.Bytes(), "image/png", true, session.CSRFToken)
	if upload.Code != http.StatusCreated {
		t.Fatalf("Bild-Upload: %d %s", upload.Code, upload.Body.String())
	}
	id := idFromResponse(t, upload)
	mediaPath := "/api/v1/media/products/" + id
	if response := authRequest(router, http.MethodGet, mediaPath, "", nil, ""); response.Code != http.StatusNotFound {
		t.Fatalf("Entwurfsbild öffentlich: %d", response.Code)
	}
	product.Status = catalog.ProductPublished
	if _, err := repository.UpdateProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateVariant(ctx, catalog.ProductVariant{ProductID: product.ID, SKU: "NW-MEDIA-01", Label: "Standard", PriceCents: 100, StockQuantity: 1, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	served := authRequest(router, http.MethodGet, mediaPath, "", nil, "")
	if served.Code != http.StatusOK || served.Header().Get("X-Content-Type-Options") != "nosniff" ||
		http.DetectContentType(served.Body.Bytes()) != "image/jpeg" || strings.Contains(served.Body.String(), "evil.php") {
		t.Fatalf("Öffentliches Bild ungültig: %d", served.Code)
	}
	detail := authRequest(router, http.MethodGet, "/api/v1/products/testbildprodukt", "", nil, "")
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), mediaPath) {
		t.Fatalf("Produktdetail ohne Bild: %d %s", detail.Code, detail.Body.String())
	}
	listing := authRequest(router, http.MethodGet, "/api/v1/products", "", nil, "")
	if listing.Code != http.StatusOK || !strings.Contains(listing.Body.String(), `"image_url":"`+mediaPath+`"`) {
		t.Fatalf("Produktliste ohne Bild: %d %s", listing.Code, listing.Body.String())
	}
	for i := 1; i < catalog.MaxProductImages; i++ {
		if got := requestImage(source.Bytes(), "image/png", true, session.CSRFToken); got.Code != http.StatusCreated {
			t.Fatalf("Bild %d: %d", i+1, got.Code)
		}
	}
	assertProblem(t, requestImage(source.Bytes(), "image/png", true, session.CSRFToken), http.StatusConflict, "CONFLICT")
	deleted := authRequest(router, http.MethodDelete, "/api/v1/admin/images/"+id, "", cookie, session.CSRFToken)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("Bild löschen: %d", deleted.Code)
	}
	if response := authRequest(router, http.MethodGet, mediaPath, "", nil, ""); response.Code != http.StatusNotFound {
		t.Fatalf("Gelöschtes Bild sichtbar: %d", response.Code)
	}
}
