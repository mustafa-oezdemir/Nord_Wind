LOCK TABLE admin_audit_events IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM admin_audit_events LIMIT 1) THEN
        RAISE EXCEPTION 'Audit-Ereignisse vorhanden; Rollback verweigert';
    END IF;
END $$;
DROP TABLE admin_audit_events;
