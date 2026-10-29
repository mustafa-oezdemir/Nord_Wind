package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/database"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/server"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				attr.Key = "timestamp"
			}
			return attr
		},
	})).With("service", "nordwinds-api", "environment", appEnv)
	db, err := database.OpenFromEnv()
	if err != nil {
		logger.Error("Datenbankkonfiguration ungültig", "event", "config.invalid")
		os.Exit(1)
	}
	defer db.Close()

	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = "NordWinds"
	}
	router, err := server.New(appName, db, logger)
	if err != nil {
		logger.Error("Router konnte nicht erstellt werden", "event", "router.failed")
		os.Exit(1)
	}
	port := os.Getenv("API_HTTP_PORT")
	if port == "" {
		port = "8080"
	}
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Error("HTTP-Port konnte nicht geöffnet werden", "event", "listener.failed")
		os.Exit(1)
	}
	httpServer := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- httpServer.Serve(listener) }()
	logger.Info("API gestartet", "event", "app.started", "port", port)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP-Shutdown fehlgeschlagen", "event", "shutdown.failed")
		}
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP-Server beendet", "event", "server.failed")
			os.Exit(1)
		}
	}
}
