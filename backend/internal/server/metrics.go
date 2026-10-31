package server

import (
	"crypto/subtle"
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/database"
)

// operationalMetrics sammelt Betriebskennzahlen im Prozess. Labels sind fest oder
// stammen aus registrierten Routenmustern; Kunden-, Bestell- oder Sitzungsdaten
// gelangen nie in eine Metrik (ADR-012).
type operationalMetrics struct {
	mu               sync.Mutex
	requests         [6]uint64
	requestBuckets   [5]uint64
	requestSeconds   float64
	dbChecks         uint64
	dbErrors         uint64
	dbSeconds        float64
	paymentProcessed [4]uint64
	paymentErrors    uint64
	routes           map[routeKey]*routeStats
	auth             map[authKey]uint64
	inFlight         atomic.Int64
	started          time.Time
	db               pinger
}

type routeKey struct{ method, route string }

type routeStats struct {
	statusClass [6]uint64
	buckets     [len(routeBoundaries)]uint64
	sum         float64
	count       uint64
}

type authKey struct{ action, result string }

const metricsContextKey = "nordwinds.metrics"

var requestBoundaries = [5]time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}

// routeBoundaries sind feiner als die bisherigen Grenzen, damit p95/p99 schneller
// API-Antworten (meist deutlich unter 100 ms) aussagekräftig bleiben.
var routeBoundaries = [...]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

var knownMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true, http.MethodOptions: true,
}

// authResults legt alle erlaubten Ergebnisse fest; sie werden mit 0 vorbelegt,
// damit increase()/rate() in Alarmregeln von Anfang an eine Zeitreihe finden.
var authResults = map[string][]string{
	"login": {"success", "invalid_credentials", "rate_limited", "error"},
}

func newOperationalMetrics(db pinger) *operationalMetrics {
	m := &operationalMetrics{routes: map[routeKey]*routeStats{}, auth: map[authKey]uint64{}, started: time.Now(), db: db}
	for action, results := range authResults {
		for _, result := range results {
			m.auth[authKey{action, result}] = 0
		}
	}
	return m
}

func observeRequests(metrics *operationalMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(metricsContextKey, metrics)
		metrics.inFlight.Add(1)
		start := time.Now()
		c.Next()
		metrics.inFlight.Add(-1)
		duration := time.Since(start)
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		if !knownMethods[method] {
			method = "OTHER"
		}
		metrics.mu.Lock()
		defer metrics.mu.Unlock()
		class := c.Writer.Status() / 100
		if class >= 0 && class < len(metrics.requests) {
			metrics.requests[class]++
		}
		metrics.requestSeconds += duration.Seconds()
		for index, limit := range requestBoundaries {
			if duration <= limit {
				metrics.requestBuckets[index]++
			}
		}
		key := routeKey{method: method, route: route}
		stats := metrics.routes[key]
		if stats == nil {
			stats = &routeStats{}
			metrics.routes[key] = stats
		}
		if class >= 0 && class < len(stats.statusClass) {
			stats.statusClass[class]++
		}
		seconds := duration.Seconds()
		for index, limit := range routeBoundaries {
			if seconds <= limit {
				stats.buckets[index]++
			}
		}
		stats.sum += seconds
		stats.count++
	}
}

func (m *operationalMetrics) observeDB(duration time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dbChecks++
	m.dbSeconds += duration.Seconds()
	if err != nil {
		m.dbErrors++
	}
}

func metricsFrom(c *gin.Context) *operationalMetrics {
	value, exists := c.Get(metricsContextKey)
	if !exists {
		return nil
	}
	m, _ := value.(*operationalMetrics)
	return m
}

func observePayment(c *gin.Context, outcome string, err error) {
	m := metricsFrom(c)
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.paymentErrors++
		return
	}
	switch outcome {
	case "pending":
		m.paymentProcessed[0]++
	case "succeeded":
		m.paymentProcessed[1]++
	case "failed":
		m.paymentProcessed[2]++
	case "refunded":
		m.paymentProcessed[3]++
	}
}

