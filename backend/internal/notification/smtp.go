package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// Category ist eine stabile Fehlerkategorie; sie wird gespeichert und protokolliert,
// SMTP-Antworttexte (die Adressen enthalten können) dagegen nie.
type Category string

const (
	CategoryConnect           Category = "connect"
	CategoryTLS               Category = "tls"
	CategoryAuth              Category = "auth"
	CategoryTimeout           Category = "timeout"
	CategoryRejectedTemporary Category = "rejected_temporary"
	CategoryRejectedPermanent Category = "rejected_permanent"
	CategoryRender            Category = "render"
	CategoryRecipient         Category = "recipient_unavailable"
	CategoryUnknown           Category = "unknown"
)

// SendError beschreibt einen Zustellfehler. Permanent bedeutet: Wiederholen ist sinnlos
// (z. B. 550 Postfach unbekannt).
type SendError struct {
	Category  Category
	Permanent bool
	cause     error
}

func (e *SendError) Error() string { return "E-Mail-Versand fehlgeschlagen: " + string(e.Category) }
func (e *SendError) Unwrap() error { return e.cause }

// Transport übergibt eine fertige Nachricht an einen Mailserver.
type Transport interface {
	Send(ctx context.Context, from, to string, message []byte) error
}

// SMTPTransport spricht SMTP mit STARTTLS, implizitem TLS oder – nur für lokale
// Testpostfächer – unverschlüsselt. Zertifikate werden immer gegen den Hostnamen geprüft.
type SMTPTransport struct {
	config  Config
	timeout time.Duration
	// TLSConfig ersetzt die Standard-TLS-Konfiguration (nur für Tests mit eigener CA).
	TLSConfig *tls.Config
}

func NewSMTPTransport(config Config) *SMTPTransport {
	return &SMTPTransport{config: config, timeout: 20 * time.Second}
}

func (t *SMTPTransport) tlsConfig() *tls.Config {
	if t.TLSConfig != nil {
		return t.TLSConfig.Clone()
	}
	return &tls.Config{ServerName: t.config.Host, MinVersion: tls.VersionTLS12}
}

func (t *SMTPTransport) Send(ctx context.Context, from, to string, message []byte) error {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var connection net.Conn
	var err error
	if t.config.TLS == TLSImplicit {
		connection, err = (&tls.Dialer{NetDialer: dialer, Config: t.tlsConfig()}).DialContext(ctx, "tcp", t.config.Address())
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", t.config.Address())
	}
	if err != nil {
		return classify(err, CategoryConnect)
	}
	defer connection.Close()
	if err := connection.SetDeadline(deadline); err != nil {
		return classify(err, CategoryConnect)
	}
	client, err := smtp.NewClient(connection, t.config.Host)
	if err != nil {
		return classify(err, CategoryConnect)
	}
	defer client.Close()
	if err := client.Hello(heloName(t.config.From.Address)); err != nil {
		return classify(err, CategoryConnect)
	}
	if t.config.TLS == TLSStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			// Kein stiller Rückfall auf Klartext.
			return &SendError{Category: CategoryTLS}
		}
		if err := client.StartTLS(t.tlsConfig()); err != nil {
			return classify(err, CategoryTLS)
		}
	}
	if t.config.Username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return &SendError{Category: CategoryAuth}
		}
		// PlainAuth verweigert die Übertragung ohne TLS (außer zu localhost).
		auth := smtp.PlainAuth("", t.config.Username, t.config.Password, t.config.Host)
		if err := client.Auth(auth); err != nil {
			return classify(err, CategoryAuth)
		}
	}
	if err := client.Mail(from); err != nil {
		return classify(err, CategoryUnknown)
	}
	if err := client.Rcpt(to); err != nil {
		return classify(err, CategoryUnknown)
	}
	writer, err := client.Data()
	if err != nil {
		return classify(err, CategoryUnknown)
	}
	if _, err := writer.Write(message); err != nil {
		return classify(err, CategoryUnknown)
	}
	if err := writer.Close(); err != nil {
		return classify(err, CategoryUnknown)
	}
	if err := client.Quit(); err != nil {
		// Die Nachricht ist angenommen; ein Fehler beim Abmelden ändert daran nichts.
		return nil
	}
	return nil
}

// classify ordnet Fehler einer Kategorie zu. SMTP 5xx ist dauerhaft, 4xx vorübergehend.
func classify(err error, fallback Category) error {
	var protocol *textproto.Error
	if errors.As(err, &protocol) {
		switch {
		case fallback == CategoryAuth:
			return &SendError{Category: CategoryAuth, cause: err}
		case protocol.Code >= 500:
			return &SendError{Category: CategoryRejectedPermanent, Permanent: true, cause: err}
		case protocol.Code >= 400:
			return &SendError{Category: CategoryRejectedTemporary, cause: err}
		}
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return &SendError{Category: CategoryTimeout, cause: err}
	}
	var certificate *tls.CertificateVerificationError
	if errors.As(err, &certificate) {
		return &SendError{Category: CategoryTLS, cause: err}
	}
	return &SendError{Category: fallback, cause: err}
}

func heloName(from string) string {
	if domain := from[strings.LastIndex(from, "@")+1:]; domain != "" {
		return domain
	}
	return "localhost"
}
