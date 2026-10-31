package notification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestConfigFromEnv(t *testing.T) {
	local := map[string]string{"MAIL_TRANSPORT": "smtp", "SMTP_HOST": "mailpit", "SMTP_PORT": "1025",
		"SMTP_TLS": "none", "MAIL_FROM": "NordWinds <no-reply@nordwinds.local>"}
	production := map[string]string{"MAIL_TRANSPORT": "smtp", "SMTP_HOST": "smtp.anbieter.de", "SMTP_TLS": "starttls",
		"SMTP_USERNAME": "shop", "SMTP_PASSWORD": "geheim-aus-dem-tresor", "MAIL_FROM": "NordWinds <no-reply@nordwinds.de>",
		"MAIL_REPLY_TO": "service@nordwinds.de"}
	with := func(base map[string]string, changes map[string]string) map[string]string {
		result := map[string]string{}
		for key, value := range base {
			result[key] = value
		}
		for key, value := range changes {
			if value == "-" {
				delete(result, key)
				continue
			}
			result[key] = value
		}
		return result
	}
	for _, tc := range []struct {
		name        string
		values      map[string]string
		environment string
		origin      string
		ok          bool
	}{
		{"ohne Konfiguration deaktiviert", map[string]string{}, "production", "https://shop.de", true},
		{"lokal Mailpit", local, "development", "http://localhost:8080", true},
		{"Staging Mailpit", local, "staging", "https://staging.localhost:18443", true},
		{"lokal externer Server ohne Freigabe", with(local, map[string]string{"SMTP_HOST": "smtp.anbieter.de", "SMTP_TLS": "starttls"}), "development", "http://localhost:8080", false},
		{"lokal externer Server mit Freigabe", with(local, map[string]string{"SMTP_HOST": "smtp.anbieter.de", "SMTP_TLS": "starttls", "MAIL_ALLOW_EXTERNAL_DELIVERY": "true"}), "development", "http://localhost:8080", true},
		{"lokal externer Server unverschlüsselt", with(local, map[string]string{"SMTP_HOST": "smtp.anbieter.de", "MAIL_ALLOW_EXTERNAL_DELIVERY": "true"}), "development", "http://localhost:8080", false},
		{"Produktion gültig", production, "production", "https://shop.de", true},
		{"Produktion mit Mailpit", with(production, map[string]string{"SMTP_HOST": "mailpit"}), "production", "https://shop.de", false},
		{"Produktion mit Port 1025", with(production, map[string]string{"SMTP_PORT": "1025"}), "production", "https://shop.de", false},
		{"Produktion unverschlüsselt", with(production, map[string]string{"SMTP_TLS": "none"}), "production", "https://shop.de", false},
		{"Produktion ohne Zugangsdaten", with(production, map[string]string{"SMTP_USERNAME": "-", "SMTP_PASSWORD": "-"}), "production", "https://shop.de", false},
		{"Produktion mit Testabsender", with(production, map[string]string{"MAIL_FROM": "no-reply@nordwinds.local"}), "production", "https://shop.de", false},
		{"Produktion mit example.com", with(production, map[string]string{"MAIL_FROM": "no-reply@shop.example.com"}), "production", "https://shop.de", false},
		{"Produktion ohne HTTPS-Origin", production, "production", "http://shop.de", false},
		{"unbekannter Transport", with(local, map[string]string{"MAIL_TRANSPORT": "sendmail"}), "development", "http://localhost:8080", false},
		{"ungültiger Port", with(local, map[string]string{"SMTP_PORT": "70000"}), "development", "http://localhost:8080", false},
		{"ungültiger TLS-Modus", with(local, map[string]string{"SMTP_TLS": "ssl3"}), "development", "http://localhost:8080", false},
		{"Absender mit Zeilenumbruch", with(local, map[string]string{"MAIL_FROM": "a@b.de\r\nBcc: x@y.de"}), "development", "http://localhost:8080", false},
		{"Benutzer ohne Passwort", with(local, map[string]string{"SMTP_USERNAME": "shop"}), "development", "http://localhost:8080", false},
		{"Origin mit Pfad", local, "development", "http://localhost:8080/shop", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := ConfigFromEnv(env(tc.values), tc.environment, tc.origin)
			if tc.ok != (err == nil) {
				t.Fatalf("ok=%v erwartet, Fehler %v", tc.ok, err)
			}
			if err != nil && !errors.Is(err, ErrConfig) {
				t.Fatalf("Fehlerart: %v", err)
			}
			if err == nil && config.Preview != (tc.environment != "production") {
				t.Fatalf("Vorschau-Kennzeichen falsch: %+v", config)
			}
		})
	}

	config, err := ConfigFromEnv(env(production), "production", "https://shop.de")
	if err != nil || config.Port != 587 || config.TLS != TLSStartTLS || config.ReplyTo == nil || config.From.Name != "NordWinds" {
		t.Fatalf("Standardwerte: %+v, %v", config, err)
	}
	if config, _ := ConfigFromEnv(env(with(production, map[string]string{"SMTP_TLS": "tls"})), "production", "https://shop.de"); config.Port != 465 {
		t.Fatalf("Port für implizites TLS: %d", config.Port)
	}
}

func TestConfigPasswordFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "smtp_password")
	if err := os.WriteFile(file, []byte("aus-der-datei\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{"MAIL_TRANSPORT": "smtp", "SMTP_HOST": "smtp.anbieter.de", "SMTP_USERNAME": "shop",
		"SMTP_PASSWORD_FILE": file, "MAIL_FROM": "no-reply@nordwinds.de"}
	config, err := ConfigFromEnv(env(values), "production", "https://shop.de")
	if err != nil || config.Password != "aus-der-datei" {
		t.Fatalf("Passwortdatei: %q, %v", config.Password, err)
	}
	values["SMTP_PASSWORD"] = "doppelt"
	if _, err := ConfigFromEnv(env(values), "production", "https://shop.de"); err == nil {
		t.Fatal("Passwort und Passwortdatei gleichzeitig akzeptiert")
	}
	values["SMTP_PASSWORD"] = ""
	values["SMTP_PASSWORD_FILE"] = filepath.Join(t.TempDir(), "fehlt")
	if _, err := ConfigFromEnv(env(values), "production", "https://shop.de"); err == nil {
		t.Fatal("fehlende Passwortdatei akzeptiert")
	}
}
