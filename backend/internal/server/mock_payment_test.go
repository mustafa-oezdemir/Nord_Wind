package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/payment"
)

func TestMockWebhookRejectsUnsignedAndMalformedEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := bytes.Repeat([]byte("s"), 32)
	handler := &mockPaymentHandler{secret: secret}
	var logs bytes.Buffer
	metricsToken := string(bytes.Repeat([]byte("m"), 32))
	router, err := New("NordWinds", fakeDB{}, slog.New(slog.NewJSONHandler(&logs, nil)), metricsToken)
	if err != nil {
		t.Fatal(err)
	}
	router.POST("/hook", handler.webhook)
	request := func(body []byte, signature string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if signature != "" {
			req.Header.Set("X-Nordwinds-Mock-Signature", signature)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	badJSON := []byte(`{"id":"evt_123"`)
	assertProblem(t, request(badJSON, ""), http.StatusUnauthorized, "UNAUTHENTICATED")
	assertProblem(t, request(badJSON, payment.Sign(secret, badJSON, time.Now())), http.StatusBadRequest, "INVALID_REQUEST")
	unknown := []byte(`{"id":"evt_123","unknown":"value"}`)
	assertProblem(t, request(unknown, payment.Sign(secret, unknown, time.Now())), http.StatusBadRequest, "INVALID_REQUEST")
	sentinel := "payment-secret-never-log"
	assertProblem(t, request([]byte(sentinel), sentinel), http.StatusUnauthorized, "UNAUTHENTICATED")
	if bytes.Contains(logs.Bytes(), []byte(sentinel)) || bytes.Contains(logs.Bytes(), secret) {
		t.Fatal("Payment-Secret wurde protokolliert")
	}
	metricsRequest := httptest.NewRequest(http.MethodGet, "/internal/metrics", nil)
	metricsRequest.Header.Set("Authorization", "Bearer "+metricsToken)
	metricsResponse := httptest.NewRecorder()
	router.ServeHTTP(metricsResponse, metricsRequest)
	if metricsResponse.Code != http.StatusOK || !bytes.Contains(metricsResponse.Body.Bytes(), []byte("nordwinds_mock_payment_errors_total 4")) {
		t.Fatal("Fehlgeschlagene Payment-Verifikation nicht messbar")
	}
}
