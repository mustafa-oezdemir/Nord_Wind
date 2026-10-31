package server

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/database"
)

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
}

const metricsContextKey = "nordwinds.metrics"

var requestBoundaries = [5]time.Duration{100 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second, 5 * time.Second}

func observeRequests(metrics *operationalMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(metricsContextKey, metrics)
		start := time.Now()
		c.Next()
		metrics.mu.Lock()
		defer metrics.mu.Unlock()
		class := c.Writer.Status() / 100
		if class >= 0 && class < len(metrics.requests) {
			metrics.requests[class]++
		}
		duration := time.Since(start)
		metrics.requestSeconds += duration.Seconds()
		for index, limit := range requestBoundaries {
			if duration <= limit {
				metrics.requestBuckets[index]++
			}
		}
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

func observePayment(c *gin.Context, outcome string, err error) {
	value, exists := c.Get(metricsContextKey)
	if !exists {
		return
	}
	m, ok := value.(*operationalMetrics)
	if !ok {
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

func (m *operationalMetrics) serve(c *gin.Context, token string) {
	header := c.GetHeader("Authorization")
	provided := strings.TrimPrefix(header, "Bearer ")
	if !strings.HasPrefix(header, "Bearer ") || len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
		writeProblem(c, Unauthenticated)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var output strings.Builder
	for class := 2; class <= 5; class++ {
		fmt.Fprintf(&output, "nordwinds_http_requests_total{status_class=\"%dxx\"} %d\n", class, m.requests[class])
	}
	for index, boundary := range requestBoundaries {
		fmt.Fprintf(&output, "nordwinds_http_request_duration_seconds_bucket{le=\"%g\"} %d\n", boundary.Seconds(), m.requestBuckets[index])
	}
	total := uint64(0)
	for _, count := range m.requests {
		total += count
	}
	fmt.Fprintf(&output, "nordwinds_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", total)
	fmt.Fprintf(&output, "nordwinds_http_request_duration_seconds_sum %g\n", m.requestSeconds)
	fmt.Fprintf(&output, "nordwinds_http_request_duration_seconds_count %d\n", total)
	fmt.Fprintf(&output, "nordwinds_db_readiness_checks_total %d\n", m.dbChecks)
	fmt.Fprintf(&output, "nordwinds_db_readiness_errors_total %d\n", m.dbErrors)
	fmt.Fprintf(&output, "nordwinds_db_readiness_duration_seconds_sum %g\n", m.dbSeconds)
	queryErrors, slowQueries := database.QueryMetrics()
	fmt.Fprintf(&output, "nordwinds_db_query_errors_total %d\n", queryErrors)
	fmt.Fprintf(&output, "nordwinds_db_slow_queries_total %d\n", slowQueries)
	for index, outcome := range []string{"pending", "succeeded", "failed", "refunded"} {
		fmt.Fprintf(&output, "nordwinds_mock_payment_events_total{outcome=\"%s\"} %d\n", outcome, m.paymentProcessed[index])
	}
	fmt.Fprintf(&output, "nordwinds_mock_payment_errors_total %d\n", m.paymentErrors)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(output.String()))
}
