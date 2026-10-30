DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM order_reservations) THEN
        RAISE EXCEPTION 'Reservierungen vorhanden; Migration 10 kann nicht zurückgenommen werden';
    END IF;
END $$;

DROP TABLE order_reservations;
