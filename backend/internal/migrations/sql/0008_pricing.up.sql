-- NW-15: Steuerklasse je Produkt, prozentuale Rabattcodes und Code je Warenkorb (ADR-007).
ALTER TABLE products ADD COLUMN tax_class TEXT NOT NULL DEFAULT 'standard'
    CONSTRAINT products_tax_class_valid CHECK (tax_class IN ('standard', 'reduced'));

CREATE TABLE discount_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL CONSTRAINT discount_codes_code_format CHECK (code ~ '^[A-Z0-9][A-Z0-9-]{2,31}$'),
    percent_bp INTEGER NOT NULL CONSTRAINT discount_codes_percent_range CHECK (percent_bp BETWEEN 1 AND 10000),
    is_active BOOLEAN NOT NULL DEFAULT true,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT discount_codes_validity CHECK (valid_from IS NULL OR valid_until IS NULL OR valid_until > valid_from)
);
CREATE UNIQUE INDEX discount_codes_code_unique ON discount_codes (code);

CREATE TABLE cart_discounts (
    customer_id UUID PRIMARY KEY REFERENCES customers(id) ON DELETE CASCADE,
    discount_code_id UUID NOT NULL REFERENCES discount_codes(id) ON DELETE CASCADE,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX cart_discounts_code_idx ON cart_discounts (discount_code_id);

ALTER TABLE admin_audit_events DROP CONSTRAINT admin_audit_entity_type_valid;
ALTER TABLE admin_audit_events ADD CONSTRAINT admin_audit_entity_type_valid
    CHECK (entity_type IN ('category', 'product', 'variant', 'product_category', 'image', 'discount_code'));
