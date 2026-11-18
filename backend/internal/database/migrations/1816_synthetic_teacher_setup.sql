-- A synthetic teacher can be created before the school supplies an actual
-- joining date. Ordinary staff still require one. No existing row is changed.
ALTER TABLE employees ADD COLUMN synthetic_test boolean NOT NULL DEFAULT false;
ALTER TABLE employees ALTER COLUMN joined_on DROP NOT NULL;
ALTER TABLE employees ADD CONSTRAINT employee_joined_on_required_ck
    CHECK (joined_on IS NOT NULL OR (kind = 'teacher' AND synthetic_test AND left_on IS NULL));

-- A retry records only the outcome, never a password or encrypted export.
CREATE TABLE synthetic_teacher_setup_runs (
    idempotency_key uuid PRIMARY KEY,
    fingerprint text NOT NULL CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    created integer NOT NULL CHECK (created BETWEEN 0 AND 15),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
