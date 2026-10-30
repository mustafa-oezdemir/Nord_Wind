CREATE TABLE admin_audit_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    entity_type TEXT NOT NULL CONSTRAINT admin_audit_entity_type_valid CHECK (entity_type IN ('category', 'product', 'variant', 'product_category')),
    entity_id UUID NOT NULL,
    related_id UUID,
    action TEXT NOT NULL CONSTRAINT admin_audit_action_valid CHECK (action IN ('create', 'update', 'delete', 'attach', 'detach')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX admin_audit_recent_idx ON admin_audit_events (created_at DESC, id DESC);
CREATE INDEX admin_audit_actor_idx ON admin_audit_events (actor_customer_id, created_at DESC);
