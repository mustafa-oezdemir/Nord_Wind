package monitoring

import (
	"strings"
)

const generatedNotice = "# Erzeugt von monitoring-init (ADR-012) und bei jedem Start neu geschrieben. Nicht von Hand ändern.\n"

// quote erzeugt einen YAML-String in einfachen Anführungszeichen.
func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// grafanaLiteral verhindert, dass Grafana $ in Provisioning-Werten als Umgebungsvariable liest.
func grafanaLiteral(value string) string { return quote(strings.ReplaceAll(value, "$", "$$")) }

// webConfig ist die Web-Konfiguration von Prometheus: nur TLS, nur mit Anmeldung.
// Prometheus liest sie bei jeder Anfrage neu; geänderte Passwörter wirken sofort.
func webConfig(mount, user, hash string) string {
	return generatedNotice + `tls_server_config:
  cert_file: ` + mount + `/tls.crt
  key_file: ` + mount + `/tls.key
  min_version: TLS12
http_server_config:
  headers:
    X-Content-Type-Options: nosniff
    X-Frame-Options: deny
basic_auth_users:
  ` + quote(user) + `: ` + quote(hash) + "\n"
}

// httpClientConfig nutzt promtool im Healthcheck des Prometheus-Containers.
func httpClientConfig(mount string) string {
	return generatedNotice + `basic_auth:
  username_file: ` + mount + `/self_scrape_user
  password_file: ` + mount + `/self_scrape_password
tls_config:
  ca_file: ` + mount + `/ca.crt
`
}

// datasourceConfig richtet Prometheus als einzige, nicht änderbare Datenquelle ein.
// Grafana prüft das Zertifikat gegen die lokale CA und den Namen "prometheus".
func datasourceConfig(user, password, caPEM string) string {
	indented := "        " + strings.ReplaceAll(strings.TrimSpace(caPEM), "\n", "\n        ")
	return generatedNotice + `apiVersion: 1
datasources:
  - name: Prometheus
    uid: nordwinds-prometheus
    type: prometheus
    access: proxy
    url: https://prometheus:9090
    isDefault: true
    editable: false
    basicAuth: true
    basicAuthUser: ` + grafanaLiteral(user) + `
    jsonData:
      httpMethod: POST
      prometheusType: Prometheus
      timeInterval: 15s
      tlsAuthWithCACert: true
      serverName: prometheus
    secureJsonData:
      basicAuthPassword: ` + grafanaLiteral(password) + `
      tlsCACert: |
` + indented + "\n"
}

func dashboardProvider(path string) string {
	return generatedNotice + `apiVersion: 1
providers:
  - name: nordwinds
    orgId: 1
    folder: NordWinds
    type: file
    disableDeletion: true
    allowUiUpdates: false
    updateIntervalSeconds: 60
    options:
      path: ` + path + "\n"
}
