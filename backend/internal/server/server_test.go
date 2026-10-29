package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeDB struct{ err error }

func (f fakeDB) PingContext(context.Context) error { return f.err }

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func TestHealthAndBrand(t *testing.T) {
	router, err := New("NordWinds", fakeDB{}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/health/live", "/health/ready"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: Status %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"NordWinds"`) {
		t.Fatalf("Metadaten: Status %d, Body %s", response.Code, response.Body.String())
	}
}

func TestReadinessFailsWhenDatabaseIsUnavailable(t *testing.T) {
	router, err := New("NordWinds", fakeDB{err: errors.New("offline")}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("Status %d statt 503", response.Code)
	}
	if strings.Contains(response.Body.String(), "offline") {
		t.Fatal("Interner Datenbankfehler wurde veröffentlicht")
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/problem+json") {
		t.Fatal("Readiness-Fehler hat kein Problemformat")
	}
}
