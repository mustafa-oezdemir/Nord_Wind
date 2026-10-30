-- NW-19: Manueller Versand und messbare Versandversuche.
CREATE TABLE order_shipments (
    order_id UUID PRIMARY KEY REFERENCES orders(id) ON DELETE RESTRICT,
    carrier TEXT NOT NULL CONSTRAINT order_shipments_carrier_valid CHECK (carrier IN ('MANUAL')),
    tracking_number TEXT NOT NULL CONSTRAINT order_shipments_tracking_valid
        CHECK (tracking_number ~ '^[A-Za-z0-9][A-Za-z0-9 -]{4,63}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE shipment_attempts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id UUID REFERENCES orders(id) ON DELETE RESTRICT,
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'invalid_state', 'invalid_tracking', 'shipment_failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX shipment_attempts_created_idx ON shipment_attempts (created_at DESC);
