// Package smtptest stellt einen kleinen SMTP-Server für Tests bereit, ähnlich httptest.
// Er speichert empfangene Nachrichten im Speicher und stellt nie etwas zu.
package smtptest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Options steuern das Verhalten des Testservers.
type Options struct {
	// TLS aktiviert STARTTLS bzw. implizites TLS (Implicit).
	TLS      *tls.Config
	Implicit bool
	// Username und Password verlangen AUTH PLAIN vor MAIL FROM.
	Username, Password string
	// RcptReply ersetzt die Antwort auf RCPT TO, z. B. "550 5.1.1 Mailbox unbekannt".
	RcptReply string
	// Silent nimmt Verbindungen an und antwortet nie (Zeitüberschreitung).
	Silent bool
}

// Received ist eine angenommene Nachricht.
type Received struct {
	From     string
	To       []string
	Data     string
	TLS      bool
	AuthUser string
}

type Server struct {
	Addr     string
	options  Options
	listener net.Listener
	mu       sync.Mutex
	messages []Received
}

// Start startet den Server auf 127.0.0.1 und beendet ihn mit dem Test.
func Start(t testing.TB, options Options) *Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if options.Implicit {
		listener = tls.NewListener(listener, options.TLS)
	}
	server := &Server{Addr: listener.Addr().String(), options: options, listener: listener}
	go server.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return server
}

// Messages liefert eine Kopie aller angenommenen Nachrichten.
func (s *Server) Messages() []Received {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Received(nil), s.messages...)
}

// Port liefert den Port als Zahl.
func (s *Server) Port() int {
	_, raw, _ := net.SplitHostPort(s.Addr)
	port, _ := strconv.Atoi(raw)
	return port
}

func (s *Server) serve() {
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handle(connection)
	}
}

func (s *Server) handle(connection net.Conn) {
	defer connection.Close()
	if s.options.Silent {
		time.Sleep(5 * time.Second)
		return
	}
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	text := textproto.NewConn(connection)
	reply := func(line string) { _ = text.PrintfLine("%s", line) }
	reply("220 smtptest ESMTP")
	secure := s.options.Implicit
	authed := ""
	current := Received{}
	for {
		line, err := text.ReadLine()
		if err != nil {
			return
		}
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch verb {
		case "EHLO", "HELO":
			lines := []string{"250-smtptest"}
			if s.options.TLS != nil && !secure {
				lines = append(lines, "250-STARTTLS")
			}
			if s.options.Username != "" {
				lines = append(lines, "250-AUTH PLAIN")
			}
			lines = append(lines, "250 8BITMIME")
			for _, l := range lines {
				reply(l)
			}
		case "STARTTLS":
			if s.options.TLS == nil || secure {
				reply("502 5.5.1 nicht verfügbar")
				continue
			}
			reply("220 2.0.0 bereit")
			tlsConnection := tls.Server(connection, s.options.TLS)
			if err := tlsConnection.Handshake(); err != nil {
				return
			}
			connection = tlsConnection
			text = textproto.NewConn(tlsConnection)
			reply = func(line string) { _ = text.PrintfLine("%s", line) }
			secure = true
		case "AUTH":
			fields := strings.Fields(line)
			if len(fields) != 3 || strings.ToUpper(fields[1]) != "PLAIN" {
				reply("504 5.5.4 nur PLAIN")
				continue
			}
			decoded, err := base64.StdEncoding.DecodeString(fields[2])
			parts := strings.Split(string(decoded), "\x00")
			if err != nil || len(parts) != 3 || parts[1] != s.options.Username || parts[2] != s.options.Password {
				reply("535 5.7.8 Anmeldung fehlgeschlagen")
				continue
			}
			authed = parts[1]
			reply("235 2.7.0 angemeldet")
		case "MAIL":
			if s.options.Username != "" && authed == "" {
				reply("530 5.7.0 Anmeldung erforderlich")
				continue
			}
			current = Received{From: address(line), TLS: secure, AuthUser: authed}
			reply("250 2.1.0 ok")
		case "RCPT":
			if s.options.RcptReply != "" {
				reply(s.options.RcptReply)
				continue
			}
			current.To = append(current.To, address(line))
			reply("250 2.1.5 ok")
		case "DATA":
			reply("354 Daten senden, Ende mit <CRLF>.<CRLF>")
			data, err := text.ReadDotBytes()
			if err != nil {
				return
			}
			current.Data = string(data)
			s.mu.Lock()
			s.messages = append(s.messages, current)
			s.mu.Unlock()
			reply("250 2.0.0 angenommen")
		case "RSET", "NOOP":
			reply("250 2.0.0 ok")
		case "QUIT":
			reply("221 2.0.0 tschüss")
			return
		default:
			reply("502 5.5.2 unbekannt")
		}
	}
}

func address(line string) string {
	start, end := strings.Index(line, "<"), strings.LastIndex(line, ">")
	if start < 0 || end < start {
		return ""
	}
	return line[start+1 : end]
}

// Certificate erzeugt ein selbstsigniertes Zertifikat für 127.0.0.1 und liefert die
// Server-Konfiguration sowie den Zertifikatspool für den Client.
func Certificate(t testing.TB) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "smtptest"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion: tls.VersionTLS12}, pool
}
