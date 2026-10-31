package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var (
	sampleLine = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{(?:[a-zA-Z_][a-zA-Z0-9_]*="(?:[^"\\]|\\.)*",?)*\})? (\S+)$`)
	typeLine   = regexp.MustCompile(`^# TYPE ([a-zA-Z_:][a-zA-Z0-9_:]*) (counter|gauge|histogram|summary|untyped)$`)
	helpLine   = regexp.MustCompile(`^# HELP ([a-zA-Z_:][a-zA-Z0-9_:]*) .+$`)
)

// parseExposition prüft das Textformat 0.0.4 so streng, wie Prometheus es beim Scrape
// erwartet: HELP und TYPE vor den Werten, jede Familie zusammenhängend, gültige Zahlen.
func parseExposition(t *testing.T, body string) map[string]float64 {
	t.Helper()
	values := map[string]float64{}
	types := map[string]string{}
	closed := map[string]bool{}
	current := ""
	for number, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "# HELP "):
			if !helpLine.MatchString(line) {
				t.Fatalf("Zeile %d: ungültiges HELP %q", number+1, line)
			}
		case strings.HasPrefix(line, "# TYPE "):
			match := typeLine.FindStringSubmatch(line)
			if match == nil {
				t.Fatalf("Zeile %d: ungültiges TYPE %q", number+1, line)
			}
			if _, seen := types[match[1]]; seen {
				t.Fatalf("Familie %s doppelt", match[1])
			}
			if current != "" {
				closed[current] = true
			}
			types[match[1]], current = match[2], match[1]
		default:
			match := sampleLine.FindStringSubmatch(line)
			if match == nil {
				t.Fatalf("Zeile %d: ungültiger Wert %q", number+1, line)
			}
			family := match[1]
			if kind := types[current]; kind == "histogram" || kind == "summary" {
				family = current
				if !strings.HasPrefix(match[1], current) {
					t.Fatalf("Zeile %d: %s gehört nicht zu %s", number+1, match[1], current)
				}
			}
			if family != current || closed[family] {
				t.Fatalf("Zeile %d: %s steht außerhalb seiner Familie", number+1, match[1])
			}
			value, err := strconv.ParseFloat(strings.TrimPrefix(match[3], "+"), 64)
			if err != nil {
				t.Fatalf("Zeile %d: Zahl %q", number+1, match[3])
			}
			values[match[1]+match[2]] = value
		}
	}
	return values
}

func scrape(t *testing.T, router *gin.Engine, token string) map[string]float64 {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/internal/metrics", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Scrape: %d %q", response.Code, response.Header().Get("Content-Type"))
	}
	return parseExposition(t, response.Body.String())
}

func TestMetricsExpositionRoutesAndRuntime(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("t", 32)
	router, err := New("NordWinds", fakeDB{}, discardLogger(), token)
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/test/fail/:id", func(c *gin.Context) { writeProblem(c, InternalError) })
	for _, path := range []string{"/health/live", "/health/live", "/api/v1/meta", "/test/fail/4711", "/gibt-es-nicht"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("PROPFIND", "/health/live", nil))

	values := scrape(t, router, token)
	for series, want := range map[string]float64{
		`nordwinds_http_route_requests_total{method="GET",route="/health/live",status_class="2xx"}`:         2,
		`nordwinds_http_route_requests_total{method="GET",route="/api/v1/meta",status_class="2xx"}`:         1,
		`nordwinds_http_route_requests_total{method="GET",route="/test/fail/:id",status_class="5xx"}`:       1,
		`nordwinds_http_route_requests_total{method="GET",route="unmatched",status_class="4xx"}`:            1,
		`nordwinds_http_route_request_duration_seconds_count{method="GET",route="/health/live"}`:            2,
		`nordwinds_http_route_request_duration_seconds_bucket{method="GET",route="/health/live",le="+Inf"}`: 2,
		`nordwinds_http_requests_total{status_class="5xx"}`:                                                 1,
		`nordwinds_auth_attempts_total{action="login",result="rate_limited"}`:                               0,
		`nordwinds_http_requests_in_flight`:                                                                 1,
		`go_info{version="` + runtime.Version() + `"}`:                                                      1,
	} {
		if got, ok := values[series]; !ok || got != want {
			t.Errorf("%s = %v (vorhanden: %v), erwartet %v", series, got, ok, want)
		}
	}
	// Unbekannte Methoden landen gesammelt unter OTHER, die konkrete ID nie im Label.
	foundOther := false
	for series := range values {
		if strings.Contains(series, "4711") || strings.Contains(series, "PROPFIND") || strings.Contains(series, "gibt-es-nicht") {
			t.Fatalf("Rohwert im Label: %s", series)
		}
		if strings.Contains(series, `method="OTHER"`) {
			foundOther = true
		}
	}
	if !foundOther {
		t.Error("unbekannte Methode nicht als OTHER gezählt")
	}
	// Buckets sind kumulativ.
	previous := -1.0
	for _, le := range []string{"0.005", "0.01", "0.025", "0.05", "0.1", "0.25", "0.5", "1", "2.5", "5", "10", "+Inf"} {
		value := values[`nordwinds_http_route_request_duration_seconds_bucket{method="GET",route="/health/live",le="`+le+`"}`]
		if value < previous {
			t.Fatalf("Bucket le=%s fällt: %v < %v", le, value, previous)
		}
		previous = value
	}
	if values["go_goroutines"] < 1 || values["process_start_time_seconds"] <= 0 {
		t.Error("Laufzeitwerte fehlen")
	}
	if runtime.GOOS == "linux" && (values["process_resident_memory_bytes"] <= 0 || values["process_open_fds"] <= 0) {
		t.Error("Prozesswerte fehlen unter Linux")
	}
	if _, ok := values["nordwinds_business_metrics_up"]; ok {
		t.Error("Geschäftskennzahlen ohne Datenbank gemeldet")
	}
}

func TestLoginAttemptsAreCountedWithoutIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("u", 32)
	router, err := New("NordWinds", fakeDB{err: errors.New("offline")}, discardLogger(), token)
	if err != nil {
		t.Fatal(err)
	}
	router.GET("/probe", func(c *gin.Context) {
		observeAuth(c, "login", "invalid_credentials")
		observeAuth(c, "login", "erfunden")
		observeAuth(c, "logout", "success")
		c.Status(http.StatusNoContent)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))
	values := scrape(t, router, token)
	if values[`nordwinds_auth_attempts_total{action="login",result="invalid_credentials"}`] != 1 {
		t.Fatal("Fehlversuch nicht gezählt")
	}
	for series := range values {
		if strings.Contains(series, "erfunden") || strings.Contains(series, "logout") {
			t.Fatalf("unbekanntes Label angelegt: %s", series)
		}
	}
}
