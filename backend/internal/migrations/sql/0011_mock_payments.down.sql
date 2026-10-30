DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM mock_payment_events) OR EXISTS (SELECT 1 FROM mock_payments) THEN
        RAISE EXCEPTION 'Mock-Zahlungen vorhanden; Migration 11 kann nicht zurückgenommen werden';
    END IF;
END $$;

DROP TABLE mock_payment_events;
DROP TABLE mock_payments;
