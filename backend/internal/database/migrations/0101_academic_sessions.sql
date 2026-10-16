CREATE TABLE academic_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    name_key text GENERATED ALWAYS AS (replace(name, '–', '-')) STORED UNIQUE,
    start_date date NOT NULL,
    end_date date NOT NULL,
    is_active boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_sessions_dates_ck CHECK (end_date > start_date),
    CONSTRAINT academic_sessions_name_ck CHECK (name ~ '^[0-9]{4}([-–][0-9]{4})?$')
);
CREATE UNIQUE INDEX academic_sessions_one_active_idx ON academic_sessions (is_active) WHERE is_active;
CREATE INDEX academic_sessions_start_idx ON academic_sessions (start_date DESC, id DESC);

INSERT INTO permissions (id, description) VALUES
    ('academic_sessions:read', 'Read academic sessions'),
    ('academic_sessions:create', 'Create academic sessions'),
    ('academic_sessions:update', 'Edit academic sessions'),
    ('academic_sessions:activate', 'Activate or deactivate academic sessions')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id IN (
    'academic_sessions:read', 'academic_sessions:create',
    'academic_sessions:update', 'academic_sessions:activate'
) ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
VALUES ('head_teacher', 'academic_sessions:read') ON CONFLICT DO NOTHING;
