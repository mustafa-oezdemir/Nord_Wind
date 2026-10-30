-- NW-17: Der abgebuchte Bestand wird bis zur Zahlung einer Bestellung zugeordnet.
CREATE TABLE order_reservations (
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE RESTRICT,
    quantity INTEGER NOT NULL CONSTRAINT order_reservations_quantity_positive CHECK (quantity > 0),
    state TEXT NOT NULL DEFAULT 'active' CONSTRAINT order_reservations_state_valid
        CHECK (state IN ('active', 'consumed', 'released')),
    reserved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    PRIMARY KEY (order_id, variant_id),
    CONSTRAINT order_reservations_expiry_valid CHECK (expires_at > reserved_at),
    CONSTRAINT order_reservations_settlement_valid CHECK (
        (state = 'active' AND settled_at IS NULL) OR
        (state <> 'active' AND settled_at IS NOT NULL)
    )
);

CREATE INDEX order_reservations_expiry_idx ON order_reservations (expires_at, order_id)
    WHERE state = 'active';
