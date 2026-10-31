package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func TestAuthenticationOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_AUTH_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_AUTH_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var databaseName string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(databaseName, "nordwinds_test_") {
		t.Fatalf("Auth-Test verweigert Datenbank %q", databaseName)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM customers`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("Auth-Test benötigt leere Kunden-Tabelle: %d, %v", count, err)
	}
	t.Cleanup(func() {
		for _, statement := range []string{`DELETE FROM auth_sessions`, `DELETE FROM auth_rate_limits`, `DELETE FROM customers`} {
			if _, err := db.ExecContext(context.Background(), statement); err != nil {
				t.Errorf("Fixture-Cleanup: %v", err)
			}
		}
		_ = db.Close()
	})
	var logs bytes.Buffer
	router, err := New("NordWinds", db, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	service, err := identity.NewService(identity.NewRepository(db))
	if err != nil {
		t.Fatal(err)
	}
	handler := RegisterAuthRoutes(router, service, AuthConfig{PublicOrigin: "https://shop.example.invalid", SecureCookie: true})
	router.GET("/test/admin", handler.RequireSession(), handler.RequireRole(identity.AdminRole), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	password := "Very-Secret-Passphrase-2026"
	registerBody := fmt.Sprintf(`{"email":"Test@Example.Invalid","password":%q}`, password)
	registered := authRequest(router, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
	if registered.Code != http.StatusCreated {
		t.Fatalf("Registrierung: %d %s", registered.Code, registered.Body.String())
	}
	duplicate := authRequest(router, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
	assertProblem(t, duplicate, http.StatusConflict, "CONFLICT")
	wrong := authRequest(router, http.MethodPost, "/api/v1/auth/login", `{"email":"test@example.invalid","password":"wrong-password"}`, nil, "")
	assertProblem(t, wrong, http.StatusUnauthorized, "UNAUTHENTICATED")
	loginBody := fmt.Sprintf(`{"email":"test@example.invalid","password":%q}`, password)
	fixed := &http.Cookie{Name: "__Host-nordwinds_session", Value: strings.Repeat("x", 43)}
	login := authRequest(router, http.MethodPost, "/api/v1/auth/login", loginBody, fixed, "")
	if login.Code != http.StatusOK {
		t.Fatalf("Login: %d %s", login.Code, login.Body.String())
	}
	cookie := sessionCookie(t, login)
	if cookie.Value == fixed.Value || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatal("Session-Cookie oder Rotation ungültig")
	}
	csrf := csrfFromResponse(t, login)
	if got := authRequest(router, http.MethodGet, "/api/v1/auth/session", "", fixed, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Fixiertes Cookie wurde akzeptiert: %d", got.Code)
	}
	if got := authRequest(router, http.MethodGet, "/api/v1/auth/session", "", cookie, ""); got.Code != http.StatusOK {
		t.Fatalf("Gültige Session: %d", got.Code)
	}
	for _, header := range []string{"", "invalid"} {
		response := authRequest(router, http.MethodPost, "/api/v1/auth/logout", "", cookie, header)
		assertProblem(t, response, http.StatusForbidden, "FORBIDDEN")
	}
	crossSite := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	crossSite.Host = "shop.example.invalid"
	crossSite.Header.Set("Origin", "https://evil.example.invalid")
	crossSite.Header.Set("X-CSRF-Token", csrf)
	crossSite.AddCookie(cookie)
	crossSiteResponse := httptest.NewRecorder()
	router.ServeHTTP(crossSiteResponse, crossSite)
	assertProblem(t, crossSiteResponse, http.StatusForbidden, "FORBIDDEN")
	if got := authRequest(router, http.MethodGet, "/test/admin", "", cookie, ""); got.Code != http.StatusForbidden {
		t.Fatalf("Kundenrolle darf Admin-Route nicht lesen: %d", got.Code)
	}
	if _, err := db.ExecContext(ctx, `UPDATE customers SET role = 'admin' WHERE email = 'test@example.invalid'`); err != nil {
		t.Fatal(err)
	}
	if got := authRequest(router, http.MethodGet, "/test/admin", "", cookie, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Rollenwechsel muss alte Session beenden: %d", got.Code)
	}
	rotated := authRequest(router, http.MethodPost, "/api/v1/auth/login", loginBody, cookie, "")
	if rotated.Code != http.StatusOK {
		t.Fatalf("Session-Rotation: %d", rotated.Code)
	}
	newCookie := sessionCookie(t, rotated)
	if got := authRequest(router, http.MethodGet, "/test/admin", "", newCookie, ""); got.Code != http.StatusNoContent {
		t.Fatalf("Adminrolle nach Neuanmeldung: %d", got.Code)
	}
	if newCookie.Value == cookie.Value {
		t.Fatal("Login hat Session nicht rotiert")
	}
	if got := authRequest(router, http.MethodGet, "/api/v1/auth/session", "", cookie, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Alte Session blieb gültig: %d", got.Code)
	}
	newCSRF := csrfFromResponse(t, rotated)
	logout := authRequest(router, http.MethodPost, "/api/v1/auth/logout", "", newCookie, newCSRF)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("Logout: %d %s", logout.Code, logout.Body.String())
	}
	if got := authRequest(router, http.MethodGet, "/api/v1/auth/session", "", newCookie, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Logout-Session blieb gültig: %d", got.Code)
	}
	expiring := authRequest(router, http.MethodPost, "/api/v1/auth/login", loginBody, nil, "")
	if expiring.Code != http.StatusOK {
		t.Fatalf("Neuer Login: %d", expiring.Code)
	}
	expiredCookie := sessionCookie(t, expiring)
	if _, err := db.ExecContext(ctx, `UPDATE auth_sessions SET created_at = now() - interval '2 days',
		expires_at = now() - interval '1 day', idle_expires_at = now() - interval '1 day'
		WHERE token_hash = $1`, identity.TokenHash(expiredCookie.Value)); err != nil {
		t.Fatal(err)
	}
	if got := authRequest(router, http.MethodGet, "/api/v1/auth/session", "", expiredCookie, ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("Abgelaufene Session wurde akzeptiert: %d", got.Code)
	}
	for attempt := 0; attempt < 6; attempt++ {
		response := authRequest(router, http.MethodPost, "/api/v1/auth/login", `{"email":"rate@example.invalid","password":"wrong-password"}`, nil, "")
		want := http.StatusUnauthorized
		if attempt == 5 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("Rate-Limit-Versuch %d: %d statt %d", attempt+1, response.Code, want)
		}
	}
	for attempt := 3; attempt <= 21; attempt++ {
		response := authRequest(router, http.MethodPost, "/api/v1/auth/register", registerBody, nil, "")
		if attempt == 21 {
			assertProblem(t, response, http.StatusTooManyRequests, "RATE_LIMITED")
			if response.Header().Get("Retry-After") != "900" {
				t.Fatal("Retry-After fehlt beim Registrierungs-Limit")
			}
		} else {
			assertProblem(t, response, http.StatusConflict, "CONFLICT")
		}
	}
	text := logs.String()
	for _, secret := range []string{password, cookie.Value, newCookie.Value, csrf, newCSRF, "test@example.invalid"} {
		if strings.Contains(text, secret) {
			t.Fatal("Geheimnis oder E-Mail im Access-Log")
		}
	}
}

func authRequest(router http.Handler, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "shop.example.invalid"
	request.Header.Set("Origin", "https://shop.example.invalid")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if csrf != "" {
		request.Header.Set("X-CSRF-Token", csrf)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func sessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "__Host-nordwinds_session" {
			return cookie
		}
	}
	t.Fatal("Session-Cookie fehlt")
	return nil
}

func csrfFromResponse(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.CSRFToken) != 43 {
		t.Fatalf("CSRF-Token fehlt: %v", err)
	}
	return body.CSRFToken
}
