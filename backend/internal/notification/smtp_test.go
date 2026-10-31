package notification

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/notification/smtptest"
)

func transportFor(server *smtptest.Server, mode TLSMode, username, password string) *SMTPTransport {
	config := Config{Host: "127.0.0.1", Port: server.Port(), TLS: mode, Username: username, Password: password,
		From: mail.Address{Address: "no-reply@nordwinds.de"}}
	return NewSMTPTransport(config)
}

func sendCategory(t *testing.T, err error) Category {
	t.Helper()
	var failure *SendError
	if !errors.As(err, &failure) {
		t.Fatalf("SendError erwartet: %v", err)
	}
	return failure.Category
}

func TestSMTPTransportDeliversToLocalCapture(t *testing.T) {
	server := smtptest.Start(t, smtptest.Options{})
	err := transportFor(server, TLSNone, "", "").Send(context.Background(), "no-reply@nordwinds.de", "erika@example.org", []byte("Subject: Test\r\n\r\nHallo\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	messages := server.Messages()
	if len(messages) != 1 || messages[0].From != "no-reply@nordwinds.de" || messages[0].To[0] != "erika@example.org" ||
		!strings.Contains(messages[0].Data, "Hallo") || messages[0].TLS {
		t.Fatalf("Nachricht: %+v", messages)
	}
}

func TestSMTPTransportStartTLSWithAuth(t *testing.T) {
	serverTLS, pool := smtptest.Certificate(t)
	server := smtptest.Start(t, smtptest.Options{TLS: serverTLS, Username: "shop", Password: "geheim"})
	transport := transportFor(server, TLSStartTLS, "shop", "geheim")
	transport.TLSConfig = &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
	if err := transport.Send(context.Background(), "no-reply@nordwinds.de", "erika@example.org", []byte("Subject: x\r\n\r\ny\r\n")); err != nil {
		t.Fatal(err)
	}
	messages := server.Messages()
	if len(messages) != 1 || !messages[0].TLS || messages[0].AuthUser != "shop" {
		t.Fatalf("Zustellung ohne TLS oder Anmeldung: %+v", messages)
	}

	wrong := transportFor(server, TLSStartTLS, "shop", "falsch")
	wrong.TLSConfig = transport.TLSConfig
	if category := sendCategory(t, wrong.Send(context.Background(), "no-reply@nordwinds.de", "erika@example.org", []byte("x"))); category != CategoryAuth {
		t.Fatalf("falsches Passwort: %s", category)
	}

	untrusted := transportFor(server, TLSStartTLS, "shop", "geheim")
	if category := sendCategory(t, untrusted.Send(context.Background(), "no-reply@nordwinds.de", "erika@example.org", []byte("x"))); category != CategoryTLS {
		t.Fatalf("unbekannte Zertifizierungsstelle: %s", category)
	}
	if len(server.Messages()) != 1 {
		t.Fatal("Nachricht trotz Fehler angenommen")
	}
}

func TestSMTPTransportImplicitTLS(t *testing.T) {
	serverTLS, pool := smtptest.Certificate(t)
	server := smtptest.Start(t, smtptest.Options{TLS: serverTLS, Implicit: true})
	transport := transportFor(server, TLSImplicit, "", "")
	transport.TLSConfig = &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12}
	if err := transport.Send(context.Background(), "no-reply@nordwinds.de", "erika@example.org", []byte("x\r\n")); err != nil {
		t.Fatal(err)
	}
	if messages := server.Messages(); len(messages) != 1 || !messages[0].TLS {
		t.Fatalf("implizites TLS: %+v", messages)
	}
}

func TestSMTPTransportRefusesPlaintextFallback(t *testing.T) {
	server := smtptest.Start(t, smtptest.Options{Username: "shop", Password: "geheim"})
	if category := sendCategory(t, transportFor(server, TLSStartTLS, "shop", "geheim").Send(context.Background(),
		"no-reply@nordwinds.de", "erika@example.org", []byte("x"))); category != CategoryTLS {
		t.Fatalf("Server ohne STARTTLS: %s", category)
	}
	if len(server.Messages()) != 0 {
		t.Fatal("Klartextzustellung trotz STARTTLS-Pflicht")
	}
}

func TestSMTPTransportClassifiesFailures(t *testing.T) {
	for reply, want := range map[string]struct {
		category  Category
		permanent bool
	}{
		"550 5.1.1 Postfach unbekannt":          {CategoryRejectedPermanent, true},
		"451 4.3.0 Vorübergehend nicht möglich": {CategoryRejectedTemporary, false},
	} {
		server := smtptest.Start(t, smtptest.Options{RcptReply: reply})
		err := transportFor(server, TLSNone, "", "").Send(context.Background(), "no-reply@nordwinds.de", "erika@example.org", []byte("x"))
		var failure *SendError
		if !errors.As(err, &failure) || failure.Category != want.category || failure.Permanent != want.permanent {
			t.Fatalf("%s: %+v", reply, err)
		}
		if strings.Contains(failure.Error(), "erika@") {
			t.Fatal("Fehlertext enthält die Adresse")
		}
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	_ = listener.Close()
	closed := NewSMTPTransport(Config{Host: "127.0.0.1", Port: address.Port, TLS: TLSNone, From: mail.Address{Address: "a@nordwinds.de"}})
	if category := sendCategory(t, closed.Send(context.Background(), "a@nordwinds.de", "b@example.org", []byte("x"))); category != CategoryConnect {
		t.Fatalf("geschlossener Port: %s", category)
	}

	silent := smtptest.Start(t, smtptest.Options{Silent: true})
	slow := transportFor(silent, TLSNone, "", "")
	slow.timeout = 300 * time.Millisecond
	if category := sendCategory(t, slow.Send(context.Background(), "a@nordwinds.de", "b@example.org", []byte("x"))); category != CategoryTimeout {
		t.Fatalf("stummer Server: %s", category)
	}
}
