-- NW-18: Ausschließlich simulierte Zahlungen, keine Karten- oder Bankdaten.
CREATE TABLE mock_payments (
    id TEXT PRIMARY KEY CONSTRAINT mock_payments_id_format CHECK (id ~ '^pay_[0-9a-f]{32}$'),
    order_id UUID NOT NULL UNIQUE REFERENCES orders(id) ON DELETE RESTRICT,
    method TEXT NOT NULL CONSTRAINT mock_payments_method_valid
        CHECK (method IN ('debit_card', 'klarna', 'paypal', 'bank_transfer')),
    state TEXT NOT NULL DEFAULT 'pending' CONSTRAINT mock_payments_state_valid
        CHECK (state IN ('pending', 'succeeded', 'failed', 'refunded')),
    amount_cents BIGINT NOT NULL CONSTRAINT mock_payments_amount_nonnegative CHECK (amount_cents >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'EUR' CONSTRAINT mock_payments_currency_eur CHECK (currency = 'EUR'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE mock_payment_events (
    id TEXT PRIMARY KEY CONSTRAINT mock_payment_events_id_format CHECK (id ~ '^evt_[0-9a-f]{32}$'),
    payment_id TEXT NOT NULL REFERENCES mock_payments(id) ON DELETE RESTRICT,
    body_hash BYTEA NOT NULL CONSTRAINT mock_payment_events_hash_size CHECK (octet_length(body_hash) = 32),
    outcome TEXT NOT NULL CONSTRAINT mock_payment_events_outcome_valid
        CHECK (outcome IN ('succeeded', 'failed', 'refunded')),
    disposition TEXT NOT NULL CONSTRAINT mock_payment_events_disposition_valid
        CHECK (disposition IN ('applied', 'ignored', 'manual_review')),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX mock_payment_events_payment_idx ON mock_payment_events (payment_id, received_at);
