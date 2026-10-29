-- Bestehende Katalogdaten und eine eventuell gemeinsam genutzte Extension bleiben erhalten.
DROP INDEX product_variants_sku_trgm_idx;
DROP INDEX products_description_trgm_idx;
DROP INDEX products_name_trgm_idx;
DROP INDEX categories_description_trgm_idx;
DROP INDEX categories_name_trgm_idx;
