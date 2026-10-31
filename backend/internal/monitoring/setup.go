package monitoring

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	caValidity     = 10 * 365 * 24 * time.Hour
	serverValidity = 397 * 24 * time.Hour
	renewBefore    = 30 * 24 * time.Hour
)

// Report fasst zusammen, was ein Lauf erzeugt hat. Es enthält keine Geheimnisse.
type Report struct {
	CAFingerprint   string
	CACreated       bool
	ServerRenewed   bool
	ServerNotAfter  time.Time
	GrafanaKeyFresh bool
}

// Run legt fehlende Schlüssel an, erneuert ablaufende Zertifikate und schreibt alle
// Dateien für Prometheus und Grafana atomar mit passenden Eigentümern und Rechten neu.
func Run(config Config) (Report, error) {
	var report Report
	now := config.Now()
	if err := os.MkdirAll(config.StateDir, 0o700); err != nil {
		return report, err
	}
	if err := os.Chmod(config.StateDir, 0o700); err != nil {
		return report, err
	}
	for _, dir := range []string{config.PrometheusDir, config.GrafanaDir,
		filepath.Join(config.GrafanaDir, "provisioning", "datasources"), filepath.Join(config.GrafanaDir, "provisioning", "dashboards"),
		filepath.Join(config.GrafanaDir, "provisioning", "plugins"), filepath.Join(config.GrafanaDir, "provisioning", "alerting")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return report, err
		}
	}

	caCert, caKey, err := loadPair(config.StateDir, "ca")
	if err != nil || !caCert.IsCA || caCert.NotAfter.Before(now.Add(renewBefore)) {
		caCert, caKey, err = createCA(config, now)
		if err != nil {
			return report, err
		}
		report.CACreated = true
	}
	serverCert, serverKey, err := loadPair(config.StateDir, "server")
	if report.CACreated || err != nil || !serverValid(serverCert, caCert, config.Hosts, now) {
		serverCert, serverKey, err = createServer(config, caCert, caKey, now)
		if err != nil {
			return report, err
		}
		report.ServerRenewed = true
	}
	report.ServerNotAfter = serverCert.NotAfter
	digest := sha256.Sum256(caCert.Raw)
	report.CAFingerprint = strings.ToUpper(hex.EncodeToString(digest[:]))

	secretKey, err := os.ReadFile(filepath.Join(config.StateDir, "grafana_secret_key"))
	if err != nil || len(secretKey) < 32 {
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return report, err
		}
		secretKey = []byte(base64.RawURLEncoding.EncodeToString(random))
		if err := writeFile(filepath.Join(config.StateDir, "grafana_secret_key"), secretKey, 0o600, -1, -1); err != nil {
			return report, err
		}
		report.GrafanaKeyFresh = true
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(config.PrometheusPassword), bcrypt.DefaultCost)
	if err != nil {
		return report, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	serverPEM := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverCert.Raw}), caPEM...)
	serverKeyPEM, err := encodeKey(serverKey)
	if err != nil {
		return report, err
	}
	mount := config.Mount
	prometheus := []publishedFile{
		{"ca.crt", caPEM, 0o444},
		{"tls.crt", serverPEM, 0o444},
		{"tls.key", serverKeyPEM, 0o400},
		{"web-config.yml", []byte(webConfig(mount, config.PrometheusUser, string(hash))), 0o400},
		{"self_scrape_user", []byte(config.PrometheusUser), 0o400},
		{"self_scrape_password", []byte(config.PrometheusPassword), 0o400},
		{"metrics_token", []byte(config.MetricsToken), 0o400},
		{"http-client.yml", []byte(httpClientConfig(mount)), 0o400},
	}
	for _, file := range prometheus {
		if err := writeFile(filepath.Join(config.PrometheusDir, file.name), file.data, file.mode, config.PrometheusUID, config.PrometheusGID); err != nil {
			return report, err
		}
	}
	grafana := []publishedFile{
		{"ca.crt", caPEM, 0o444},
		{"admin_password", []byte(config.GrafanaAdminPassword), 0o400},
		{"secret_key", secretKey, 0o400},
		{"provisioning/datasources/prometheus.yaml", []byte(datasourceConfig(config.PrometheusUser, config.PrometheusPassword, string(caPEM))), 0o400},
		{"provisioning/dashboards/nordwinds.yaml", []byte(dashboardProvider(config.DashboardDir)), 0o444},
	}
	for _, file := range grafana {
		if err := writeFile(filepath.Join(config.GrafanaDir, file.name), file.data, file.mode, config.GrafanaUID, config.GrafanaGID); err != nil {
			return report, err
		}
	}
	return report, nil
}

