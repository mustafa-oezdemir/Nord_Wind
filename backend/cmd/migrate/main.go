package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/database"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down" && os.Args[1] != "status") {
		logger.Error("Aufruf: migrate up|down|status")
		os.Exit(2)
	}
	db, err := database.OpenFromEnv()
	if err != nil {
		logger.Error("Datenbankkonfiguration ungültig", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if os.Args[1] == "up" {
		err = migrations.Up(ctx, db)
		if err == nil {
			if password := os.Getenv("NORDWINDS_DB_RUNTIME_PASSWORD"); password != "" {
				err = migrations.GrantRuntime(ctx, db, password)
			}
		}
		// Rolle für den PostgreSQL-Exporter (ADR-012), nur wenn ein Passwort gesetzt ist.
		if password := os.Getenv("NORDWINDS_DB_MONITOR_PASSWORD"); err == nil && password != "" {
			role := os.Getenv("NORDWINDS_DB_MONITOR_USER")
			if role == "" {
				role = "nordwinds_monitor"
			}
			err = migrations.GrantMonitor(ctx, db, role, password)
		}
	} else if os.Args[1] == "down" {
		err = migrations.Down(ctx, db)
	} else {
		err = migrations.Check(ctx, db)
	}
	if err != nil {
		logger.Error("Migration fehlgeschlagen", "error", err)
		os.Exit(1)
	}
	logger.Info("Migration geprüft", "command", os.Args[1])
}
