-- NW-20: Kontogebundene Adressen und Schließungsmarkierung.
CREATE TABLE customer_addresses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    address JSONB NOT NULL CONSTRAINT customer_addresses_object CHECK (jsonb_typeof(address) = 'object'),
    label TEXT NOT NULL CONSTRAINT customer_addresses_label_valid CHECK (length(btrim(label)) BETWEEN 1 AND 60),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX customer_addresses_owner_idx ON customer_addresses (customer_id, created_at, id);

ALTER TABLE customers ADD COLUMN anonymized_at TIMESTAMPTZ;
