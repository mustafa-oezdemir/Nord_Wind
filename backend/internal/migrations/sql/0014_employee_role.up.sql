-- Mitarbeiter dürfen nur den Versandbereich bedienen; die Registrierung bleibt customer.
ALTER TABLE customers
    DROP CONSTRAINT customers_role_valid,
    ADD CONSTRAINT customers_role_valid CHECK (role IN ('customer', 'admin', 'employee'));

ALTER TABLE auth_sessions
    DROP CONSTRAINT auth_sessions_role_valid,
    ADD CONSTRAINT auth_sessions_role_valid CHECK (role_at_issue IN ('customer', 'admin', 'employee'));

ALTER TABLE order_status_events
    DROP CONSTRAINT order_status_events_actor_valid,
    ADD CONSTRAINT order_status_events_actor_valid CHECK (actor IN ('customer', 'admin', 'employee', 'system')),
    ADD COLUMN actor_customer_id UUID REFERENCES customers(id) ON DELETE RESTRICT;