// observeAuth zählt Anmeldeergebnisse ohne E-Mail-Adresse oder Herkunft.
func observeAuth(c *gin.Context, action, result string) {
	m := metricsFrom(c)
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := authKey{action, result}
	if _, known := m.auth[key]; known {
		m.auth[key]++
	}
}

func (m *operationalMetrics) serve(c *gin.Context, token string) {
	header := c.GetHeader("Authorization")
	provided := strings.TrimPrefix(header, "Bearer ")
	if !strings.HasPrefix(header, "Bearer ") || len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		writeProblem(c, Unauthenticated)
		return
	}
	var output strings.Builder
	w := promWriter{&output}
	// Datenbankabfragen laufen vor der Sperre, damit langsame Abfragen keine Anfragen blockieren.
	business := collectBusinessMetrics(c.Request.Context(), m.db)
	m.writeHTTP(w)
	m.writeDatabase(w)
	m.writePayments(w)
	m.writeAuth(w)
	business.write(w)
	writeRuntime(w, m.started)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(output.String()))
}

func (m *operationalMetrics) writeHTTP(w promWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.family("nordwinds_http_requests_total", "counter", "HTTP-Antworten der API je Statusklasse (alle Routen).")
	for class := 2; class <= 5; class++ {
		w.sample("nordwinds_http_requests_total", labels("status_class", statusClass(class)), float64(m.requests[class]))
	}
	w.family("nordwinds_http_request_duration_seconds", "histogram", "Antwortzeit der API über alle Routen.")
	for index, boundary := range requestBoundaries {
		w.sample("nordwinds_http_request_duration_seconds_bucket", labels("le", formatFloat(boundary.Seconds())), float64(m.requestBuckets[index]))
	}
	total := uint64(0)
	for _, count := range m.requests {
		total += count
	}
	w.sample("nordwinds_http_request_duration_seconds_bucket", labels("le", "+Inf"), float64(total))
	w.sample("nordwinds_http_request_duration_seconds_sum", "", m.requestSeconds)
	w.sample("nordwinds_http_request_duration_seconds_count", "", float64(total))

	keys := make([]routeKey, 0, len(m.routes))
	for key := range m.routes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		return keys[i].method < keys[j].method
	})
	w.family("nordwinds_http_route_requests_total", "counter", "HTTP-Antworten je Methode, Routenmuster und Statusklasse.")
	for _, key := range keys {
		for class, count := range m.routes[key].statusClass {
			if count > 0 {
				w.sample("nordwinds_http_route_requests_total",
					labels("method", key.method, "route", key.route, "status_class", statusClass(class)), float64(count))
			}
		}
	}
	w.family("nordwinds_http_route_request_duration_seconds", "histogram", "Antwortzeit je Methode und Routenmuster.")
	for _, key := range keys {
		stats := m.routes[key]
		for index, boundary := range routeBoundaries {
			w.sample("nordwinds_http_route_request_duration_seconds_bucket",
				labels("method", key.method, "route", key.route, "le", formatFloat(boundary)), float64(stats.buckets[index]))
		}
		w.sample("nordwinds_http_route_request_duration_seconds_bucket",
			labels("method", key.method, "route", key.route, "le", "+Inf"), float64(stats.count))
		w.sample("nordwinds_http_route_request_duration_seconds_sum", labels("method", key.method, "route", key.route), stats.sum)
		w.sample("nordwinds_http_route_request_duration_seconds_count", labels("method", key.method, "route", key.route), float64(stats.count))
	}
	w.family("nordwinds_http_requests_in_flight", "gauge", "Gerade bearbeitete HTTP-Anfragen.")
	w.sample("nordwinds_http_requests_in_flight", "", float64(m.inFlight.Load()))
}

