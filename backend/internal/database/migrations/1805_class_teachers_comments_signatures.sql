-- Marksheet v2: class teachers, their comments on each student's result, and
-- staff signatures printed on documents.
--
-- * One class teacher per section (sections already belong to one session).
--   The administrator assigns them.
-- * The class teacher writes one comment per student per term or annual
--   result: a preset (editable, English and Bangla) or their own words. The
--   head teacher (or the administrator) can review it. A comment is printed
--   on the marksheet; it never changes a published result, and a changed
--   comment makes a new document (the document cache key includes it).
-- * Each member of staff uploads their own signature: a PNG with a
--   transparent background, resized on upload.

CREATE TABLE section_class_teachers (
    section_id uuid PRIMARY KEY,
    class_id uuid NOT NULL,
    session_id uuid NOT NULL,
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    assigned_by uuid REFERENCES users(id) ON DELETE SET NULL,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT section_class_teachers_section_fk FOREIGN KEY (section_id, class_id, session_id)
        REFERENCES academic_sections(id, class_id, session_id) ON DELETE CASCADE
);
CREATE INDEX section_class_teachers_employee_idx ON section_class_teachers (employee_id, session_id);

CREATE TABLE employee_signatures (
    employee_id uuid PRIMARY KEY REFERENCES employees(id) ON DELETE CASCADE,
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 262144),
    version text NOT NULL CHECK (version ~ '^[0-9a-f]{12}$'),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE result_comment_presets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    text_en text NOT NULL CHECK (text_en = btrim(text_en) AND text_en <> '' AND char_length(text_en) <= 300),
    text_bn text NOT NULL CHECK (text_bn = btrim(text_bn) AND text_bn <> '' AND char_length(text_bn) <= 300),
    display_order integer NOT NULL DEFAULT 0 CHECK (display_order BETWEEN 0 AND 10000),
    is_active boolean NOT NULL DEFAULT true,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO result_comment_presets (text_en, text_bn, display_order) VALUES
    ('Excellent work. Keep it up.', 'চমৎকার ফলাফল। এভাবেই এগিয়ে যাও।', 10),
    ('Very good progress this term.', 'এই সময়ে খুব ভালো অগ্রগতি হয়েছে।', 20),
    ('Good effort; can do even better with regular practice.', 'ভালো চেষ্টা; নিয়মিত অনুশীলনে আরও ভালো করতে পারবে।', 30),
    ('Needs to pay more attention in class.', 'ক্লাসে আরও মনোযোগী হতে হবে।', 40),
    ('Needs extra care in the weaker subjects.', 'দুর্বল বিষয়গুলোতে বাড়তি যত্ন প্রয়োজন।', 50),
    ('Attendance must improve.', 'উপস্থিতি বাড়াতে হবে।', 60);

-- A comment is a preset (printed in the document's language) or the
-- teacher's own text (printed as written), never both.
CREATE TABLE result_comments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope text NOT NULL CHECK (scope IN ('term', 'annual')),
    scope_id uuid NOT NULL,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL REFERENCES academic_sections(id) ON DELETE RESTRICT,
    preset_id uuid REFERENCES result_comment_presets(id) ON DELETE RESTRICT,
    custom_text text NOT NULL DEFAULT '' CHECK (custom_text = btrim(custom_text) AND char_length(custom_text) <= 400),
    status text NOT NULL DEFAULT 'written' CHECK (status IN ('written', 'reviewed')),
    written_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    written_at timestamptz NOT NULL DEFAULT now(),
    reviewed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at timestamptz,
    CONSTRAINT result_comments_key UNIQUE (scope, scope_id, student_id),
    CONSTRAINT result_comments_text_ck CHECK ((preset_id IS NULL) <> (custom_text = '')),
    CONSTRAINT result_comments_review_ck CHECK ((status = 'reviewed') = (reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL))
);
CREATE INDEX result_comments_section_idx ON result_comments (scope, scope_id, section_id);

INSERT INTO permissions (id, description) VALUES
    ('exams:comment', 'Write class-teacher comments on results (teachers: their own sections) and review them (head teacher)')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'exams:comment'), ('head_teacher', 'exams:comment'), ('teacher', 'exams:comment')
ON CONFLICT DO NOTHING;
