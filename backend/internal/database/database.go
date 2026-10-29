package database

import (
	"database/sql"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenFromEnv erstellt einen Pool für die interne PostgreSQL-Verbindung.
func OpenFromEnv() (*sql.DB, error) {
	db, err := openPoolFromEnv()
	if err != nil {
		return nil, err
	}
	configurePool(db)
	return db, nil
}

func openPoolFromEnv() (*sql.DB, error) {
	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		return nil, errors.New("POSTGRES_PASSWORD fehlt")
	}
	port := envOr("POSTGRES_PORT", "5432")
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("POSTGRES_PORT ist ungültig")
	}
	connection := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(envOr("POSTGRES_USER", "nordwinds_app"), password),
		Host:   net.JoinHostPort(envOr("POSTGRES_HOST", "db"), port),
		Path:   "/" + envOr("POSTGRES_DB", "nordwinds"),
	}
	query := connection.Query()
	query.Set("sslmode", envOr("POSTGRES_SSLMODE", "disable"))
	query.Set("connect_timeout", "5")
	connection.RawQuery = query.Encode()

	db, err := sql.Open("pgx", connection.String())
	if err != nil {
		return nil, err
	}
	return db, nil
}

func configurePool(db *sql.DB) {
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(10 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
