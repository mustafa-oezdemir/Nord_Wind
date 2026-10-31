package notification

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
	"time"
)

// Message ist eine fertig gerenderte E-Mail mit HTML- und Textfassung.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

var errHeader = errors.New("unzulässiger Kopfzeilenwert")

// Build erzeugt eine RFC-5322-Nachricht als multipart/alternative mit UTF-8 und
// Quoted-Printable. Zeilenumbrüche in Kopfzeilen werden abgewiesen (Header Injection).
func (m Message) Build(from mail.Address, replyTo *mail.Address, now time.Time) ([]byte, error) {
	to, err := mail.ParseAddress(m.To)
	if err != nil || to.Address != m.To || strings.ContainsAny(m.To, "\r\n") {
		return nil, errHeader
	}
	if strings.ContainsAny(m.Subject, "\r\n") || m.Subject == "" {
		return nil, errHeader
	}
	id, err := messageID(from.Address)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	parts := multipart.NewWriter(&body)
	for _, part := range []struct{ contentType, content string }{
		{"text/plain; charset=UTF-8", m.Text},
		{"text/html; charset=UTF-8", m.HTML},
	} {
		writer, err := parts.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		encoder := quotedprintable.NewWriter(writer)
		if _, err := encoder.Write([]byte(normalizeNewlines(part.content))); err != nil {
			return nil, err
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
	}
	if err := parts.Close(); err != nil {
		return nil, err
	}

	var message bytes.Buffer
	header := func(name, value string) { fmt.Fprintf(&message, "%s: %s\r\n", name, value) }
	header("From", from.String())
	header("To", (&mail.Address{Address: to.Address}).String())
	if replyTo != nil {
		header("Reply-To", replyTo.String())
	}
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", id)
	header("MIME-Version", "1.0")
	// RFC 3834: automatisch erzeugt, keine Abwesenheitsnotizen zurück.
	header("Auto-Submitted", "auto-generated")
	header("X-Auto-Response-Suppress", "All")
	header("Content-Type", `multipart/alternative; boundary="`+parts.Boundary()+`"`)
	message.WriteString("\r\n")
	message.Write(body.Bytes())
	return message.Bytes(), nil
}

func messageID(from string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	domain := from[strings.LastIndex(from, "@")+1:]
	return "<" + hex.EncodeToString(random) + "@" + domain + ">", nil
}

func normalizeNewlines(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\n", "\r\n")
}
