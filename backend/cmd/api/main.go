package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/cart"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/catalog"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/database"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/identity"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/inventory"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/notification"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/payment"
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
	connection, err := database.OpenGORMFromEnv(logger)
	if err != nil {
		logger.Error("Datenbankkonfiguration ungültig", "event", "config.invalid")
		os.Exit(1)
	}
	defer connection.Close()
	if appEnv != "development" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var elevated, schemaCreate, databaseCreate, ownsTables, roleMembership bool
		err := connection.SQL.QueryRowContext(ctx, `SELECT
			rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls,
			has_schema_privilege(current_user, 'public', 'CREATE'),
			has_database_privilege(current_user, current_database(), 'CREATE'),
			EXISTS (SELECT 1 FROM pg_class WHERE relnamespace = 'public'::regnamespace
				AND relowner = current_user::regrole),
			EXISTS (SELECT 1 FROM pg_auth_members WHERE member = current_user::regrole)
			FROM pg_roles WHERE rolname = current_user`).Scan(&elevated, &schemaCreate, &databaseCreate, &ownsTables, &roleMembership)
		cancel()
		if err != nil || elevated || schemaCreate || databaseCreate || ownsTables || roleMembership {
			logger.Error("API benötigt eine Datenbankrolle ohne Verwaltungs- und DDL-Rechte", "event", "db.role.unsafe")
			os.Exit(1)
		}
	}

	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = "NordWinds"
	}
	router, err := server.New(appName, connection.SQL, logger, os.Getenv("NORDWINDS_METRICS_TOKEN"))
	if err != nil {
		logger.Error("Router konnte nicht erstellt werden", "event", "router.failed")
		os.Exit(1)
	}
	server.RegisterCatalogRoutes(router, catalog.NewRepository(connection.SQL))
	publicOrigin := os.Getenv("APP_PUBLIC_ORIGIN")
	if appEnv != "development" {
		parsed, err := url.Parse(publicOrigin)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			logger.Error("Öffentliche HTTPS-Origin fehlt oder ist ungültig", "event", "config.invalid")
			os.Exit(1)
		}
	}
	identityService, err := identity.NewService(identity.NewRepository(connection.SQL))
	if err != nil {
		logger.Error("Authentifizierung konnte nicht initialisiert werden", "event", "auth.init.failed")
		os.Exit(1)
	}
	authConfig := server.AuthConfig{
		PublicOrigin: publicOrigin, SecureCookie: appEnv != "development", Logger: logger,
		TrustedProxyClientIP: os.Getenv("NORDWINDS_TRUST_PROXY_CLIENT_IP") == "true",
	}
	if appEnv == "development" {
		if host := os.Getenv("SMTP_HOST"); host != "" {
			port := os.Getenv("SMTP_PORT")
			if port == "" {
				port = "1025"
			}
			authConfig.RegistrationNotifier = notification.SMTP{Address: net.JoinHostPort(host, port), From: "no-reply@nordwinds.local"}
		}
	}
	authHandler := server.RegisterAuthRoutes(router, identityService, authConfig)
	server.RegisterAdminRoutes(router, authHandler, catalog.NewAdminRepository(connection.SQL))
	server.RegisterMediaRoutes(router, catalog.NewRepository(connection.SQL))
	server.RegisterCartRoutes(router, authHandler, cart.NewRepository(connection.SQL))
	server.RegisterDiscountAdminRoutes(router, authHandler, cart.NewDiscountAdmin(connection.SQL))
	orderRepository := orders.NewRepository(connection.SQL)
	// Ohne echten Betreiber und geprüfte Rechtstexte darf die Vorschau keine Livebestellungen anlegen.
	server.RegisterOrderRoutes(router, authHandler, orderRepository,
		demoCheckoutEnabled(appEnv, os.Getenv("NORDWINDS_DEMO_CHECKOUT_ENABLED")))
	server.RegisterAdminOrderRoutes(router, authHandler, orderRepository, logger)
	server.RegisterAccountRoutes(router, authHandler, orderRepository)
	mockPayments := payment.NewRepository(connection.SQL)
	if appEnv == "development" {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			logger.Error("Mock-Signatur konnte nicht initialisiert werden", "event", "mock.init.failed")
			os.Exit(1)
		}
		if configured := os.Getenv("NORDWINDS_MOCK_WEBHOOK_SECRET"); configured != "" {
			if len(configured) < 32 {
				logger.Error("Mock-Signaturschlüssel zu kurz", "event", "config.invalid")
				os.Exit(1)
			}
			secret = []byte(configured)
		}
		server.RegisterMockPaymentRoutes(router, authHandler, mockPayments, secret)
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
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			if _, err := inventory.ReleaseExpired(ctx, connection.SQL, time.Now(), 100); err != nil && ctx.Err() == nil {
				logger.Error("Abgelaufene Reservierungen konnten nicht freigegeben werden", "event", "inventory.expiry.failed")
			}
			if appEnv == "development" {
				if err := mockPayments.SyncExpired(ctx); err != nil && ctx.Err() == nil {
					logger.Error("Mock-Zahlungen konnten nicht abgeglichen werden", "event", "mock.expiry.failed")
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
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

func demoCheckoutEnabled(environment, flag string) bool {
	return environment == "development" && flag == "true"
}
