package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSafeOriginLocalDevelopmentAlias(t *testing.T) {
	tests := []struct {
		name       string
		expected   string
		host       string
		origin     string
		referer    string
		fetchSite  string
		secure     bool
		wantAccept bool
	}{
		{"configured localhost", "http://localhost:8080", "localhost:8080", "http://localhost:8080", "", "same-origin", false, true},
		{"127 alias", "http://localhost:8080", "127.0.0.1:8080", "http://127.0.0.1:8080", "", "same-origin", false, true},
		{"localhost alias", "http://127.0.0.1:8080", "localhost:8080", "http://localhost:8080", "", "same-origin", false, true},
		{"referer alias", "http://localhost:8080", "127.0.0.1:8080", "", "http://127.0.0.1:8080/warenkorb", "same-origin", false, true},
		{"different port", "http://localhost:8080", "127.0.0.1:8081", "http://127.0.0.1:8081", "", "same-origin", false, false},
		{"host mismatch", "http://localhost:8080", "localhost:8080", "http://127.0.0.1:8080", "", "same-origin", false, false},
		{"remote host", "http://localhost:8080", "evil.example.invalid:8080", "http://evil.example.invalid:8080", "", "same-origin", false, false},
		{"cross site", "http://localhost:8080", "127.0.0.1:8080", "http://127.0.0.1:8080", "", "cross-site", false, false},
		{"https stays strict", "https://localhost:8080", "127.0.0.1:8080", "https://127.0.0.1:8080", "", "same-origin", true, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			request.Host = test.host
			request.Header.Set("Origin", test.origin)
			request.Header.Set("Referer", test.referer)
			request.Header.Set("Sec-Fetch-Site", test.fetchSite)
			context.Request = request
			handler := AuthHandler{config: AuthConfig{PublicOrigin: test.expected, SecureCookie: test.secure}}
			if got := handler.safeOrigin(context); got != test.wantAccept {
				t.Fatalf("safeOrigin() = %t, erwartet %t", got, test.wantAccept)
			}
		})
	}
}
