INSERT INTO module_definitions(key, name, is_core) VALUES ('diary', 'Daily class diary', false) ON CONFLICT (key) DO NOTHING;
INSERT INTO school_modules(school_id, module_key, enabled) VALUES (1, 'diary', false) ON CONFLICT (school_id, module_key) DO NOTHING;

CREATE TABLE diary_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    daily_required boolean NOT NULL DEFAULT false,
    reminder_time time NOT NULL DEFAULT '16:00',
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO diary_settings(id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- One diary per section, subject and day. The teacher must hold a teacher
-- assignment for the class and subject in the session (checked in the service).
CREATE TABLE class_diaries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    teacher_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    diary_date date NOT NULL,
    lesson_topics text NOT NULL DEFAULT '' CHECK (octet_length(lesson_topics) <= 10000),
    class_notes text NOT NULL DEFAULT '' CHECK (octet_length(class_notes) <= 20000),
    homework text NOT NULL DEFAULT '' CHECK (octet_length(homework) <= 10000),
    instructions text NOT NULL DEFAULT '' CHECK (octet_length(instructions) <= 10000),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published')),
    published_at timestamptz,
    submission_enabled boolean NOT NULL DEFAULT false,
    submission_due_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT class_diaries_section_fk FOREIGN KEY (section_id, class_id, session_id) REFERENCES academic_sections(id, class_id, session_id) ON DELETE RESTRICT,
    CONSTRAINT class_diaries_subject_fk FOREIGN KEY (class_id, subject_id) REFERENCES academic_class_subjects(class_id, subject_id) ON DELETE RESTRICT,
    CONSTRAINT class_diaries_day_key UNIQUE (section_id, subject_id, diary_date),
    CONSTRAINT class_diaries_published_ck CHECK ((status = 'published') = (published_at IS NOT NULL)),
    CONSTRAINT class_diaries_content_ck CHECK (status = 'draft' OR lesson_topics <> '' OR class_notes <> '' OR homework <> '' OR instructions <> ''),
    CONSTRAINT class_diaries_due_ck CHECK (submission_due_at IS NULL OR submission_enabled)
);
CREATE INDEX class_diaries_feed_idx ON class_diaries(section_id, diary_date DESC) WHERE status = 'published';
CREATE INDEX class_diaries_teacher_idx ON class_diaries(teacher_user_id, diary_date DESC);

CREATE TABLE diary_attachments (
    diary_id uuid NOT NULL REFERENCES class_diaries(id) ON DELETE CASCADE,
    file_id uuid NOT NULL UNIQUE REFERENCES file_objects(id) ON DELETE RESTRICT,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 9),
    PRIMARY KEY (diary_id, file_id)
);

CREATE TABLE homework_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    diary_id uuid NOT NULL REFERENCES class_diaries(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    answer_text text NOT NULL DEFAULT '' CHECK (octet_length(answer_text) <= 10000),
    status text NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'reviewed')),
    review_note text NOT NULL DEFAULT '' CHECK (octet_length(review_note) <= 2000),
    reviewed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at timestamptz,
    submitted_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT homework_submissions_key UNIQUE (diary_id, student_id),
    CONSTRAINT homework_submissions_review_ck CHECK ((status = 'reviewed') = (reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL))
);

CREATE TABLE homework_submission_files (
    submission_id uuid NOT NULL REFERENCES homework_submissions(id) ON DELETE CASCADE,
    file_id uuid NOT NULL UNIQUE REFERENCES file_objects(id) ON DELETE RESTRICT,
    position smallint NOT NULL CHECK (position BETWEEN 0 AND 4),
    PRIMARY KEY (submission_id, file_id)
);

INSERT INTO permissions(id, description) VALUES
 ('diary:write', 'Write and publish class diaries for assigned classes and subjects, and review homework'),
 ('diary:read', 'Read all class diaries for oversight'),
 ('diary:read_self', 'Read own class diary and submit homework'),
 ('diary:read_children', 'Read linked children class diaries'),
 ('diary:manage', 'Configure the class diary and daily publication rules')
ON CONFLICT(id) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id) VALUES
 ('teacher', 'diary:write'), ('head_teacher', 'diary:write'), ('head_teacher', 'diary:read'),
 ('admin', 'diary:read'), ('admin', 'diary:manage'),
 ('student', 'diary:read_self'), ('guardian', 'diary:read_children')
ON CONFLICT DO NOTHING;
