DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM customer_addresses) OR EXISTS (SELECT 1 FROM customers WHERE anonymized_at IS NOT NULL) THEN
        RAISE EXCEPTION 'Kontodaten vorhanden; Migration 13 kann nicht zurückgesetzt werden';
    END IF;
END $$;
ALTER TABLE customers DROP COLUMN anonymized_at;
DROP TABLE customer_addresses;
