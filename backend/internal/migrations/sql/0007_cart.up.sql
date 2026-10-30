CREATE TABLE cart_items (
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    variant_id UUID NOT NULL REFERENCES product_variants(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL CONSTRAINT cart_items_quantity_valid CHECK (quantity BETWEEN 1 AND 99),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (customer_id, variant_id)
);
CREATE INDEX cart_items_variant_idx ON cart_items (variant_id);
