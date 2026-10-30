LOCK TABLE auth_sessions, auth_rate_limits, customers IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM auth_sessions LIMIT 1)
        OR EXISTS (SELECT 1 FROM customers WHERE password_hash IS NOT NULL OR role <> 'customer' LIMIT 1) THEN
        RAISE EXCEPTION 'Identity-Daten vorhanden; Rollback verweigert';
    END IF;
END $$;
DROP TABLE auth_sessions;
DROP TABLE auth_rate_limits;
ALTER TABLE customers
    DROP CONSTRAINT customers_password_hash_valid,
    DROP CONSTRAINT customers_role_valid,
    DROP COLUMN password_hash,
    DROP COLUMN role;
