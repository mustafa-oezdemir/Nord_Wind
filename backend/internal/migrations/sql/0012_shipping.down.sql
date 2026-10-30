DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM order_shipments) OR EXISTS (SELECT 1 FROM shipment_attempts) THEN
        RAISE EXCEPTION 'Versanddaten vorhanden; Migration 12 kann nicht zurückgesetzt werden';
    END IF;
END $$;
DROP TABLE shipment_attempts;
DROP TABLE order_shipments;
