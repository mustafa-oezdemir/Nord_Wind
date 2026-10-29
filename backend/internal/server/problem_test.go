package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestProblemResponsesAndSafeAccessLogs(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	router, err := New("NordWinds", fakeDB{}, logger)
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/test/invalid", func(c *gin.Context) { HandleError(c, &APIError{Code: InvalidRequest}) })
	router.GET("/test/conflict", func(c *gin.Context) { HandleError(c, &APIError{Code: Conflict}) })
	router.GET("/test/:id", func(c *gin.Context) { HandleError(c, &APIError{Code: InvalidRequest}) })
	router.GET("/test/panic", func(c *gin.Context) { panic("SQL secret_token cookie street_address") })
	router.POST("/test/internal", func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		HandleError(c, errors.New("SQL secret_token cookie street_address"))
	})

	tests := []struct {
		method string
		path   string
		code   ErrorCode
		status int
		level  string
		route  string
	}{
		{http.MethodGet, "/test/invalid", InvalidRequest, 400, "INFO", "/test/invalid"},
		{http.MethodGet, "/missing", NotFound, 404, "INFO", "unmatched"},
		{http.MethodGet, "/test/conflict", Conflict, 409, "INFO", "/test/conflict"},
		{http.MethodGet, "/test/secret_token", InvalidRequest, 400, "INFO", "/test/:id"},
		{http.MethodPost, "/api/v1/meta", MethodNotAllowed, 405, "INFO", "unmatched"},
		{http.MethodGet, "/test/panic", InternalError, 500, "ERROR", "/test/panic"},
		{http.MethodPost, "/test/internal", InternalError, 500, "ERROR", "/test/internal"},
	}
	seenIDs := make(map[string]bool)
	for _, tt := range tests {
		t.Run(string(tt.code)+tt.path, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path+"?token=secret_token&address=street_address", strings.NewReader("street_address"))
			request.Header.Set("Authorization", "Bearer secret_token")
			request.Header.Set("Cookie", "session=secret_token")
			request.Header.Set("X-Request-ID", "untrusted_secret_token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.status {
				t.Fatalf("HTTP %d statt %d: %s", response.Code, tt.status, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
				t.Fatalf("Content-Type %q", contentType)
			}
			var body problem
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != tt.status || body.Code != tt.code || body.RequestID != response.Header().Get("X-Request-ID") || len(body.RequestID) != 32 || !strings.HasPrefix(body.Type, "/api/v1/problems/") {
				t.Fatalf("Ungültige Problemantwort: %+v", body)
			}
			if seenIDs[body.RequestID] {
				t.Fatal("Request-ID wurde wiederverwendet")
			}
			seenIDs[body.RequestID] = true
			if strings.Contains(response.Body.String(), "secret_token") || strings.Contains(response.Body.String(), "street_address") || strings.Contains(response.Body.String(), "SQL") {
				t.Fatal("Sensible Daten in der Problemantwort")
			}
		})
	}
	for _, secret := range []string{"secret_token", "street_address", "SQL", "session=", "Bearer"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("Sensible Daten im Log: %s", secret)
		}
	}
	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != len(tests) {
		t.Fatalf("%d Logzeilen statt %d", len(lines), len(tests))
	}
	for i, line := range lines {
		var entry struct {
			Level     string `json:"level"`
			Route     string `json:"route"`
			Code      string `json:"error_code"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.Level != tests[i].level || entry.Route != tests[i].route || entry.Code != string(tests[i].code) || len(entry.RequestID) != 32 {
			t.Fatalf("Falscher Logeintrag: %+v", entry)
		}
	}
	for code, spec := range problemSpecs {
		info := httptest.NewRecorder()
		router.ServeHTTP(info, httptest.NewRequest(http.MethodGet, spec.typeURI, nil))
		if info.Code != http.StatusOK || !strings.Contains(info.Body.String(), string(code)) {
			t.Fatalf("Problemtyp %s ist nicht dokumentiert", code)
		}
	}
}

func TestInputLimitsAndDeadline(t *testing.T) {
	router, err := New("NordWinds", fakeDB{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	router.POST("/test/read", func(c *gin.Context) {
		if _, err := io.ReadAll(c.Request.Body); err != nil {
			HandleError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/test/deadline", func(c *gin.Context) {
		deadline, ok := c.Request.Context().Deadline()
		if !ok || deadline.After(time.Now().Add(requestDeadline)) || RequestID(c.Request.Context()) == "" {
			panic("Request-Kontext fehlt")
		}
		HandleError(c, c.Request.Context().Err())
	})
	large := strings.Repeat("x", maxBodyBytes+1)
	checks := []struct {
		name string
		req  *http.Request
		want int
	}{
		{"known body length", httptest.NewRequest(http.MethodPost, "/test/read", strings.NewReader(large)), 413},
		{"chunked body length", func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/test/read", strings.NewReader(large))
			r.ContentLength = -1
			return r
		}(), 413},
		{"long target", httptest.NewRequest(http.MethodGet, "/?q="+strings.Repeat("x", maxTargetBytes), nil), 414},
		{"expired request", func() *http.Request {
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			t.Cleanup(cancel)
			return httptest.NewRequest(http.MethodGet, "/test/deadline", nil).WithContext(ctx)
		}(), 503},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, check.req)
			if response.Code != check.want || !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
				t.Fatalf("HTTP %d, Content-Type %q", response.Code, response.Header().Get("Content-Type"))
			}
		})
	}
}
