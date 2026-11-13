-- Students upload their own profile photo; it waits for approval.
--
-- * A school setting turns student uploads on or off (on by default).
-- * The student's browser crops the photo to a square and resizes it; the
--   server decodes it, checks it is square and re-encodes it as a JPEG, which
--   also drops any camera metadata.
-- * One submission per student. The class teacher of the student's section,
--   the head teacher or the administrator approves it, or rejects it with a
--   reason the student sees. A new upload replaces a pending or rejected one.
-- * Only an approved photo is used: approval copies it into student_photos,
--   which the profile, the ID card, the marksheet and the admit card read.
--   The approved submission keeps no copy of the image.

ALTER TABLE school_settings ADD COLUMN student_photo_uploads boolean NOT NULL DEFAULT true;

CREATE TABLE student_photo_submissions (
    student_id uuid PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    content bytea CHECK (content IS NULL OR octet_length(content) BETWEEN 1 AND 524288),
    reason text NOT NULL DEFAULT '' CHECK (reason = btrim(reason) AND char_length(reason) <= 300),
    submitted_by uuid REFERENCES users(id) ON DELETE SET NULL,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    CONSTRAINT student_photo_submissions_content_ck CHECK ((status = 'approved') = (content IS NULL)),
    CONSTRAINT student_photo_submissions_reason_ck CHECK ((status = 'rejected') = (reason <> '')),
    CONSTRAINT student_photo_submissions_review_ck CHECK ((status = 'pending') = (reviewed_at IS NULL))
);
CREATE INDEX student_photo_submissions_pending_idx ON student_photo_submissions (submitted_at) WHERE status = 'pending';

INSERT INTO permissions (id, description) VALUES
    ('students:photo_submit', 'Upload your own profile photo for approval (students)'),
    ('students:photo_review', 'Approve or reject student photos (teachers: the sections they are class teacher of)')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('student', 'students:photo_submit'),
    ('admin', 'students:photo_review'), ('head_teacher', 'students:photo_review'), ('teacher', 'students:photo_review')
ON CONFLICT DO NOTHING;
