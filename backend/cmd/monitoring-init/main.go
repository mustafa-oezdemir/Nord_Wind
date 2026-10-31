// monitoring-init bereitet Zertifikate und Zugangsdaten für Prometheus und Grafana vor
// (ADR-012). Der Container läuft vor beiden Diensten und beendet sich danach.
package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/monitoring"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := monitoring.ConfigFromEnv(os.Getenv)
	if err != nil {
		// Die Meldungen nennen nur Variablennamen und Regeln, nie Werte.
		logger.Error("Monitoring-Konfiguration in .env unvollständig", "event", "monitoring.init.invalid",
			"problems", strings.Split(err.Error(), "\n"))
		os.Exit(1)
	}
	report, err := monitoring.Run(config)
	if err != nil {
		logger.Error("Monitoring-Dateien konnten nicht erstellt werden", "event", "monitoring.init.failed", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("Monitoring vorbereitet", "event", "monitoring.init.ok",
		"ca_sha256", report.CAFingerprint, "ca_created", report.CACreated,
		"server_certificate_renewed", report.ServerRenewed, "server_certificate_not_after", report.ServerNotAfter,
		"grafana_secret_key_created", report.GrafanaKeyFresh)
}
