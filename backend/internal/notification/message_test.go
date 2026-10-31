package notification

import (
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestMessageBuild(t *testing.T) {
	from := mail.Address{Name: "NordWinds Küste", Address: "no-reply@nordwinds.de"}
	replyTo := &mail.Address{Address: "service@nordwinds.de"}
	message := Message{To: "erika@example.org", Subject: "Bestellbestätigung NW-2026-100001 – Größe Ü",
		Text: "Hallo Erika,\nÜbersicht folgt.\n", HTML: "<p>Hallo Erika, Übersicht</p>"}
	raw, err := message.Build(from, replyTo, time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	header := parsed.Header
	subject, err := new(mime.WordDecoder).DecodeHeader(header.Get("Subject"))
	if err != nil || subject != message.Subject {
		t.Fatalf("Betreff: %q, %v", subject, err)
	}
	if addresses, err := header.AddressList("From"); err != nil || addresses[0].Name != from.Name || addresses[0].Address != from.Address {
		t.Fatalf("Absender: %v, %v", addresses, err)
	}
	if header.Get("To") != "<erika@example.org>" || header.Get("Reply-To") != "<service@nordwinds.de>" ||
		header.Get("Auto-Submitted") != "auto-generated" || header.Get("MIME-Version") != "1.0" {
		t.Fatalf("Kopfzeilen: %v", header)
	}
	if !regexp.MustCompile(`^<[0-9a-f]{32}@nordwinds\.de>$`).MatchString(header.Get("Message-ID")) {
		t.Fatalf("Message-ID: %q", header.Get("Message-ID"))
	}
	if date, err := header.Date(); err != nil || !date.Equal(time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("Datum: %v, %v", date, err)
	}
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type: %v %v", mediaType, err)
	}
	reader := multipart.NewReader(parsed.Body, params["boundary"])
	want := []struct{ contentType, body string }{
		{"text/plain; charset=UTF-8", "Hallo Erika,\r\nÜbersicht folgt.\r\n"},
		{"text/html; charset=UTF-8", message.HTML},
	}
	for _, expected := range want {
		part, err := reader.NextRawPart()
		if err != nil {
			t.Fatal(err)
		}
		if part.Header.Get("Content-Type") != expected.contentType || part.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
			t.Fatalf("Teil: %v", part.Header)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(part))
		if err != nil || string(body) != expected.body {
			t.Fatalf("Inhalt: %q, %v", body, err)
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("zusätzlicher Teil: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatalf("Zeile länger als RFC 5322 erlaubt: %d", len(line))
		}
	}
}

func TestMessageRejectsHeaderInjection(t *testing.T) {
	from := mail.Address{Address: "no-reply@nordwinds.de"}
	for _, message := range []Message{
		{To: "erika@example.org\r\nBcc: alle@example.org", Subject: "Hallo", Text: "x", HTML: "x"},
		{To: "Erika <erika@example.org>", Subject: "Hallo", Text: "x", HTML: "x"},
		{To: "erika@example.org", Subject: "Hallo\r\nBcc: alle@example.org", Text: "x", HTML: "x"},
		{To: "erika@example.org", Subject: "", Text: "x", HTML: "x"},
		{To: "keine-adresse", Subject: "Hallo", Text: "x", HTML: "x"},
	} {
		if _, err := message.Build(from, nil, time.Now()); err == nil {
			t.Fatalf("Kopfzeilen-Injektion nicht abgewiesen: %+v", message)
		}
	}
}
