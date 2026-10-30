-- NW-16: Transaktionale Bestellanlage mit Idempotenz, Preis- und Adress-Snapshot,
-- unveränderlichen Positionen und Statusverlauf (ADR-008).
LOCK TABLE orders, order_items IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    -- Vor NW-16 gab es keinen Checkout; vorhandene Zeilen hätten keine Preisaufstellung.
    IF EXISTS (SELECT 1 FROM orders) OR EXISTS (SELECT 1 FROM order_items) THEN
        RAISE EXCEPTION 'orders enthält Daten ohne Preisaufstellung; Migration 9 erwartet leere Bestelltabellen';
    END IF;
END $$;

CREATE SEQUENCE order_number_seq AS BIGINT START WITH 100001 MINVALUE 100001 NO CYCLE;

ALTER TABLE orders
    ADD COLUMN idempotency_key TEXT NOT NULL
        CONSTRAINT orders_idempotency_key_format CHECK (idempotency_key ~ '^[A-Za-z0-9_-]{16,64}$'),
    ADD COLUMN request_hash BYTEA NOT NULL
        CONSTRAINT orders_request_hash_length CHECK (octet_length(request_hash) = 32),
    ADD COLUMN customer_email TEXT NOT NULL
        CONSTRAINT orders_customer_email_nonempty CHECK (btrim(customer_email) <> ''),
    ADD COLUMN goods_cents BIGINT NOT NULL,
    ADD COLUMN discount_cents BIGINT NOT NULL,
    ADD COLUMN shipping_cents BIGINT NOT NULL,
    ADD COLUMN discount_code TEXT,
    ADD COLUMN discount_percent_bp INTEGER,
    ADD COLUMN tax_lines JSONB NOT NULL,
    ADD CONSTRAINT orders_amounts_valid CHECK (goods_cents >= 0 AND shipping_cents >= 0
        AND discount_cents >= 0 AND discount_cents <= goods_cents),
    ADD CONSTRAINT orders_total_matches CHECK (total_cents = goods_cents - discount_cents + shipping_cents),
    ADD CONSTRAINT orders_discount_pair CHECK ((discount_code IS NULL) = (discount_percent_bp IS NULL)),
    ADD CONSTRAINT orders_discount_percent_range CHECK (discount_percent_bp IS NULL OR discount_percent_bp BETWEEN 1 AND 10000),
    ADD CONSTRAINT orders_tax_lines_valid CHECK (jsonb_typeof(tax_lines) = 'array' AND jsonb_array_length(tax_lines) > 0),
    ADD CONSTRAINT orders_shipping_address CHECK (jsonb_typeof(shipping_snapshot -> 'address') = 'object');
CREATE UNIQUE INDEX orders_customer_idempotency_unique ON orders (customer_id, idempotency_key);

ALTER TABLE order_items
    ADD COLUMN position SMALLINT NOT NULL CONSTRAINT order_items_position_range CHECK (position BETWEEN 1 AND 50),
    ADD COLUMN variant_label_snapshot TEXT NOT NULL
        CONSTRAINT order_items_label_nonempty CHECK (btrim(variant_label_snapshot) <> ''),
    ADD COLUMN line_total_cents BIGINT NOT NULL,
    ADD CONSTRAINT order_items_line_total_matches CHECK (line_total_cents = unit_price_cents * quantity),
    ADD CONSTRAINT order_items_tax_fields CHECK (tax_snapshot ? 'tax_class' AND tax_snapshot ? 'rate_bp'),
    ADD CONSTRAINT order_items_position_unique UNIQUE (order_id, position);

CREATE TABLE order_status_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    from_status TEXT,
    to_status TEXT NOT NULL,
    actor TEXT NOT NULL CONSTRAINT order_status_events_actor_valid CHECK (actor IN ('customer', 'admin', 'system')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT order_status_events_status_valid CHECK (
        (from_status IS NULL OR from_status IN ('CREATED', 'PAYMENT_PENDING', 'PAID', 'PROCESSING', 'SHIPPED', 'DELIVERED', 'CANCELLED', 'PAYMENT_FAILED'))
        AND to_status IN ('CREATED', 'PAYMENT_PENDING', 'PAID', 'PROCESSING', 'SHIPPED', 'DELIVERED', 'CANCELLED', 'PAYMENT_FAILED'))
);
CREATE INDEX order_status_events_order_idx ON order_status_events (order_id, id);

-- Positionen und Statusverlauf sind nach dem Kauf unveränderlich.
CREATE FUNCTION nordwinds_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% ist unveränderlich', TG_TABLE_NAME USING ERRCODE = 'integrity_constraint_violation';
END $$;
CREATE TRIGGER order_items_append_only BEFORE UPDATE OR DELETE ON order_items
    FOR EACH ROW EXECUTE FUNCTION nordwinds_append_only();
CREATE TRIGGER order_status_events_append_only BEFORE UPDATE OR DELETE ON order_status_events
    FOR EACH ROW EXECUTE FUNCTION nordwinds_append_only();

-- Bestellungen: nur Status und updated_at dürfen sich ändern, und nur entlang ADR-008.
CREATE FUNCTION nordwinds_guard_order() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'Bestellungen werden nicht gelöscht' USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF to_jsonb(NEW) - 'status' - 'updated_at' IS DISTINCT FROM to_jsonb(OLD) - 'status' - 'updated_at' THEN
        RAISE EXCEPTION 'Bestell-Snapshot ist unveränderlich' USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND (OLD.status, NEW.status) NOT IN (
        ('CREATED', 'PAYMENT_PENDING'), ('CREATED', 'CANCELLED'),
        ('PAYMENT_PENDING', 'PAID'), ('PAYMENT_PENDING', 'PAYMENT_FAILED'), ('PAYMENT_PENDING', 'CANCELLED'),
        ('PAYMENT_FAILED', 'PAYMENT_PENDING'), ('PAYMENT_FAILED', 'CANCELLED'),
        ('PAID', 'PROCESSING'), ('PROCESSING', 'SHIPPED'), ('SHIPPED', 'DELIVERED')
    ) THEN
        RAISE EXCEPTION 'Statuswechsel % -> % ist nicht erlaubt', OLD.status, NEW.status
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER orders_guard BEFORE UPDATE OR DELETE ON orders
    FOR EACH ROW EXECUTE FUNCTION nordwinds_guard_order();
