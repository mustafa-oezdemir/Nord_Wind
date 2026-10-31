package notification

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Store kapselt notification_outbox.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

var ErrNotFound = errors.New("Benachrichtigung nicht gefunden")

// Claim reserviert bis zu limit fällige Einträge für lease. Mehrere API-Instanzen holen
// dank SKIP LOCKED nie denselben Eintrag; nach einem Absturz wird ein Eintrag mit
// abgelaufener Reservierung erneut geholt.
func (s *Store) Claim(ctx context.Context, limit int, lease time.Duration) ([]Notification, error) {
	// RETURNING liefert keine feste Reihenfolge; sortiert nach id gehen Ereignisse derselben
	// Bestellung in ihrer zeitlichen Abfolge hinaus (erst „unterwegs“, dann „zugestellt“).
	rows, err := s.db.QueryContext(ctx, `WITH claimed AS (
			UPDATE notification_outbox SET state = 'sending',
				locked_until = now() + make_interval(secs => $2), attempts = attempts + 1, updated_at = now()
			WHERE id IN (
				SELECT id FROM notification_outbox
				WHERE (state = 'pending' AND next_attempt_at <= now()) OR (state = 'sending' AND locked_until < now())
				ORDER BY next_attempt_at, id LIMIT $1 FOR UPDATE SKIP LOCKED)
			RETURNING id, kind, COALESCE(customer_id::text, '') AS customer_id, COALESCE(order_number, '') AS order_number,
				COALESCE(recipient, '') AS recipient, payload, attempts, created_at)
		SELECT id, kind, customer_id, order_number, recipient, payload, attempts, created_at FROM claimed ORDER BY id`,
		limit, lease.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Notification{}
	for rows.Next() {
		var n Notification
		var payload []byte
		if err := rows.Scan(&n.ID, &n.Kind, &n.CustomerID, &n.OrderNumber, &n.Recipient, &payload, &n.Attempts, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Payload = payload
		result = append(result, n)
	}
	return result, rows.Err()
}

// ResolveRecipient liefert die aktuelle Adresse eines aktiven Kontos. Geschlossene Konten
// bekommen keine Post mehr, außer der Bestätigung der Schließung selbst.
func (s *Store) ResolveRecipient(ctx context.Context, n Notification) (string, error) {
	if n.Kind == KindAccountClosed {
		if n.Recipient == "" {
			return "", ErrNotFound
		}
		return n.Recipient, nil
	}
	var email string
	err := s.db.QueryRowContext(ctx, `SELECT email FROM customers WHERE id = $1 AND anonymized_at IS NULL`,
		n.CustomerID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return email, err
}

func (s *Store) MarkSent(ctx context.Context, id int64) error {
	return s.finish(ctx, id, "sent", "")
}

// MarkSkipped schließt einen Eintrag ohne Versand ab, z. B. bei geschlossenem Konto.
func (s *Store) MarkSkipped(ctx context.Context, id int64, category Category) error {
	return s.finish(ctx, id, "skipped", category)
}

func (s *Store) MarkFailed(ctx context.Context, id int64, category Category) error {
	return s.finish(ctx, id, "failed", category)
}

// finish setzt einen Endzustand und löscht eine gespeicherte Empfängeradresse.
func (s *Store) finish(ctx context.Context, id int64, state string, category Category) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET state = $2, recipient = NULL,
			last_error = NULLIF($3, ''), locked_until = NULL, updated_at = now(),
			sent_at = CASE WHEN $2 = 'sent' THEN now() END
		WHERE id = $1 AND state = 'sending'`, id, state, string(category))
	return err
}

// MarkRetry plant einen neuen Versuch nach delay. Die Zeit kommt wie bei Claim aus der
// Datenbank, damit abweichende Uhren der API-Instanzen keine Rolle spielen.
func (s *Store) MarkRetry(ctx context.Context, id int64, category Category, delay time.Duration) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET state = 'pending', last_error = $2,
			next_attempt_at = now() + make_interval(secs => $3), locked_until = NULL, updated_at = now()
		WHERE id = $1 AND state = 'sending'`, id, string(category), delay.Seconds())
	return err
}

// EnqueueLoginLocked schreibt höchstens einen Sicherheitshinweis je Sperrfenster.
// Für unbekannte oder geschlossene Konten entsteht nichts; die API-Antwort ist ohnehin
// dieselbe, sodass niemand erfährt, ob eine Adresse registriert ist.
func (s *Store) EnqueueLoginLocked(ctx context.Context, email string, rateLimitKey []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO notification_outbox (kind, dedupe_key, customer_id)
		SELECT 'security.login_locked',
			'login-locked:' || c.id || ':' || floor(extract(epoch FROM r.window_started_at))::bigint, c.id
		FROM customers c JOIN auth_rate_limits r ON r.key_hash = $2
		WHERE c.email = $1 AND c.anonymized_at IS NULL AND c.role = 'customer'
		ON CONFLICT (dedupe_key) DO NOTHING`, email, rateLimitKey)
	return err
}

// AdminEntry ist die Verwaltungssicht ohne Empfängeradresse und ohne Inhalt.
type AdminEntry struct {
	ID            int64      `json:"id"`
	Kind          Kind       `json:"kind"`
	State         string     `json:"state"`
	OrderNumber   *string    `json:"order_number"`
	Attempts      int        `json:"attempts"`
	LastError     *string    `json:"last_error"`
	NextAttemptAt time.Time  `json:"next_attempt_at"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at"`
}

var validStates = map[string]bool{"pending": true, "sending": true, "sent": true, "failed": true, "skipped": true}

// ValidState meldet, ob ein Filterwert ein Outbox-Zustand ist.
func ValidState(state string) bool { return validStates[state] }

func (s *Store) List(ctx context.Context, state string, limit int) ([]AdminEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, state, order_number, attempts, last_error,
			next_attempt_at, created_at, sent_at
		FROM notification_outbox WHERE ($1 = '' OR state = $1)
		ORDER BY created_at DESC, id DESC LIMIT $2`, state, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AdminEntry{}
	for rows.Next() {
		var entry AdminEntry
		if err := rows.Scan(&entry.ID, &entry.Kind, &entry.State, &entry.OrderNumber, &entry.Attempts,
			&entry.LastError, &entry.NextAttemptAt, &entry.CreatedAt, &entry.SentAt); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// Counts liefert die Anzahl je Zustand für Betrieb und Monitoring.
func (s *Store) Counts(ctx context.Context) (map[string]int64, error) {
	result := map[string]int64{"pending": 0, "sending": 0, "sent": 0, "failed": 0, "skipped": 0}
	rows, err := s.db.QueryContext(ctx, `SELECT state, count(*) FROM notification_outbox GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count int64
		if err := rows.Scan(&state, &count); err != nil {
			return nil, err
		}
		result[state] = count
	}
	return result, rows.Err()
}

var ErrNotRetryable = errors.New("Benachrichtigung kann nicht erneut gesendet werden")

// Retry setzt eine fehlgeschlagene Benachrichtigung erneut auf fällig. Die Bestätigung
// einer Kontoschließung ist ausgenommen, weil ihre Adresse bereits gelöscht ist.
func (s *Store) Retry(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE notification_outbox SET state = 'pending', attempts = 0,
			next_attempt_at = now(), last_error = NULL, updated_at = now()
		WHERE id = $1 AND state = 'failed' AND kind <> 'account.closed'`, id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected == 1 {
		return nil
	}
	var state, kind string
	err = s.db.QueryRowContext(ctx, `SELECT state, kind FROM notification_outbox WHERE id = $1`, id).Scan(&state, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrNotRetryable
}

// MaskEmail kürzt eine Adresse für Protokolle, z. B. „e***@example.org“.
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 1 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}
