-- Rollback darf weder Mitarbeiterkonten noch neue Akteursnachweise verlieren.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM customers WHERE role = 'employee')
        OR EXISTS (SELECT 1 FROM auth_sessions WHERE role_at_issue = 'employee')
        OR EXISTS (SELECT 1 FROM order_status_events WHERE actor = 'employee' OR actor_customer_id IS NOT NULL) THEN
        RAISE EXCEPTION 'Mitarbeiter- oder Akteursdaten vorhanden; Rollback verweigert';
    END IF;
END $$;

ALTER TABLE order_status_events
    DROP COLUMN actor_customer_id,
    DROP CONSTRAINT order_status_events_actor_valid,
    ADD CONSTRAINT order_status_events_actor_valid CHECK (actor IN ('customer', 'admin', 'system'));

ALTER TABLE auth_sessions
    DROP CONSTRAINT auth_sessions_role_valid,
    ADD CONSTRAINT auth_sessions_role_valid CHECK (role_at_issue IN ('customer', 'admin'));

ALTER TABLE customers
    DROP CONSTRAINT customers_role_valid,
    ADD CONSTRAINT customers_role_valid CHECK (role IN ('customer', 'admin'));
