-- Trigram-Indizes unterstützen die öffentliche Teilwortsuche mit ILIKE.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX categories_name_trgm_idx ON categories USING gin (name gin_trgm_ops);
CREATE INDEX categories_description_trgm_idx ON categories USING gin (description gin_trgm_ops);
CREATE INDEX products_name_trgm_idx ON products USING gin (name gin_trgm_ops);
CREATE INDEX products_description_trgm_idx ON products USING gin (description gin_trgm_ops);
CREATE INDEX product_variants_sku_trgm_idx ON product_variants USING gin (sku gin_trgm_ops);
