DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM product_images) OR EXISTS (SELECT 1 FROM admin_audit_events WHERE entity_type = 'image') THEN
        RAISE EXCEPTION 'Produktbilder oder Bild-Audit-Ereignisse vorhanden; Down verweigert Datenverlust';
    END IF;
END $$;
DROP TABLE product_images;
ALTER TABLE admin_audit_events DROP CONSTRAINT admin_audit_entity_type_valid;
ALTER TABLE admin_audit_events ADD CONSTRAINT admin_audit_entity_type_valid
    CHECK (entity_type IN ('category', 'product', 'variant', 'product_category'));
