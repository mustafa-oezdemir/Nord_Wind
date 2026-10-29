LOCK TABLE app_settings IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM app_settings LIMIT 1) THEN
        RAISE EXCEPTION 'app_settings enthält Daten; Rollback verweigert';
    END IF;
END $$;

DROP TABLE app_settings;
