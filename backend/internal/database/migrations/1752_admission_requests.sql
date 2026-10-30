-- Checkpoint 3: retry safety for the one-page admission.
--
-- Each admission request carries an idempotency key. The key is stored with
-- the student it created, in the admission's own transaction, so a retried
-- request (a double submit or a lost response) finds the original admission
-- instead of creating a second one or failing on the admission number.
CREATE TABLE admission_requests (
    idempotency_key uuid PRIMARY KEY,
    student_id uuid NOT NULL UNIQUE REFERENCES students(id) ON DELETE CASCADE,
    admission_no text NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