type publishedFile struct {
	name string
	data []byte
	mode os.FileMode
}

func createCA(config Config, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	dns, ips := splitHosts(config.Hosts)
	ranges := make([]*net.IPNet, 0, len(ips))
	for _, ip := range ips {
		bits := 8 * len(ip)
		if v4 := ip.To4(); v4 != nil {
			ip, bits = v4, 32
		}
		ranges = append(ranges, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "NordWinds Monitoring CA (lokal)", Organization: []string{"NordWinds"}},
		NotBefore:    now.Add(-time.Hour), NotAfter: now.Add(caValidity),
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:     true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true,
		// Selbst wenn jemand die CA ins System übernimmt, kann sie nur Zertifikate
		// für die Monitoring-Namen ausstellen, nie für fremde Websites.
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         dns,
		PermittedIPRanges:           ranges,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, storePair(config.StateDir, "ca", cert, key)
}

func createServer(config Config, ca *x509.Certificate, caKey *ecdsa.PrivateKey, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	dns, ips := splitHosts(config.Hosts)
	template := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: dns[0], Organization: []string{"NordWinds"}},
		NotBefore:    now.Add(-time.Hour), NotAfter: now.Add(serverValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dns,
		IPAddresses:           ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, storePair(config.StateDir, "server", cert, key)
}

func serverValid(cert, ca *x509.Certificate, hosts []string, now time.Time) bool {
	if cert.NotAfter.Before(now.Add(renewBefore)) {
		return false
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	for _, host := range hosts {
		if _, err := cert.Verify(x509.VerifyOptions{DNSName: host, Roots: roots, CurrentTime: now,
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			return false
		}
	}
	return true
}

func splitHosts(hosts []string) (dns []string, ips []net.IP) {
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			ips = append(ips, ip)
		} else {
			dns = append(dns, host)
		}
	}
	return dns, ips
}

func randomSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(err)
	}
	return serial.Add(serial, big.NewInt(1))
}

func encodeKey(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func storePair(dir, name string, cert *x509.Certificate, key *ecdsa.PrivateKey) error {
	keyPEM, err := encodeKey(key)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(dir, name+".key"), keyPEM, 0o600, -1, -1); err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, name+".crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600, -1, -1)
}

func loadPair(dir, name string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(filepath.Join(dir, name+".crt"))
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, name+".key"))
	if err != nil {
		return nil, nil, err
	}
	certBlock, _ := pem.Decode(certPEM)
	keyBlock, _ := pem.Decode(keyPEM)
	if certBlock == nil || keyBlock == nil {
		return nil, nil, errors.New("PEM ungültig")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || !key.PublicKey.Equal(cert.PublicKey) {
		return nil, nil, errors.New("Schlüssel passt nicht zum Zertifikat")
	}
	return cert, key, nil
}

// writeFile ersetzt eine Datei atomar. Rechte werden vor dem Eigentümerwechsel gesetzt,
// weil root ohne CAP_FOWNER fremde Dateien nicht mehr ändern darf.
func writeFile(path string, data []byte, mode os.FileMode, uid, gid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if current, err := os.ReadFile(path); err == nil && bytes.Equal(current, data) && uid < 0 {
		return nil
	}
	temp, err := os.CreateTemp(dir, ".monitoring-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		return err
	}
	if uid >= 0 {
		if err := os.Chown(name, uid, gid); err != nil {
			return fmt.Errorf("Eigentümer von %s: %w", filepath.Base(path), err)
		}
	}
	return os.Rename(name, path)
}
