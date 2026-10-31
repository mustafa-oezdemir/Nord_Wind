package notification

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/migrations"
	"github.com/mustafa-oezdemir/Nord_Wind/backend/internal/orders"
)

// fakeTransport nimmt Nachrichten an oder liefert einen vorgegebenen Fehler.
type fakeTransport struct {
	mu   sync.Mutex
	err  error
	sent []string
}

func (f *fakeTransport) Send(_ context.Context, _, to string, message []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, to+"\n"+string(message))
	return nil
}

type noOrders struct{}

func (noOrders) AdminGet(context.Context, string) (orders.Order, error) {
	return orders.Order{}, orders.ErrNotFound
}

// Prüft Outbox, Trigger, Reservierung, Wiederholung und Datenminimierung gegen PostgreSQL.
func TestOutboxOnPostgres(t *testing.T) {
	dsn := os.Getenv("NORDWINDS_NOTIFICATION_TEST_URL")
	if dsn == "" {
		t.Skip("NORDWINDS_NOTIFICATION_TEST_URL für eine separate PostgreSQL-Testdatenbank setzen")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "nordwinds_test_") {
		t.Fatalf("Separate Testdatenbank erforderlich: %q, %v", name, err)
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	reset := func() {
		t.Helper()
		if _, err := db.ExecContext(ctx, `TRUNCATE notification_outbox, auth_rate_limits, customers CASCADE`); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	t.Cleanup(func() {
		_, _ = db.Exec(`TRUNCATE notification_outbox, auth_rate_limits, customers CASCADE`)
		_ = db.Close()
	})
	store := NewStore(db)
	newCustomer := func(email string) string {
		t.Helper()
		var id string
		if err := db.QueryRowContext(ctx, `INSERT INTO customers (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// due ist der Abstand bis zum nächsten Versuch in Sekunden, gemessen an der Datenbankuhr.
	row := func(id int64) (state string, attempts int, lastError, recipient sql.NullString, due float64, sent sql.NullTime) {
		t.Helper()
		if err := db.QueryRowContext(ctx, `SELECT state, attempts, last_error, recipient,
				extract(epoch FROM next_attempt_at - now())::float8, sent_at
			FROM notification_outbox WHERE id = $1`, id).Scan(&state, &attempts, &lastError, &recipient, &due, &sent); err != nil {
			t.Fatal(err)
		}
		return
	}
	near := func(due float64, want time.Duration) bool { return due <= want.Seconds() && due > want.Seconds()-10 }
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	config := testConfig(true)
	dispatcher := func(transport Transport) *Dispatcher {
		d := NewDispatcher(store, noOrders{}, transport, config, slog.New(slog.NewJSONHandler(io.Discard, nil)))
		d.Now = func() time.Time { return now }
		return d
	}

	t.Run("Registrierung reiht genau eine Begrüßung ein", func(t *testing.T) {
		reset()
		id := newCustomer("erika@example.org")
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notification_outbox WHERE kind = 'account.welcome'
			AND customer_id = $1 AND state = 'pending' AND recipient IS NULL`, id).Scan(&count); err != nil || count != 1 {
			t.Fatalf("Begrüßung: %d, %v", count, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO customers (email, role) VALUES ('admin@example.org', 'admin')`); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notification_outbox`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("Admin-Konto erhielt eine Begrüßung: %d", count)
		}
		_, err := db.ExecContext(ctx, `INSERT INTO notification_outbox (kind, dedupe_key, customer_id)
			VALUES ('account.welcome', 'customer-welcome:' || $1::text, $1::uuid)`, id)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("Doppeltes Ereignis nicht abgewiesen: %v", err)
		}
		_, err = db.ExecContext(ctx, `INSERT INTO notification_outbox (kind, dedupe_key, customer_id) VALUES ('account.closed', 'x-closed-1234', $1)`, id)
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatalf("Schließungsbestätigung ohne Adresse angenommen: %v", err)
		}
	})

	t.Run("Zustellung löscht gespeicherte Adresse und protokolliert den Zeitpunkt", func(t *testing.T) {
		reset()
		id := newCustomer("erika@example.org")
		transport := &fakeTransport{}
		if processed, err := dispatcher(transport).Tick(ctx); err != nil || processed != 1 {
			t.Fatalf("Tick: %d, %v", processed, err)
		}
		if len(transport.sent) != 1 || !strings.HasPrefix(transport.sent[0], "erika@example.org\n") ||
			!strings.Contains(transport.sent[0], "Subject: Willkommen bei NordWinds") {
			t.Fatalf("Versand: %v", transport.sent)
		}
		var outboxID int64
		_ = db.QueryRowContext(ctx, `SELECT id FROM notification_outbox WHERE customer_id = $1`, id).Scan(&outboxID)
		state, attempts, _, recipient, _, sent := row(outboxID)
		if state != "sent" || attempts != 1 || recipient.Valid || !sent.Valid {
			t.Fatalf("Zustand nach Versand: %s %d %v %v", state, attempts, recipient, sent)
		}
		if processed, _ := dispatcher(transport).Tick(ctx); processed != 0 || len(transport.sent) != 1 {
			t.Fatal("Benachrichtigung doppelt versendet")
		}
	})

	t.Run("Vorübergehende Fehler werden mit Backoff wiederholt, dauerhafte nicht", func(t *testing.T) {
		reset()
		newCustomer("erika@example.org")
		var outboxID int64
		_ = db.QueryRowContext(ctx, `SELECT id FROM notification_outbox`).Scan(&outboxID)
		temporary := &fakeTransport{err: &SendError{Category: CategoryConnect}}
		if _, err := dispatcher(temporary).Tick(ctx); err != nil {
			t.Fatal(err)
		}
		state, attempts, lastError, _, due, _ := row(outboxID)
		if state != "pending" || attempts != 1 || lastError.String != "connect" || !near(due, time.Minute) {
			t.Fatalf("erster Fehler: %s %d %s %v", state, attempts, lastError.String, due)
		}
		if processed, _ := dispatcher(temporary).Tick(ctx); processed != 0 {
			t.Fatal("Eintrag vor Ablauf des Backoffs erneut geholt")
		}
		for attempt := 2; attempt <= MaxAttempts; attempt++ {
			if _, err := db.ExecContext(ctx, `UPDATE notification_outbox SET next_attempt_at = now() - interval '1 second'`); err != nil {
				t.Fatal(err)
			}
			if _, err := dispatcher(temporary).Tick(ctx); err != nil {
				t.Fatal(err)
			}
			state, attempts, _, _, due, _ = row(outboxID)
			if attempt < MaxAttempts && (state != "pending" || !near(due, Backoff[attempt-1])) {
				t.Fatalf("Versuch %d: %s %v", attempt, state, due)
			}
		}
		if state != "failed" || attempts != MaxAttempts {
			t.Fatalf("nach %d Versuchen: %s %d", MaxAttempts, state, attempts)
		}

		if err := store.Retry(ctx, outboxID); err != nil {
			t.Fatal(err)
		}
		permanent := &fakeTransport{err: &SendError{Category: CategoryRejectedPermanent, Permanent: true}}
		if _, err := dispatcher(permanent).Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if state, attempts, lastError, _, _, _ := row(outboxID); state != "failed" || attempts != 1 || lastError.String != "rejected_permanent" {
			t.Fatalf("dauerhafter Fehler: %s %d %s", state, attempts, lastError.String)
		}
		if err := store.Retry(ctx, 999999); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unbekannte ID: %v", err)
		}
	})

	t.Run("Abgelaufene Reservierung wird erneut geholt; parallele Abholer teilen sich die Arbeit", func(t *testing.T) {
		reset()
		for index := 0; index < 30; index++ {
			newCustomer("kunde" + string(rune('a'+index%26)) + string(rune('a'+index/26)) + "@example.org")
		}
		var wait sync.WaitGroup
		results := make([][]Notification, 2)
		for worker := range results {
			wait.Add(1)
			go func(worker int) {
				defer wait.Done()
				claimed, err := store.Claim(ctx, 20, time.Minute)
				if err != nil {
					t.Error(err)
				}
				results[worker] = claimed
			}(worker)
		}
		wait.Wait()
		seen := map[int64]bool{}
		for _, claimed := range results {
			for _, n := range claimed {
				if seen[n.ID] {
					t.Fatalf("Eintrag %d doppelt geholt", n.ID)
				}
				seen[n.ID] = true
			}
		}
		if len(seen) != 30 {
			t.Fatalf("%d statt 30 Einträge geholt", len(seen))
		}
		if again, _ := store.Claim(ctx, 50, time.Minute); len(again) != 0 {
			t.Fatal("reservierte Einträge erneut geholt")
		}
		if _, err := db.ExecContext(ctx, `UPDATE notification_outbox SET locked_until = now() - interval '1 second'`); err != nil {
			t.Fatal(err)
		}
		recovered, err := store.Claim(ctx, 50, time.Minute)
		if err != nil || len(recovered) != 30 || recovered[0].Attempts != 2 {
			t.Fatalf("nach Absturz: %d, %v", len(recovered), err)
		}
		// Ereignisse gehen in der Reihenfolge ihres Entstehens hinaus.
		for index := 1; index < len(recovered); index++ {
			if recovered[index-1].ID >= recovered[index].ID {
				t.Fatalf("Reihenfolge verletzt an Position %d", index)
			}
		}
	})

	t.Run("Geschlossene Konten: Bestätigung an die alte Adresse, sonst nichts", func(t *testing.T) {
		reset()
		id := newCustomer("erika@example.org")
		if _, err := db.ExecContext(ctx, `UPDATE customers SET email = 'closed-' || id::text || '@example.invalid',
			password_hash = NULL, anonymized_at = now() WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		transport := &fakeTransport{}
		if _, err := dispatcher(transport).Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if len(transport.sent) != 1 || !strings.HasPrefix(transport.sent[0], "erika@example.org\n") ||
			!strings.Contains(transport.sent[0], "geschlossen") {
			t.Fatalf("Schließungsbestätigung: %v", transport.sent)
		}
		var welcome, closed string
		var stored sql.NullString
		_ = db.QueryRowContext(ctx, `SELECT state FROM notification_outbox WHERE kind = 'account.welcome'`).Scan(&welcome)
		_ = db.QueryRowContext(ctx, `SELECT state, recipient FROM notification_outbox WHERE kind = 'account.closed'`).Scan(&closed, &stored)
		if welcome != "skipped" || closed != "sent" || stored.Valid {
			t.Fatalf("Zustände: Begrüßung %s, Schließung %s, Adresse gespeichert %v", welcome, closed, stored.Valid)
		}
		var closedID int64
		_ = db.QueryRowContext(ctx, `SELECT id FROM notification_outbox WHERE kind = 'account.closed'`).Scan(&closedID)
		if _, err := db.ExecContext(ctx, `UPDATE notification_outbox SET state = 'failed', sent_at = NULL WHERE id = $1`, closedID); err != nil {
			t.Fatal(err)
		}
		if err := store.Retry(ctx, closedID); !errors.Is(err, ErrNotRetryable) {
			t.Fatalf("Schließung ohne Adresse erneut geplant: %v", err)
		}
	})

	t.Run("Gesperrte Anmeldung: ein Hinweis je Fenster, nichts für unbekannte Adressen", func(t *testing.T) {
		reset()
		newCustomer("erika@example.org")
		key := sha256.Sum256([]byte("account:erika@example.org"))
		unknown := sha256.Sum256([]byte("account:niemand@example.org"))
		if _, err := db.ExecContext(ctx, `INSERT INTO auth_rate_limits (key_hash, window_started_at, attempts)
			VALUES ($1, now(), 6), ($2, now(), 6)`, key[:], unknown[:]); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := store.EnqueueLoginLocked(ctx, "erika@example.org", key[:]); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.EnqueueLoginLocked(ctx, "niemand@example.org", unknown[:]); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notification_outbox WHERE kind = 'security.login_locked'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("Sicherheitshinweise: %d, %v", count, err)
		}
		if _, err := db.ExecContext(ctx, `UPDATE auth_rate_limits SET window_started_at = now() + interval '20 minutes' WHERE key_hash = $1`, key[:]); err != nil {
			t.Fatal(err)
		}
		if err := store.EnqueueLoginLocked(ctx, "erika@example.org", key[:]); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM notification_outbox WHERE kind = 'security.login_locked'`).Scan(&count); err != nil || count != 2 {
			t.Fatalf("neues Sperrfenster: %d, %v", count, err)
		}
	})

	t.Run("Verwaltungssicht ohne Adressen", func(t *testing.T) {
		reset()
		newCustomer("erika@example.org")
		entries, err := store.List(ctx, "pending", 10)
		if err != nil || len(entries) != 1 || entries[0].Kind != KindWelcome {
			t.Fatalf("Liste: %+v, %v", entries, err)
		}
		counts, err := store.Counts(ctx)
		if err != nil || counts["pending"] != 1 || counts["failed"] != 0 {
			t.Fatalf("Zählung: %v, %v", counts, err)
		}
		if MaskEmail("erika@example.org") != "e***@example.org" || MaskEmail("kaputt") != "***" {
			t.Fatal("Maskierung")
		}
	})
}
