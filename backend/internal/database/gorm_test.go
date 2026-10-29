package database

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestOpenGORMFromEnvOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_GORM_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_GORM_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Scheme != "postgres" || !strings.HasPrefix(strings.TrimPrefix(parsed.Path, "/"), "nordwinds_test_") {
		t.Fatal("GORM-Test verlangt eine separate Datenbank nordwinds_test_*")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	t.Setenv("POSTGRES_HOST", host)
	t.Setenv("POSTGRES_PORT", port)
	t.Setenv("POSTGRES_DB", strings.TrimPrefix(parsed.Path, "/"))
	t.Setenv("POSTGRES_USER", parsed.User.Username())
	t.Setenv("POSTGRES_PASSWORD", password)
	t.Setenv("POSTGRES_SSLMODE", "disable")

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	connection, err := OpenGORMFromEnv(logger)
	if err != nil {
		t.Fatalf("GORM-Initialisierung: %v", err)
	}
	defer connection.Close()
	pool, err := connection.ORM.DB()
	if err != nil || pool != connection.SQL {
		t.Fatalf("GORM nutzt einen anderen Pool: %v", err)
	}
	if pool.Stats().MaxOpenConnections != 5 {
		t.Fatalf("MaxOpenConns = %d", pool.Stats().MaxOpenConnections)
	}
	if !connection.ORM.Config.TranslateError || !connection.ORM.Config.DisableAutomaticPing {
		t.Fatal("GORM-Konfiguration weicht vom sicheren Setup ab")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var value int
	if err := connection.ORM.WithContext(ctx).Raw(`SELECT 1`).Scan(&value).Error; err != nil || value != 1 {
		t.Fatalf("GORM-Abfrage: %d, %v", value, err)
	}
	if err := pool.QueryRowContext(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname = 'public'`).Scan(&after); err != nil || before != after {
		t.Fatalf("GORM-Setup änderte Schema: %d → %d, %v", before, after, err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := connection.ORM.WithContext(cancelled).Exec(`SELECT 1`).Error; !errors.Is(err, context.Canceled) {
		t.Fatalf("Context-Abbruch wurde nicht weitergegeben: %v", err)
	}
	if strings.Contains(output.String(), "SELECT") || strings.Contains(output.String(), password) {
		t.Fatal("SQL oder Zugangsdaten im GORM-Log")
	}
}

func TestSafeGORMLoggerRedactsSQLAndArguments(t *testing.T) {
	const secret = "synthetic-token-and-address"
	var output bytes.Buffer
	logger := newSafeGORMLogger(slog.New(slog.NewJSONHandler(&output, nil)))
	ctx := context.Background()
	called := false
	logger.Trace(ctx, time.Now().Add(-2*time.Second), func() (string, int64) {
		called = true
		return "SELECT '" + secret + "'", 1
	}, nil)
	logger.Error(ctx, "SQL %s", secret)
	logger.Warn(ctx, "SQL %s", secret)
	logger.LogMode(gormlogger.Info).Info(ctx, "SQL %s", secret)
	logger.Trace(ctx, time.Now(), func() (string, int64) {
		called = true
		return secret, 0
	}, gorm.ErrRecordNotFound)
	if called || strings.Contains(output.String(), secret) || strings.Contains(output.String(), "SELECT") {
		t.Fatalf("Sensible SQL-Daten wurden verarbeitet oder protokolliert: %s", output.String())
	}
	for _, event := range []string{"database.query_slow", "database.gorm_error", "database.gorm_warning", "database.gorm_info"} {
		if !strings.Contains(output.String(), event) {
			t.Fatalf("Strukturiertes Ereignis %s fehlt: %s", event, output.String())
		}
	}
}

func TestOpenGORMFromEnvRequiresLoggerAndPassword(t *testing.T) {
	if _, err := OpenGORMFromEnv(nil); err == nil {
		t.Fatal("GORM benötigt einen strukturierten Logger")
	}
	t.Setenv("POSTGRES_PASSWORD", "")
	if _, err := OpenGORMFromEnv(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))); err == nil {
		t.Fatal("Fehlendes Passwort wurde akzeptiert")
	}
}
