package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOperationalMetricsRequireTokenAndCountFailures(t *testing.T) {
	token := strings.Repeat("m", 32)
	router, err := New("NordWinds", fakeDB{err: errors.New("db offline")}, discardLogger(), token)
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/test/failure", func(c *gin.Context) { writeProblem(c, InternalError) })
	for _, path := range []string{"/health/ready", "/test/failure"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusServiceUnavailable && response.Code != http.StatusInternalServerError {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/internal/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized || strings.Contains(unauthorized.Body.String(), token) {
		t.Fatal("Metriken ohne Token sichtbar")
	}
	request := httptest.NewRequest(http.MethodGet, "/internal/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	for _, expected := range []string{
		`nordwinds_http_requests_total{status_class="5xx"} 2`,
		`nordwinds_db_readiness_checks_total 1`,
		`nordwinds_db_readiness_errors_total 1`,
		`nordwinds_db_query_errors_total`,
		`nordwinds_http_request_duration_seconds_bucket{le="+Inf"}`,
		`nordwinds_mock_payment_errors_total 0`,
	} {
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("Metrik fehlt: %s", expected)
		}
	}
	if strings.Contains(response.Body.String(), token) || strings.Contains(response.Body.String(), "db offline") {
		t.Fatal("Geheimnis oder Fehlertext in Metriken")
	}
}
