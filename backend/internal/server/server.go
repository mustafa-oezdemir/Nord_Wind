package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type pinger interface {
	PingContext(context.Context) error
}

// New richtet die minimalen Betriebs- und Metadaten-Endpunkte ein.
func New(appName string, db pinger, logger *slog.Logger, metricsToken ...string) (*gin.Engine, error) {
	if logger == nil {
		return nil, errors.New("Logger fehlt")
	}
	if len(metricsToken) > 1 || (len(metricsToken) == 1 && metricsToken[0] != "" && len(metricsToken[0]) < 32) {
		return nil, errors.New("NORDWINDS_METRICS_TOKEN muss mindestens 32 Zeichen haben")
	}
	metrics := newOperationalMetrics(db)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(assignRequestID, observeRequests(metrics), accessLog(logger), recoverProblem, limitInput, withDeadline)
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		start := time.Now()
		err := db.PingContext(ctx)
		metrics.observeDB(time.Since(start), err)
		if err != nil {
			HandleError(c, &APIError{Code: ServiceUnavailable})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	if len(metricsToken) == 1 && metricsToken[0] != "" {
		router.GET("/internal/metrics", func(c *gin.Context) { metrics.serve(c, metricsToken[0]) })
	}
	router.GET("/api/v1/meta", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":     appName,
			"locale":   "de-DE",
			"currency": "EUR",
		})
	})
	router.GET("/api/v1/problems/:slug", func(c *gin.Context) {
		for code, spec := range problemSpecs {
			if strings.TrimPrefix(spec.typeURI, "/api/v1/problems/") == c.Param("slug") {
				c.String(http.StatusOK, "%s: %s — %s", code, spec.title, spec.detail)
				return
			}
		}
		writeProblem(c, NotFound)
	})
	router.NoRoute(func(c *gin.Context) { writeProblem(c, NotFound) })
	router.NoMethod(func(c *gin.Context) { writeProblem(c, MethodNotAllowed) })
	return router, nil
}
