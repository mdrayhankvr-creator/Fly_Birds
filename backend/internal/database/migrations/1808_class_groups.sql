-- Groups for classes 9–10 (বিভাগ): Science, Humanities and Business Studies
-- by default, school-editable. A class opts in with uses_groups; classes
-- without groups never show the fields anywhere.
--
-- Each group's subjects are mapped per class (class_group_subjects): a
-- subject mapped to any group of a class is taken only by the students whose
-- chosen group includes it; unmapped subjects stay common to the class. The
-- student's group is per session (student_group_choices), like the fourth
-- subject choice, so an old year's results keep that year's group.
--
-- student_takes_subject gains the group rule; every roster, sheet, mark and
-- result query already goes through it.

CREATE TABLE academic_groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name_en text NOT NULL,
    name_bn text NOT NULL DEFAULT '',
    display_order integer NOT NULL DEFAULT 0,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_groups_name_en_ck CHECK (name_en <> '' AND name_en = btrim(name_en) AND octet_length(name_en) <= 80),
    CONSTRAINT academic_groups_name_bn_ck CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 80),
    CONSTRAINT academic_groups_order_ck CHECK (display_order BETWEEN 0 AND 100000)
);
CREATE UNIQUE INDEX academic_groups_name_key ON academic_groups (lower(name_en));
CREATE INDEX academic_groups_active_idx ON academic_groups (is_active, display_order, id);

INSERT INTO academic_groups (name_en, name_bn, display_order) VALUES
    ('Science', 'বিজ্ঞান', 1),
    ('Humanities', 'মানবিক', 2),
    ('Business Studies', 'ব্যবসায় শিক্ষা', 3);

ALTER TABLE academic_classes ADD COLUMN uses_groups boolean NOT NULL DEFAULT false;

-- A group's subjects in one class. The pair FK keeps every mapped subject
-- one the class teaches; dropping the class subject drops the mapping.
CREATE TABLE class_group_subjects (
    class_id uuid NOT NULL,
    group_id uuid NOT NULL REFERENCES academic_groups(id) ON UPDATE RESTRICT ON DELETE CASCADE,
    subject_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (class_id, group_id, subject_id),
    CONSTRAINT class_group_subjects_class_subject_fk FOREIGN KEY (class_id, subject_id)
        REFERENCES academic_class_subjects(class_id, subject_id) ON UPDATE RESTRICT ON DELETE CASCADE
);
CREATE INDEX class_group_subjects_subject_idx ON class_group_subjects (class_id, subject_id);

-- Each student's group for a session (only meaningful while the student's
-- class of that session uses groups).
CREATE TABLE student_group_choices (
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    group_id uuid NOT NULL REFERENCES academic_groups(id) ON DELETE RESTRICT,
    updated_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, session_id)
);
CREATE INDEX student_group_choices_group_idx ON student_group_choices (session_id, group_id);

-- The fourth-subject rule as before, and the group rule: a subject mapped to
-- groups of a class (while the class uses groups) is taken only by students
-- whose chosen group includes it.
CREATE OR REPLACE FUNCTION student_takes_subject(p_student uuid, p_session uuid, p_class uuid, p_subject uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT (NOT coalesce((SELECT acs.is_fourth_subject FROM academic_class_subjects acs WHERE acs.class_id = p_class AND acs.subject_id = p_subject), false)
        OR NOT EXISTS (SELECT 1 FROM student_subject_choices c JOIN students s ON s.id = c.student_id
                       WHERE c.session_id = p_session AND s.session_id = p_session AND s.class_id = p_class)
        OR EXISTS (SELECT 1 FROM student_subject_choices c
                   WHERE c.student_id = p_student AND c.session_id = p_session AND c.subject_id = p_subject))
       AND (NOT coalesce((SELECT uses_groups FROM academic_classes WHERE id = p_class), false)
        OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs WHERE gs.class_id = p_class AND gs.subject_id = p_subject)
        OR EXISTS (SELECT 1 FROM class_group_subjects gs
                   JOIN student_group_choices gc ON gc.group_id = gs.group_id AND gc.student_id = p_student AND gc.session_id = p_session
                   WHERE gs.class_id = p_class AND gs.subject_id = p_subject))
$$;
