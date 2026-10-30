ALTER TABLE customers
    ADD COLUMN password_hash TEXT,
    ADD COLUMN role TEXT NOT NULL DEFAULT 'customer',
    ADD CONSTRAINT customers_role_valid CHECK (role IN ('customer', 'admin')),
    ADD CONSTRAINT customers_password_hash_valid CHECK (password_hash IS NULL OR length(password_hash) BETWEEN 60 AND 512);

CREATE TABLE auth_sessions (
    token_hash BYTEA PRIMARY KEY CONSTRAINT auth_sessions_token_hash_length CHECK (octet_length(token_hash) = 32),
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    role_at_issue TEXT NOT NULL CONSTRAINT auth_sessions_role_valid CHECK (role_at_issue IN ('customer', 'admin')),
    csrf_token TEXT NOT NULL CONSTRAINT auth_sessions_csrf_length CHECK (length(csrf_token) = 43),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    idle_expires_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT auth_sessions_expiry_valid CHECK (expires_at > created_at AND idle_expires_at > created_at)
);
CREATE INDEX auth_sessions_customer_idx ON auth_sessions (customer_id);
CREATE INDEX auth_sessions_expiry_idx ON auth_sessions (expires_at);

CREATE TABLE auth_rate_limits (
    key_hash BYTEA PRIMARY KEY CONSTRAINT auth_rate_limits_key_length CHECK (octet_length(key_hash) = 32),
    window_started_at TIMESTAMPTZ NOT NULL,
    attempts INTEGER NOT NULL CONSTRAINT auth_rate_limits_attempts_positive CHECK (attempts > 0)
);
CREATE INDEX auth_rate_limits_window_idx ON auth_rate_limits (window_started_at);