func (m *operationalMetrics) writeDatabase(w promWriter) {
	m.mu.Lock()
	checks, failures, seconds := m.dbChecks, m.dbErrors, m.dbSeconds
	m.mu.Unlock()
	w.family("nordwinds_db_readiness_checks_total", "counter", "Datenbankprüfungen von /health/ready.")
	w.sample("nordwinds_db_readiness_checks_total", "", float64(checks))
	w.family("nordwinds_db_readiness_errors_total", "counter", "Fehlgeschlagene Datenbankprüfungen von /health/ready.")
	w.sample("nordwinds_db_readiness_errors_total", "", float64(failures))
	w.family("nordwinds_db_readiness_duration_seconds", "summary", "Dauer der Datenbankprüfungen von /health/ready.")
	w.sample("nordwinds_db_readiness_duration_seconds_sum", "", seconds)
	w.sample("nordwinds_db_readiness_duration_seconds_count", "", float64(checks))
	queryErrors, slowQueries := database.QueryMetrics()
	w.family("nordwinds_db_query_errors_total", "counter", "Fehlgeschlagene GORM-Abfragen (ohne „nicht gefunden“).")
	w.sample("nordwinds_db_query_errors_total", "", float64(queryErrors))
	w.family("nordwinds_db_slow_queries_total", "counter", "GORM-Abfragen über einer Sekunde.")
	w.sample("nordwinds_db_slow_queries_total", "", float64(slowQueries))

	source, ok := m.db.(interface{ Stats() sql.DBStats })
	if !ok {
		return
	}
	stats := source.Stats()
	for _, gauge := range []struct {
		name, help string
		value      int
	}{
		{"nordwinds_db_pool_max_open_connections", "Obergrenze offener Datenbankverbindungen (0 = unbegrenzt).", stats.MaxOpenConnections},
		{"nordwinds_db_pool_open_connections", "Offene Datenbankverbindungen.", stats.OpenConnections},
		{"nordwinds_db_pool_in_use_connections", "Verwendete Datenbankverbindungen.", stats.InUse},
		{"nordwinds_db_pool_idle_connections", "Freie Datenbankverbindungen.", stats.Idle},
	} {
		w.family(gauge.name, "gauge", gauge.help)
		w.sample(gauge.name, "", float64(gauge.value))
	}
	w.family("nordwinds_db_pool_wait_count_total", "counter", "Anfragen, die auf eine freie Datenbankverbindung warten mussten.")
	w.sample("nordwinds_db_pool_wait_count_total", "", float64(stats.WaitCount))
	w.family("nordwinds_db_pool_wait_duration_seconds_total", "counter", "Gesamte Wartezeit auf freie Datenbankverbindungen.")
	w.sample("nordwinds_db_pool_wait_duration_seconds_total", "", stats.WaitDuration.Seconds())
}

func (m *operationalMetrics) writePayments(w promWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w.family("nordwinds_mock_payment_events_total", "counter", "Verarbeitete Ereignisse der simulierten Zahlung je Ergebnis.")
	for index, outcome := range []string{"pending", "succeeded", "failed", "refunded"} {
		w.sample("nordwinds_mock_payment_events_total", labels("outcome", outcome), float64(m.paymentProcessed[index]))
	}
	w.family("nordwinds_mock_payment_errors_total", "counter", "Abgelehnte oder fehlgeschlagene Ereignisse der simulierten Zahlung.")
	w.sample("nordwinds_mock_payment_errors_total", "", float64(m.paymentErrors))
}

func (m *operationalMetrics) writeAuth(w promWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]authKey, 0, len(m.auth))
	for key := range m.auth {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].action != keys[j].action {
			return keys[i].action < keys[j].action
		}
		return keys[i].result < keys[j].result
	})
	w.family("nordwinds_auth_attempts_total", "counter", "Anmeldeversuche je Ergebnis.")
	for _, key := range keys {
		w.sample("nordwinds_auth_attempts_total", labels("action", key.action, "result", key.result), float64(m.auth[key]))
	}
}

func statusClass(class int) string { return string(rune('0'+class)) + "xx" }
