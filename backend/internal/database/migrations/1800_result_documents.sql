-- E7: result documents (PDF marksheets and class documents) and result
-- verification.
--
-- * result_documents is both the job queue and the cache. A document is
--   identified by what it shows (kind, scope, class, section, student,
--   language, provisional) and by source_key: a hash of the published
--   versions it was built from, the template and the school's branding. A
--   correction publishes new versions, so the next request gets a new key and
--   a new document; the old file is deleted once the new one is ready.
-- * result_verifications gives a published version a random token for the
--   QR code on its marksheet. The public check shows no personal data.
-- * exams:documents lets staff create class documents; the server limits
--   teachers to the classes they teach.
-- Nothing existing changes.

CREATE TABLE result_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('marksheet', 'marksheets', 'tabulation', 'merit_list', 'failed_list', 'summary')),
    scope text NOT NULL CHECK (scope IN ('term', 'annual')),
    scope_id uuid NOT NULL,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE CASCADE,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE CASCADE,
    section_id uuid REFERENCES academic_sections(id) ON DELETE CASCADE,
    student_id uuid REFERENCES students(id) ON DELETE CASCADE,
    language text NOT NULL CHECK (language IN ('en', 'bn')),
    provisional boolean NOT NULL DEFAULT false,
    source_key text NOT NULL CHECK (source_key ~ '^[0-9a-f]{64}$'),
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed')),
    progress_done integer NOT NULL DEFAULT 0 CHECK (progress_done >= 0),
    progress_total integer NOT NULL DEFAULT 0 CHECK (progress_total >= 0),
    pages integer,
    bytes bigint,
    sha256 text,
    storage_key text,
    attempts integer NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    CONSTRAINT result_documents_student_ck CHECK ((kind = 'marksheet') = (student_id IS NOT NULL)),
    CONSTRAINT result_documents_done_ck CHECK ((status = 'done') = (storage_key IS NOT NULL))
);
CREATE UNIQUE INDEX result_documents_identity_key ON result_documents (kind, scope, scope_id, class_id,
    coalesce(section_id, '00000000-0000-0000-0000-000000000000'::uuid), coalesce(student_id, '00000000-0000-0000-0000-000000000000'::uuid),
    language, provisional, source_key);
CREATE INDEX result_documents_queue_idx ON result_documents (created_at) WHERE status = 'queued';

CREATE TABLE result_verifications (
    version_id uuid PRIMARY KEY REFERENCES result_versions(id) ON DELETE RESTRICT,
    token text NOT NULL UNIQUE CHECK (token ~ '^[A-Za-z0-9_-]{22}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO permissions (id, description) VALUES
    ('exams:documents', 'Create and download class result documents (teachers: their classes only)')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'exams:documents'), ('head_teacher', 'exams:documents'), ('teacher', 'exams:documents')
ON CONFLICT DO NOTHING;
