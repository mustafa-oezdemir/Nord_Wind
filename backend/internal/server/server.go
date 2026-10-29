package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type pinger interface {
	PingContext(context.Context) error
}

// New richtet die minimalen Betriebs- und Metadaten-Endpunkte ein.
func New(appName string, db pinger) (*gin.Engine, error) {
	router := gin.New()
	router.Use(gin.Recovery())
	if err := router.SetTrustedProxies(nil); err != nil {
		return nil, err
	}

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/api/v1/meta", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":     appName,
			"locale":   "de-DE",
			"currency": "EUR",
		})
	})
	return router, nil
}
