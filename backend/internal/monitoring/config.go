// Package monitoring erzeugt die Zugangsdaten und TLS-Zertifikate für Prometheus und
// Grafana (ADR-012). Es läuft als einmaliger Container vor beiden Diensten.
package monitoring

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// Config beschreibt Quell- und Zielverzeichnisse sowie die Zugangsdaten aus .env.
type Config struct {
	// StateDir enthält CA-Schlüssel und dauerhafte Geheimnisse; nur root kann es lesen.
	StateDir string
	// PrometheusDir und GrafanaDir werden in die jeweiligen Container eingebunden.
	PrometheusDir, GrafanaDir string
	// Mount ist der Pfad, unter dem beide Container ihr Verzeichnis sehen.
	Mount string
	// DashboardDir ist der Pfad der Dashboard-Dateien im Grafana-Container.
	DashboardDir string
	// Eigentümer der veröffentlichten Dateien; negative Werte lassen den Eigentümer unverändert.
	PrometheusUID, PrometheusGID, GrafanaUID, GrafanaGID int

	PrometheusUser, PrometheusPassword string
	GrafanaAdminPassword               string
	MetricsToken                       string
	// Hosts sind die Namen und Adressen im Prometheus-Zertifikat.
	Hosts []string
	Now   func() time.Time
}

var (
	userPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)
	rolePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{2,62}$`)
)

// ConfigFromEnv liest und prüft die Werte aus der Compose-Umgebung. Fehler nennen nur
// die Variable, nie ihren Inhalt.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	config := Config{
		StateDir: "/monitoring/state", PrometheusDir: "/monitoring/prometheus", GrafanaDir: "/monitoring/grafana",
		Mount: "/run/monitoring", DashboardDir: "/etc/nordwinds/dashboards",
		// prom/prometheus läuft als nobody, grafana/grafana als grafana (472) in der Gruppe root.
		PrometheusUID: 65534, PrometheusGID: 65534, GrafanaUID: 472, GrafanaGID: 0,
		PrometheusUser:       strings.TrimSpace(getenv("PROMETHEUS_USER")),
		PrometheusPassword:   getenv("PROMETHEUS_PASSWORD"),
		GrafanaAdminPassword: getenv("GRAFANA_ADMIN_PASSWORD"),
		MetricsToken:         getenv("NORDWINDS_METRICS_TOKEN"),
		Hosts:                []string{"prometheus", "localhost", "127.0.0.1", "::1"},
		Now:                  time.Now,
	}
	var problems []error
	if !userPattern.MatchString(config.PrometheusUser) {
		problems = append(problems, errors.New("PROMETHEUS_USER: 3–64 Zeichen aus Buchstaben, Ziffern, . _ -"))
	}
	if err := checkSecret("PROMETHEUS_PASSWORD", config.PrometheusPassword, 16); err != nil {
		problems = append(problems, err)
	} else if len(config.PrometheusPassword) > 72 {
		problems = append(problems, errors.New("PROMETHEUS_PASSWORD: höchstens 72 Byte (Grenze von bcrypt)"))
	}
	if !userPattern.MatchString(strings.TrimSpace(getenv("GRAFANA_ADMIN_USER"))) {
		problems = append(problems, errors.New("GRAFANA_ADMIN_USER: 3–64 Zeichen aus Buchstaben, Ziffern, . _ -"))
	}
	if err := checkSecret("GRAFANA_ADMIN_PASSWORD", config.GrafanaAdminPassword, 16); err != nil {
		problems = append(problems, err)
	} else if strings.EqualFold(config.GrafanaAdminPassword, "admin") {
		problems = append(problems, errors.New("GRAFANA_ADMIN_PASSWORD: Standardpasswort nicht erlaubt"))
	}
	if err := checkSecret("NORDWINDS_METRICS_TOKEN", config.MetricsToken, 32); err != nil {
		problems = append(problems, err)
	}
	// Die Exporter-Rolle legt migrate an; hier wird nur früh auf fehlende Werte hingewiesen.
	if role := getenv("NORDWINDS_DB_MONITOR_USER"); role != "" && !rolePattern.MatchString(role) {
		problems = append(problems, errors.New("NORDWINDS_DB_MONITOR_USER: Kleinbuchstaben, Ziffern und _"))
	}
	if err := checkSecret("NORDWINDS_DB_MONITOR_PASSWORD", getenv("NORDWINDS_DB_MONITOR_PASSWORD"), 32); err != nil {
		problems = append(problems, err)
	}
	if len(problems) > 0 {
		return Config{}, errors.Join(problems...)
	}
	return config, nil
}

func checkSecret(name, value string, minimum int) error {
	switch {
	case value == "":
		return fmt.Errorf("%s fehlt", name)
	case strings.HasPrefix(value, "hier-"):
		return fmt.Errorf("%s ist noch der Platzhalter aus .env.example", name)
	case len(value) < minimum:
		return fmt.Errorf("%s: mindestens %d Zeichen", name, minimum)
	case strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0:
		return fmt.Errorf("%s: keine Leer- oder Steuerzeichen", name)
	}
	return nil
}
