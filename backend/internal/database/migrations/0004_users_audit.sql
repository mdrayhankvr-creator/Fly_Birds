ALTER TABLE users ADD COLUMN display_name text NOT NULL DEFAULT '';
CREATE INDEX users_email_search_idx ON users (email);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now(),
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('success', 'denied')),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_events_created_idx ON audit_events (created_at DESC, id DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, created_at DESC);
CREATE INDEX audit_events_action_idx ON audit_events (action, created_at DESC);

INSERT INTO permissions (id, description) VALUES
    ('audit:read', 'Read audit events')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
VALUES ('admin', 'audit:read') ON CONFLICT DO NOTHING;
