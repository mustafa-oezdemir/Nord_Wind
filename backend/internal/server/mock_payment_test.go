package server

import (
	"bytes"
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
	router := gin.New()
	router.Use(assignRequestID)
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
}
