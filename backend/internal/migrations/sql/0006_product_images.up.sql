CREATE TABLE product_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    alt_text TEXT NOT NULL CONSTRAINT product_images_alt_valid CHECK (char_length(alt_text) BETWEEN 1 AND 200 AND btrim(alt_text) <> ''),
    content_type TEXT NOT NULL DEFAULT 'image/jpeg' CONSTRAINT product_images_type_jpeg CHECK (content_type = 'image/jpeg'),
    width INTEGER NOT NULL CONSTRAINT product_images_width_valid CHECK (width BETWEEN 1 AND 2000),
    height INTEGER NOT NULL CONSTRAINT product_images_height_valid CHECK (height BETWEEN 1 AND 2000),
    image_bytes BYTEA NOT NULL CONSTRAINT product_images_size_valid CHECK (octet_length(image_bytes) BETWEEN 1 AND 3145728),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX product_images_product_order_idx ON product_images (product_id, created_at, id);
ALTER TABLE admin_audit_events DROP CONSTRAINT admin_audit_entity_type_valid;
ALTER TABLE admin_audit_events ADD CONSTRAINT admin_audit_entity_type_valid
    CHECK (entity_type IN ('category', 'product', 'variant', 'product_category', 'image'));
