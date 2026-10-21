-- E3: mark statuses and the sheet workflow. A sheet is an exam_subjects row
-- (one exam, which belongs to one section, × one subject). No mark, grade or
-- result value changes here: existing marks become 'present', sheets of
-- published or locked exams become 'approved', and sheets of draft exams
-- become 'in_progress' (some marks entered) or 'not_started'.

-- Marks: a status, an optional reason (exempt and withheld need one) and a
-- version for optimistic concurrency. Only 'present' requires marks; absent,
-- exempt and incomplete carry none; withheld may keep its marks.
ALTER TABLE exam_marks
    ADD COLUMN status text NOT NULL DEFAULT 'present' CHECK (status IN ('present', 'absent', 'exempt', 'incomplete', 'withheld')),
    ADD COLUMN status_reason text NOT NULL DEFAULT '' CHECK (status_reason = btrim(status_reason) AND octet_length(status_reason) <= 240),
    ADD COLUMN row_version integer NOT NULL DEFAULT 1 CHECK (row_version >= 1),
    ALTER COLUMN marks DROP NOT NULL;
ALTER TABLE exam_marks
    ADD CONSTRAINT exam_marks_status_marks_ck CHECK (
        (status = 'present' AND marks IS NOT NULL)
        OR (status IN ('absent', 'exempt', 'incomplete') AND marks IS NULL)
        OR status = 'withheld'),
    ADD CONSTRAINT exam_marks_status_reason_ck CHECK (status NOT IN ('exempt', 'withheld') OR status_reason <> '');

-- Sheets: the workflow state of each exam subject.
ALTER TABLE exam_subjects
    ADD COLUMN sheet_status text NOT NULL DEFAULT 'not_started' CHECK (sheet_status IN ('not_started', 'in_progress', 'submitted', 'returned', 'approved')),
    ADD COLUMN submitted_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN submitted_at timestamptz,
    ADD COLUMN reviewed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN reviewed_at timestamptz,
    ADD COLUMN return_reason text NOT NULL DEFAULT '' CHECK (return_reason = btrim(return_reason) AND octet_length(return_reason) <= 240);
ALTER TABLE exam_subjects
    ADD CONSTRAINT exam_subjects_submitted_ck CHECK (sheet_status <> 'submitted' OR (submitted_by IS NOT NULL AND submitted_at IS NOT NULL)),
    -- reviewed_by is NULL only for sheets approved by this migration.
    ADD CONSTRAINT exam_subjects_reviewed_ck CHECK (sheet_status NOT IN ('approved', 'returned') OR reviewed_at IS NOT NULL),
    ADD CONSTRAINT exam_subjects_returned_ck CHECK (sheet_status <> 'returned' OR return_reason <> '');

UPDATE exam_subjects es SET sheet_status = 'approved', reviewed_at = coalesce(e.published_at, e.updated_at)
FROM exams e WHERE e.id = es.exam_id AND e.status IN ('published', 'locked');
UPDATE exam_subjects es SET sheet_status = 'in_progress'
FROM exams e WHERE e.id = es.exam_id AND e.status = 'draft'
  AND EXISTS (SELECT 1 FROM exam_marks m WHERE m.exam_subject_id = es.id);

CREATE INDEX exam_subjects_sheet_status_idx ON exam_subjects (sheet_status, exam_id);

-- Reviewing sheets: the head teacher (and the administrator) approve or
-- return submitted sheets.
INSERT INTO permissions (id, description) VALUES
    ('exams:review', 'Approve or return submitted mark sheets')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'exams:review'), ('head_teacher', 'exams:review')
ON CONFLICT DO NOTHING;

-- What the upgrade did, for the school's audit log.
INSERT INTO audit_events (action, target_type, target_id, outcome, metadata)
SELECT 'exam.sheets_migrated', 'school', 'exams', 'success',
       jsonb_build_object('approved', count(*) FILTER (WHERE sheet_status = 'approved'),
                          'in_progress', count(*) FILTER (WHERE sheet_status = 'in_progress'),
                          'not_started', count(*) FILTER (WHERE sheet_status = 'not_started'),
                          'marks', (SELECT count(*) FROM exam_marks), 'migration', 1501)
FROM exam_subjects
HAVING count(*) > 0;
