-- One-time synthetic student setup. A retry records its outcome, never its
-- temporary credentials. The identifier manifest and state are rechecked
-- before the first write.
CREATE TABLE student_setup_runs (
    idempotency_key uuid PRIMARY KEY,
    fingerprint text NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    admitted integer NOT NULL CHECK (admitted >= 0),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
