-- Checkpoint 5: bulk admission from a spreadsheet.
--
-- Each upload that admits students is recorded under the idempotency key the
-- browser made for it, with the file's hash and what it did, so a retried
-- request (a double click, a dropped connection) answers with the first
-- result instead of admitting anyone twice. Rows already admitted by an
-- earlier upload are recognised by their admission number and skipped.
CREATE TABLE bulk_admissions (
    idempotency_key uuid PRIMARY KEY,
    file_sha256 text NOT NULL CHECK (file_sha256 ~ '^[0-9a-f]{64}$'),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE CASCADE,
    admitted integer NOT NULL CHECK (admitted >= 0),
    skipped integer NOT NULL CHECK (skipped >= 0),
    logins integer NOT NULL CHECK (logins >= 0),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
