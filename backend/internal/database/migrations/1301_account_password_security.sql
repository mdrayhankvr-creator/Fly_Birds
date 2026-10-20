-- Refresh tokens a session has already rotated away from. Presenting one again
-- means the token was copied, so the session it belongs to is revoked.
CREATE TABLE auth_refresh_history (
    refresh_hash bytea PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    rotated_at timestamptz NOT NULL
);

CREATE INDEX auth_refresh_history_session_idx ON auth_refresh_history (session_id, rotated_at);

INSERT INTO permissions (id, description) VALUES
    ('auth:password_change_self', 'Change own password')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'auth:password_change_self'),
    ('head_teacher', 'auth:password_change_self'),
    ('teacher', 'auth:password_change_self'),
    ('student', 'auth:password_change_self'),
    ('guardian', 'auth:password_change_self'),
    ('accountant', 'auth:password_change_self')
ON CONFLICT DO NOTHING;
