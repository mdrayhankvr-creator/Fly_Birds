-- Module dependencies: a module may be enabled only when every module it
-- depends on is enabled, and may be disabled only when no enabled module
-- depends on it. Core modules have no dependencies.
ALTER TABLE module_definitions ADD COLUMN depends_on text[] NOT NULL DEFAULT '{}';
UPDATE module_definitions SET depends_on = CASE key
    WHEN 'students' THEN ARRAY['academics']
    WHEN 'staff' THEN ARRAY['academics']
    WHEN 'attendance' THEN ARRAY['students', 'staff']
    WHEN 'routine' THEN ARRAY['academics', 'staff']
    WHEN 'examinations' THEN ARRAY['students', 'staff']
    WHEN 'fees' THEN ARRAY['students']
    WHEN 'diary' THEN ARRAY['students', 'staff']
    ELSE '{}'::text[]
END;
ALTER TABLE module_definitions ADD CONSTRAINT module_definitions_no_self_dependency CHECK (NOT (key = ANY(depends_on)));

-- Backup runs record every snapshot; archives live under BACKUP_DIR, never in
-- the database. The checksum lets an administrator verify an archive later.
CREATE TABLE backup_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed')),
    trigger text NOT NULL CHECK (trigger IN ('manual', 'scheduled')),
    file_name text CHECK (file_name IS NULL OR file_name ~ '^flybirds-backup-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{8}\.tar\.gz$'),
    size_bytes bigint CHECK (size_bytes IS NULL OR size_bytes > 0),
    sha256 text CHECK (sha256 IS NULL OR sha256 ~ '^[0-9a-f]{64}$'),
    table_count integer CHECK (table_count IS NULL OR table_count >= 0),
    row_count bigint CHECK (row_count IS NULL OR row_count >= 0),
    file_count integer CHECK (file_count IS NULL OR file_count >= 0),
    schema_version integer,
    error text NOT NULL DEFAULT '' CHECK (octet_length(error) <= 500),
    started_by uuid REFERENCES users(id) ON DELETE SET NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    deleted_at timestamptz,
    CONSTRAINT backup_runs_completed_ck CHECK (status <> 'completed' OR (file_name IS NOT NULL AND size_bytes IS NOT NULL AND sha256 IS NOT NULL AND finished_at IS NOT NULL))
);
CREATE UNIQUE INDEX backup_runs_one_running_idx ON backup_runs ((status)) WHERE status = 'running';
CREATE INDEX backup_runs_recent_idx ON backup_runs (started_at DESC);

CREATE INDEX audit_events_target_idx ON audit_events (target_type, target_id, created_at DESC);

INSERT INTO permissions (id, description) VALUES
    ('system:read', 'Read the administrator control center overview'),
    ('data:export', 'Export school data as CSV files'),
    ('backups:manage', 'Create, verify, download and delete database backups')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'system:read'), ('admin', 'data:export'), ('admin', 'backups:manage')
ON CONFLICT DO NOTHING;
