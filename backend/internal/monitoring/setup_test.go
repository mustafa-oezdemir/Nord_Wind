package monitoring

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func validEnv() map[string]string {
	return map[string]string{
		"PROMETHEUS_USER": "nordwinds_prometheus", "PROMETHEUS_PASSWORD": "Prometheus-Testpasswort-2026",
		"GRAFANA_ADMIN_USER": "nordwinds_admin", "GRAFANA_ADMIN_PASSWORD": "Grafana-Testpasswort-2026",
		"NORDWINDS_METRICS_TOKEN":       strings.Repeat("m", 40),
		"NORDWINDS_DB_MONITOR_USER":     "nordwinds_monitor",
		"NORDWINDS_DB_MONITOR_PASSWORD": strings.Repeat("d", 40),
	}
}

func TestConfigFromEnvRejectsMissingWeakAndPlaceholderValues(t *testing.T) {
	env := validEnv()
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("gültige Werte abgelehnt: %v", err)
	}
	for key, value := range map[string]string{
		"PROMETHEUS_USER":               "a b",
		"PROMETHEUS_PASSWORD":           "kurz",
		"GRAFANA_ADMIN_USER":            "",
		"GRAFANA_ADMIN_PASSWORD":        "hier-ein-eigenes-starkes-passwort-setzen",
		"NORDWINDS_METRICS_TOKEN":       "",
		"NORDWINDS_DB_MONITOR_USER":     "Monitor-Rolle",
		"NORDWINDS_DB_MONITOR_PASSWORD": strings.Repeat("x", 31),
	} {
		broken := validEnv()
		broken[key] = value
		_, err := ConfigFromEnv(func(k string) string { return broken[k] })
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q: %v", key, value, err)
		}
		if err != nil && value != "" && len(value) > 4 && strings.Contains(err.Error(), value) {
			t.Errorf("%s: Fehlermeldung enthält den Wert", key)
		}
	}
	broken := validEnv()
	broken["PROMETHEUS_PASSWORD"] = strings.Repeat("p", 73)
	if _, err := ConfigFromEnv(func(k string) string { return broken[k] }); err == nil {
		t.Error("Passwort über 72 Byte akzeptiert")
	}
	broken = validEnv()
	broken["GRAFANA_ADMIN_PASSWORD"] = "Grafana Passwort mit Leerzeichen"
	if _, err := ConfigFromEnv(func(k string) string { return broken[k] }); err == nil {
		t.Error("Leerzeichen akzeptiert")
	}
}

