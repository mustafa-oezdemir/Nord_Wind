DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM discount_codes)
        OR EXISTS (SELECT 1 FROM cart_discounts)
        OR EXISTS (SELECT 1 FROM admin_audit_events WHERE entity_type = 'discount_code')
        OR EXISTS (SELECT 1 FROM products WHERE tax_class <> 'standard') THEN
        RAISE EXCEPTION 'Preisdaten vorhanden; Down verweigert Datenverlust';
    END IF;
END $$;
ALTER TABLE admin_audit_events DROP CONSTRAINT admin_audit_entity_type_valid;
ALTER TABLE admin_audit_events ADD CONSTRAINT admin_audit_entity_type_valid
    CHECK (entity_type IN ('category', 'product', 'variant', 'product_category', 'image'));
DROP TABLE cart_discounts;
DROP TABLE discount_codes;
ALTER TABLE products DROP COLUMN tax_class;
