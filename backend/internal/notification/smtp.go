package notification

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"time"
)

// SMTP verschickt im lokalen Entwicklungsbetrieb die Registrierungsnachricht an MailHog.
type SMTP struct {
	Address string
	From    string
}

func (sender SMTP) SendWelcome(ctx context.Context, recipient string) error {
	dialer := net.Dialer{Timeout: 2 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", sender.Address)
	if err != nil {
		return err
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(sender.Address)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(connection, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Mail(sender.From); err != nil {
		return err
	}
	if err := client.Rcpt(recipient); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := fmt.Sprintf("From: NordWinds <%s>\r\nTo: <%s>\r\nSubject: Willkommen bei NordWinds\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nWillkommen bei NordWinds!\r\n\r\nIhr Kundenkonto wurde erstellt. Sie können sich jetzt anmelden und Ihren Warenkorb verwenden.\r\n\r\nNordWinds\r\n", sender.From, recipient)
	if _, err := io.WriteString(writer, message); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
