// Package notification versendet transaktionale E-Mails aus der Outbox (ADR-011).
//
// Fachliche Ereignisse schreiben Datenbank-Trigger in notification_outbox, in derselben
// Transaktion wie das Ereignis selbst. Der Dispatcher holt fällige Einträge, rendert die
// deutschen Vorlagen und übergibt sie per SMTP. Zustellfehler werden mit Backoff wiederholt
// und beeinflussen nie die Bestellung oder das Konto.
package notification

import (
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// TLSMode bestimmt die Transportverschlüsselung zum SMTP-Server.
type TLSMode string

const (
	// TLSStartTLS verlangt STARTTLS nach dem Verbindungsaufbau (Port 587).
	TLSStartTLS TLSMode = "starttls"
	// TLSImplicit baut die Verbindung direkt per TLS auf (Port 465).
	TLSImplicit TLSMode = "tls"
	// TLSNone ist nur für lokale Testpostfächer wie Mailpit zulässig.
	TLSNone TLSMode = "none"
)

// Config ist die geprüfte Versandkonfiguration.
type Config struct {
	// Enabled ist false, wenn kein Versand konfiguriert ist; Benachrichtigungen bleiben dann
	// in der Outbox und werden nach dem Einschalten zugestellt.
	Enabled     bool
	Environment string
	Host        string
	Port        int
	Username    string
	Password    string
	TLS         TLSMode
	From        mail.Address
	ReplyTo     *mail.Address
	// PublicOrigin ist die HTTPS-Origin des Shops für Links in E-Mails.
	PublicOrigin string
	// Preview kennzeichnet E-Mails außerhalb der Produktion als Testbetrieb.
	Preview bool
}

var (
	ErrConfig = errors.New("ungültige E-Mail-Konfiguration")
	// captureHosts sind lokale Testpostfächer, die Nachrichten nur abfangen.
	captureHosts = map[string]bool{"mailpit": true, "mailhog": true, "localhost": true, "127.0.0.1": true, "::1": true}
	// reservedSuffixes dürfen in der Produktion nicht als Absenderdomain erscheinen.
	reservedSuffixes = []string{".local", ".localhost", ".invalid", ".test", ".example", "example.com", "example.org", "example.net"}
)

// IsCaptureHost meldet, ob ein Host ein lokales Testpostfach ist.
func IsCaptureHost(host string) bool { return captureHosts[strings.ToLower(strings.TrimSpace(host))] }

func configError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConfig, fmt.Sprintf(format, args...))
}

// ConfigFromEnv liest die Versandkonfiguration. environment ist APP_ENV, publicOrigin
// APP_PUBLIC_ORIGIN. Fehlerhafte Kombinationen verhindern den Start der API:
//   - In der Produktion sind Testpostfächer, unverschlüsseltes SMTP, fehlende Zugangsdaten
//     und reservierte Absenderdomains verboten.
//   - Außerhalb der Produktion ist nur ein lokales Testpostfach erlaubt, es sei denn,
//     MAIL_ALLOW_EXTERNAL_DELIVERY=true ist ausdrücklich gesetzt.
func ConfigFromEnv(getenv func(string) string, environment, publicOrigin string) (Config, error) {
	config := Config{Environment: environment, Preview: environment != "production"}
	origin, err := url.Parse(publicOrigin)
	if err != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Host == "" || origin.Path != "" {
		return Config{}, configError("APP_PUBLIC_ORIGIN muss eine Origin ohne Pfad sein")
	}
	if environment == "production" && origin.Scheme != "https" {
		return Config{}, configError("Links in E-Mails verlangen in der Produktion HTTPS")
	}
	config.PublicOrigin = origin.Scheme + "://" + origin.Host

	switch transport := strings.TrimSpace(getenv("MAIL_TRANSPORT")); transport {
	case "", "disabled":
		return config, nil
	case "smtp":
		config.Enabled = true
	default:
		return Config{}, configError("MAIL_TRANSPORT muss smtp oder disabled sein")
	}

	config.Host = strings.TrimSpace(getenv("SMTP_HOST"))
	if config.Host == "" || strings.ContainsAny(config.Host, " \r\n/") {
		return Config{}, configError("SMTP_HOST fehlt oder ist ungültig")
	}
	config.TLS = TLSMode(strings.TrimSpace(getenv("SMTP_TLS")))
	if config.TLS == "" {
		config.TLS = TLSStartTLS
	}
	defaultPort := map[TLSMode]int{TLSStartTLS: 587, TLSImplicit: 465, TLSNone: 1025}
	if _, ok := defaultPort[config.TLS]; !ok {
		return Config{}, configError("SMTP_TLS muss starttls, tls oder none sein")
	}
	config.Port = defaultPort[config.TLS]
	if raw := strings.TrimSpace(getenv("SMTP_PORT")); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, configError("SMTP_PORT ist ungültig")
		}
		config.Port = port
	}

	config.Username = strings.TrimSpace(getenv("SMTP_USERNAME"))
	password := getenv("SMTP_PASSWORD")
	if file := strings.TrimSpace(getenv("SMTP_PASSWORD_FILE")); file != "" {
		if password != "" {
			return Config{}, configError("SMTP_PASSWORD und SMTP_PASSWORD_FILE schließen sich aus")
		}
		raw, err := os.ReadFile(file)
		if err != nil {
			return Config{}, configError("SMTP_PASSWORD_FILE ist nicht lesbar")
		}
		password = strings.TrimRight(string(raw), "\r\n")
	}
	config.Password = password
	if (config.Username == "") != (config.Password == "") {
		return Config{}, configError("SMTP_USERNAME und SMTP-Passwort nur gemeinsam setzen")
	}

	from, err := parseAddress(getenv("MAIL_FROM"))
	if err != nil {
		return Config{}, configError("MAIL_FROM fehlt oder ist ungültig, z. B. NordWinds <no-reply@shop.example>")
	}
	config.From = *from
	if raw := strings.TrimSpace(getenv("MAIL_REPLY_TO")); raw != "" {
		replyTo, err := parseAddress(raw)
		if err != nil {
			return Config{}, configError("MAIL_REPLY_TO ist ungültig")
		}
		config.ReplyTo = replyTo
	}

	capture := IsCaptureHost(config.Host)
	if config.TLS == TLSNone && (!capture || environment == "production") {
		return Config{}, configError("unverschlüsseltes SMTP ist nur für lokale Testpostfächer außerhalb der Produktion erlaubt")
	}
	if environment == "production" {
		if capture || config.Port == 1025 {
			return Config{}, configError("in der Produktion ist kein Testpostfach (Mailpit, localhost, Port 1025) erlaubt")
		}
		if config.Username == "" {
			return Config{}, configError("in der Produktion sind SMTP-Zugangsdaten Pflicht")
		}
		domain := strings.ToLower(config.From.Address[strings.LastIndex(config.From.Address, "@")+1:])
		for _, suffix := range reservedSuffixes {
			if domain == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(domain, suffix) {
				return Config{}, configError("MAIL_FROM nutzt eine reservierte Domain")
			}
		}
	} else if !capture && getenv("MAIL_ALLOW_EXTERNAL_DELIVERY") != "true" {
		return Config{}, configError("außerhalb der Produktion nur an ein lokales Testpostfach senden oder MAIL_ALLOW_EXTERNAL_DELIVERY=true setzen")
	}
	return config, nil
}

// Address ist host:port für die Verbindung.
func (c Config) Address() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

func parseAddress(raw string) (*mail.Address, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("leer")
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || !strings.Contains(address.Address, "@") {
		return nil, errors.New("ungültig")
	}
	return address, nil
}