func testConfig(t *testing.T) Config {
	t.Helper()
	env := validEnv()
	config, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	config.StateDir, config.PrometheusDir, config.GrafanaDir = filepath.Join(root, "state"), filepath.Join(root, "prometheus"), filepath.Join(root, "grafana")
	if os.Geteuid() != 0 {
		config.PrometheusUID, config.PrometheusGID, config.GrafanaUID, config.GrafanaGID = -1, -1, -1, -1
	}
	return config
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRunCreatesTLSAndCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("monitoring-init läuft nur im Linux-Container")
	}
	config := testConfig(t)
	first, err := Run(config)
	if err != nil {
		t.Fatal(err)
	}
	if !first.CACreated || !first.ServerRenewed || !first.GrafanaKeyFresh || len(first.CAFingerprint) != 64 {
		t.Fatalf("erster Lauf: %+v", first)
	}

	// Rechte und Eigentümer: Schlüssel und Passwörter nur für den jeweiligen Dienst lesbar.
	for path, want := range map[string]os.FileMode{
		filepath.Join(config.StateDir, "ca.key"):                                     0o600,
		filepath.Join(config.PrometheusDir, "tls.key"):                               0o400,
		filepath.Join(config.PrometheusDir, "web-config.yml"):                        0o400,
		filepath.Join(config.PrometheusDir, "metrics_token"):                         0o400,
		filepath.Join(config.PrometheusDir, "ca.crt"):                                0o444,
		filepath.Join(config.GrafanaDir, "admin_password"):                           0o400,
		filepath.Join(config.GrafanaDir, "provisioning/datasources/prometheus.yaml"): 0o400,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s: Rechte %v, erwartet %v", path, info.Mode().Perm(), want)
		}
		if uid, ok := fileOwner(info); ok && os.Geteuid() == 0 {
			wantUID := map[bool]int{true: config.PrometheusUID, false: config.GrafanaUID}[strings.HasPrefix(path, config.PrometheusDir)]
			if strings.HasPrefix(path, config.StateDir) {
				wantUID = 0
			}
			if uid != wantUID {
				t.Errorf("%s: Eigentümer %d, erwartet %d", path, uid, wantUID)
			}
		}
	}
	if info, _ := os.Stat(config.StateDir); info.Mode().Perm() != 0o700 {
		t.Errorf("Zustandsverzeichnis: %v", info.Mode().Perm())
	}

	// Basic Auth: der Hash in web-config.yml passt zum Passwort aus .env.
	web := read(t, filepath.Join(config.PrometheusDir, "web-config.yml"))
	line := web[strings.Index(web, "'nordwinds_prometheus': ")+len("'nordwinds_prometheus': "):]
	hash := strings.Trim(strings.SplitN(line, "\n", 2)[0], "'")
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(config.PrometheusPassword)) != nil {
		t.Fatal("bcrypt-Hash passt nicht")
	}
	if strings.Contains(web, config.PrometheusPassword) || !strings.Contains(web, "cert_file: /run/monitoring/tls.crt") ||
		!strings.Contains(web, "min_version: TLS12") {
		t.Fatalf("web-config.yml:\n%s", web)
	}
	if read(t, filepath.Join(config.PrometheusDir, "metrics_token")) != config.MetricsToken {
		t.Fatal("Metrik-Token falsch")
	}

	// TLS: Zertifikat gilt für prometheus, localhost und 127.0.0.1, geprüft gegen die lokale CA.
	pair, err := tls.LoadX509KeyPair(filepath.Join(config.PrometheusDir, "tls.crt"), filepath.Join(config.PrometheusDir, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(read(t, filepath.Join(config.GrafanaDir, "ca.crt")))) {
		t.Fatal("CA nicht lesbar")
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	for _, name := range []string{"prometheus", "localhost", "127.0.0.1"} {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: name}}}
		response, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("TLS für %s: %v", name, err)
		}
		response.Body.Close()
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "shop.example.org"}}}
	if _, err := client.Get(server.URL); err == nil {
		t.Fatal("fremder Name akzeptiert")
	}

	// Namensbeschränkung: Auch mit dem CA-Schlüssel entsteht kein gültiges Zertifikat für fremde Namen.
	caCert, caKey, err := loadPair(config.StateDir, "ca")
	if err != nil {
		t.Fatal(err)
	}
	rogueKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rogueDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: randomSerial(),
		Subject: pkix.Name{CommonName: "bank.example"}, DNSNames: []string{"bank.example"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}, caCert, &rogueKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	rogue, _ := x509.ParseCertificate(rogueDER)
	if _, err := rogue.Verify(x509.VerifyOptions{DNSName: "bank.example", Roots: roots}); err == nil {
		t.Fatal("CA ohne Namensbeschränkung")
	}

	// Grafana: Datenquelle mit maskiertem $ und eingerückter CA; Admin-Passwort ohne Zeilenumbruch.
	source := read(t, filepath.Join(config.GrafanaDir, "provisioning/datasources/prometheus.yaml"))
	block, _ := pem.Decode([]byte(read(t, filepath.Join(config.GrafanaDir, "ca.crt"))))
	if block == nil || !strings.Contains(source, "url: https://prometheus:9090") || !strings.Contains(source, "        -----BEGIN CERTIFICATE-----") ||
		!strings.Contains(source, "basicAuthPassword: 'Prometheus-Testpasswort-2026'") {
		t.Fatalf("Datenquelle:\n%s", source)
	}
	if read(t, filepath.Join(config.GrafanaDir, "admin_password")) != config.GrafanaAdminPassword {
		t.Fatal("Admin-Passwort falsch")
	}

	// Zweiter Lauf: CA, Serverzertifikat und Grafana-Schlüssel bleiben; ein neues Passwort gilt sofort.
	secret := read(t, filepath.Join(config.GrafanaDir, "secret_key"))
	config.PrometheusPassword = "Neues-Pass$wort-mit-'Quote'-2026"
	second, err := Run(config)
	if err != nil {
		t.Fatal(err)
	}
	if second.CACreated || second.ServerRenewed || second.GrafanaKeyFresh || second.CAFingerprint != first.CAFingerprint {
		t.Fatalf("zweiter Lauf hat unnötig erneuert: %+v", second)
	}
	if read(t, filepath.Join(config.GrafanaDir, "secret_key")) != secret {
		t.Fatal("Grafana-Schlüssel gewechselt")
	}
	source = read(t, filepath.Join(config.GrafanaDir, "provisioning/datasources/prometheus.yaml"))
	if !strings.Contains(source, `basicAuthPassword: 'Neues-Pass$$wort-mit-''Quote''-2026'`) {
		t.Fatalf("Maskierung fehlt:\n%s", source)
	}

	// Kurz vor Ablauf wird nur das Serverzertifikat erneuert.
	config.Now = func() time.Time { return time.Now().Add(380 * 24 * time.Hour) }
	third, err := Run(config)
	if err != nil {
		t.Fatal(err)
	}
	if third.CACreated || !third.ServerRenewed || third.CAFingerprint != first.CAFingerprint || !third.ServerNotAfter.After(second.ServerNotAfter) {
		t.Fatalf("Erneuerung: %+v", third)
	}

	// Beschädigter Serverschlüssel: neues Paar statt Abbruch.
	if err := os.WriteFile(filepath.Join(config.StateDir, "server.key"), []byte("kaputt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fourth, err := Run(config); err != nil || !fourth.ServerRenewed {
		t.Fatalf("beschädigter Schlüssel: %+v %v", fourth, err)
	}
}
