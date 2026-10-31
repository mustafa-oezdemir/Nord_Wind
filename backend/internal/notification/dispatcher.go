package notification

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

// OrderSource lädt den unveränderlichen Bestell-Snapshot (ADR-008).
type OrderSource interface {
	AdminGet(ctx context.Context, number string) (orders.Order, error)
}

// Backoff ist der Abstand nach dem n-ten fehlgeschlagenen Versuch. Nach MaxAttempts
// Versuchen gilt eine Benachrichtigung als fehlgeschlagen und kann im Adminbereich
// erneut angestoßen werden.
var Backoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour}

const MaxAttempts = 6

// Dispatcher versendet fällige Benachrichtigungen.
type Dispatcher struct {
	Store     *Store
	Orders    OrderSource
	Transport Transport
	Config    Config
	Logger    *slog.Logger
	BatchSize int
	Lease     time.Duration
	Now       func() time.Time
}

func NewDispatcher(store *Store, orderSource OrderSource, transport Transport, config Config, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{Store: store, Orders: orderSource, Transport: transport, Config: config, Logger: logger,
		BatchSize: 20, Lease: 2 * time.Minute, Now: time.Now}
}

// Run arbeitet die Outbox in festen Abständen ab, bis ctx endet.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := d.Tick(ctx); err != nil && ctx.Err() == nil {
			d.Logger.Error("Benachrichtigungen konnten nicht abgearbeitet werden", "event", "notification.tick.failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick verarbeitet einen Stapel und liefert die Zahl der versuchten Einträge.
func (d *Dispatcher) Tick(ctx context.Context) (int, error) {
	batch, err := d.Store.Claim(ctx, d.BatchSize, d.Lease)
	if err != nil {
		return 0, err
	}
	for _, n := range batch {
		if err := d.deliver(ctx, n); err != nil {
			return len(batch), err
		}
	}
	return len(batch), nil
}

func (d *Dispatcher) deliver(ctx context.Context, n Notification) error {
	log := d.Logger.With("event", "notification.delivery", "notification_id", n.ID, "kind", string(n.Kind), "attempt", n.Attempts)
	recipient, err := d.Store.ResolveRecipient(ctx, n)
	if errors.Is(err, ErrNotFound) {
		log.Info("Empfänger nicht mehr aktiv; Benachrichtigung übersprungen", "outcome", "skipped")
		return d.Store.MarkSkipped(ctx, n.ID, CategoryRecipient)
	}
	if err != nil {
		return err
	}
	message, err := d.render(ctx, n, recipient)
	if errors.Is(err, errRender) {
		log.Error("Benachrichtigung konnte nicht erstellt werden", "outcome", "failed", "category", string(CategoryRender))
		return d.Store.MarkFailed(ctx, n.ID, CategoryRender)
	}
	if err != nil {
		// Vorübergehender Datenbankfehler: Die Reservierung läuft ab, dann folgt ein neuer Versuch.
		return err
	}
	raw, err := message.Build(d.Config.From, d.Config.ReplyTo, d.Now())
	if err != nil {
		log.Error("Benachrichtigung konnte nicht erstellt werden", "outcome", "failed", "category", string(CategoryRender))
		return d.Store.MarkFailed(ctx, n.ID, CategoryRender)
	}
	sendErr := d.Transport.Send(ctx, d.Config.From.Address, recipient, raw)
	if sendErr == nil {
		log.Info("Benachrichtigung zugestellt", "outcome", "sent", "recipient", MaskEmail(recipient))
		return d.Store.MarkSent(ctx, n.ID)
	}
	var failure *SendError
	if !errors.As(sendErr, &failure) {
		failure = &SendError{Category: CategoryUnknown}
	}
	if failure.Permanent || n.Attempts >= MaxAttempts {
		log.Error("Benachrichtigung endgültig fehlgeschlagen", "outcome", "failed", "category", string(failure.Category))
		return d.Store.MarkFailed(ctx, n.ID, failure.Category)
	}
	delay := Backoff[min(n.Attempts, len(Backoff))-1]
	log.Warn("Benachrichtigung wird später erneut versucht", "outcome", "retry", "category", string(failure.Category), "retry_in", delay.String())
	return d.Store.MarkRetry(ctx, n.ID, failure.Category, delay)
}

// errRender kennzeichnet dauerhafte Fehler beim Erstellen einer Nachricht.
var errRender = errors.New("Nachricht nicht darstellbar")

func (d *Dispatcher) render(ctx context.Context, n Notification, recipient string) (Message, error) {
	var order *orders.Order
	if n.OrderNumber != "" {
		loaded, err := d.Orders.AdminGet(ctx, n.OrderNumber)
		if errors.Is(err, orders.ErrNotFound) {
			return Message{}, errRender
		}
		if err != nil {
			return Message{}, err
		}
		order = &loaded
	}
	view, err := Compose(d.Config, n, order, d.Now())
	if err != nil {
		return Message{}, errRender
	}
	subject, text, html, err := Render(view)
	if err != nil {
		return Message{}, errRender
	}
	return Message{To: recipient, Subject: subject, Text: text, HTML: html}, nil
}
