-- E-Mail-Benachrichtigungen über eine Transactional Outbox (ADR-011).
-- Trigger schreiben die Benachrichtigung in derselben Transaktion wie das fachliche Ereignis;
-- ein Hintergrundjob der API versendet sie. Ein Zustellfehler rollt das Ereignis nie zurück.
CREATE TABLE notification_outbox (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind TEXT NOT NULL CONSTRAINT notification_outbox_kind_valid CHECK (kind IN (
        'account.welcome', 'account.closed', 'security.login_locked',
        'order.created', 'order.paid', 'order.payment_failed', 'order.processing',
        'order.shipped', 'order.delivered', 'order.cancelled', 'order.refunded')),
    -- Ein fachliches Ereignis erzeugt höchstens eine Benachrichtigung.
    dedupe_key TEXT NOT NULL CONSTRAINT notification_outbox_dedupe_unique UNIQUE
        CONSTRAINT notification_outbox_dedupe_length CHECK (length(dedupe_key) BETWEEN 8 AND 200),
    -- Bewusst ohne Fremdschlüssel: Die Outbox ist ein Betriebsprotokoll und blockiert weder
    -- Kontoschließung noch TRUNCATE in Testdatenbanken.
    customer_id UUID,
    order_number TEXT,
    -- Nur für account.closed: Die Kontoadresse ist danach anonymisiert. Andere Arten lösen den
    -- Empfänger erst beim Versand aus dem aktiven Konto auf. Nach dem Abschluss wird sie gelöscht.
    recipient TEXT CONSTRAINT notification_outbox_recipient_length
        CHECK (recipient IS NULL OR length(recipient) BETWEEN 3 AND 254),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb CONSTRAINT notification_outbox_payload_object
        CHECK (jsonb_typeof(payload) = 'object'),
    state TEXT NOT NULL DEFAULT 'pending' CONSTRAINT notification_outbox_state_valid
        CHECK (state IN ('pending', 'sending', 'sent', 'failed', 'skipped')),
    attempts INTEGER NOT NULL DEFAULT 0 CONSTRAINT notification_outbox_attempts_range CHECK (attempts BETWEEN 0 AND 50),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_until TIMESTAMPTZ,
    -- Nur stabile Kategorien, nie SMTP-Antworttexte mit Adressen.
    last_error TEXT CONSTRAINT notification_outbox_error_valid CHECK (last_error IS NULL OR last_error IN (
        'connect', 'tls', 'auth', 'timeout', 'rejected_temporary', 'rejected_permanent',
        'render', 'recipient_unavailable', 'unknown')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ,
    CONSTRAINT notification_outbox_recipient_lifecycle CHECK (
        (state IN ('sent', 'failed', 'skipped') AND recipient IS NULL)
        OR (state IN ('pending', 'sending') AND (kind <> 'account.closed' OR recipient IS NOT NULL))),
    CONSTRAINT notification_outbox_order_reference CHECK ((kind LIKE 'order.%') = (order_number IS NOT NULL)),
    CONSTRAINT notification_outbox_sent_at CHECK ((state = 'sent') = (sent_at IS NOT NULL))
);
CREATE INDEX notification_outbox_due_idx ON notification_outbox (next_attempt_at, id)
    WHERE state IN ('pending', 'sending');
CREATE INDEX notification_outbox_state_idx ON notification_outbox (state, created_at DESC, id DESC);

-- Bestellstatus: jeder Eintrag im Verlauf ist genau ein Ereignis (ADR-008).
CREATE FUNCTION nordwinds_notify_order_status() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    notification_kind TEXT;
BEGIN
    notification_kind := CASE NEW.to_status
        WHEN 'CREATED' THEN 'order.created'
        WHEN 'PAID' THEN 'order.paid'
        WHEN 'PAYMENT_FAILED' THEN 'order.payment_failed'
        WHEN 'PROCESSING' THEN 'order.processing'
        WHEN 'SHIPPED' THEN 'order.shipped'
        WHEN 'DELIVERED' THEN 'order.delivered'
        WHEN 'CANCELLED' THEN 'order.cancelled'
    END;
    -- PAYMENT_PENDING löst der Kunde selbst aus; dafür gibt es keine E-Mail.
    IF notification_kind IS NULL THEN
        RETURN NULL;
    END IF;
    INSERT INTO notification_outbox (kind, dedupe_key, customer_id, order_number, payload)
    SELECT notification_kind, 'order-status-event:' || NEW.id, o.customer_id, o.order_number,
        jsonb_build_object('actor', NEW.actor)
    FROM orders o WHERE o.id = NEW.order_id
    ON CONFLICT (dedupe_key) DO NOTHING;
    RETURN NULL;
END $$;
CREATE TRIGGER order_status_events_notify AFTER INSERT ON order_status_events
    FOR EACH ROW EXECUTE FUNCTION nordwinds_notify_order_status();

-- Erstattung: Der Zahlungsstatus wechselt einmalig auf refunded (NW-18).
CREATE FUNCTION nordwinds_notify_refund() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO notification_outbox (kind, dedupe_key, customer_id, order_number, payload)
    SELECT 'order.refunded', 'payment-refund:' || NEW.id, o.customer_id, o.order_number,
        jsonb_build_object('amount_cents', NEW.amount_cents)
    FROM orders o WHERE o.id = NEW.order_id
    ON CONFLICT (dedupe_key) DO NOTHING;
    RETURN NULL;
END $$;
CREATE TRIGGER mock_payments_notify_refund AFTER UPDATE OF state ON mock_payments
    FOR EACH ROW WHEN (NEW.state = 'refunded' AND OLD.state IS DISTINCT FROM NEW.state)
    EXECUTE FUNCTION nordwinds_notify_refund();

-- Konto: Begrüßung bei der Registrierung, Bestätigung bei der Schließung (NW-20).
CREATE FUNCTION nordwinds_notify_customer() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.role = 'customer' THEN
            INSERT INTO notification_outbox (kind, dedupe_key, customer_id)
            VALUES ('account.welcome', 'customer-welcome:' || NEW.id, NEW.id)
            ON CONFLICT (dedupe_key) DO NOTHING;
        END IF;
    ELSIF OLD.anonymized_at IS NULL AND NEW.anonymized_at IS NOT NULL THEN
        INSERT INTO notification_outbox (kind, dedupe_key, customer_id, recipient)
        VALUES ('account.closed', 'customer-closed:' || NEW.id, NEW.id, OLD.email)
        ON CONFLICT (dedupe_key) DO NOTHING;
    END IF;
    RETURN NULL;
END $$;
CREATE TRIGGER customers_notify AFTER INSERT OR UPDATE OF anonymized_at ON customers
    FOR EACH ROW EXECUTE FUNCTION nordwinds_notify_customer();
