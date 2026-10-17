CREATE TABLE exams (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 120),
    exam_type text NOT NULL CHECK (exam_type IN ('class_test', 'half_yearly', 'annual', 'other')),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    routine_id uuid REFERENCES exam_routines(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'locked')),
    published_at timestamptz,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT exams_section_fk FOREIGN KEY (section_id,class_id,session_id) REFERENCES academic_sections(id,class_id,session_id) ON DELETE RESTRICT,
    CONSTRAINT exams_context_key UNIQUE (id,session_id,class_id,section_id),
    CONSTRAINT exams_id_class_key UNIQUE (id,class_id)
);
CREATE UNIQUE INDEX exams_name_key ON exams(session_id,section_id,lower(name));
CREATE INDEX exams_lookup_idx ON exams(session_id,class_id,section_id,status,name);
CREATE UNIQUE INDEX exams_routine_key ON exams(routine_id) WHERE routine_id IS NOT NULL;

CREATE TABLE exam_subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    exam_id uuid NOT NULL,
    class_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    full_marks numeric(6,2) NOT NULL CHECK (full_marks > 0 AND full_marks <= 1000),
    pass_marks numeric(6,2) NOT NULL CHECK (pass_marks >= 0 AND pass_marks <= full_marks),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT exam_subjects_exam_fk FOREIGN KEY (exam_id,class_id) REFERENCES exams(id,class_id) ON DELETE CASCADE,
    CONSTRAINT exam_subjects_mapping_fk FOREIGN KEY (class_id,subject_id) REFERENCES academic_class_subjects(class_id,subject_id) ON DELETE RESTRICT,
    CONSTRAINT exam_subjects_pair_key UNIQUE (exam_id,subject_id),
    CONSTRAINT exam_subjects_id_exam_key UNIQUE (id,exam_id)
);

ALTER TABLE students ADD CONSTRAINT students_exam_context_key UNIQUE(id,session_id,class_id,section_id);
CREATE TABLE exam_marks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    exam_id uuid NOT NULL,
    exam_subject_id uuid NOT NULL,
    student_id uuid NOT NULL,
    session_id uuid NOT NULL,
    class_id uuid NOT NULL,
    section_id uuid NOT NULL,
    marks numeric(6,2) NOT NULL CHECK (marks >= 0),
    grade text,
    grade_point numeric(3,2) CHECK (grade_point BETWEEN 0 AND 5),
    passed boolean,
    entered_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT exam_marks_subject_fk FOREIGN KEY (exam_subject_id,exam_id) REFERENCES exam_subjects(id,exam_id) ON DELETE CASCADE,
    CONSTRAINT exam_marks_exam_fk FOREIGN KEY (exam_id,session_id,class_id,section_id) REFERENCES exams(id,session_id,class_id,section_id) ON DELETE CASCADE,
    CONSTRAINT exam_marks_student_fk FOREIGN KEY (student_id,session_id,class_id,section_id) REFERENCES students(id,session_id,class_id,section_id) ON DELETE RESTRICT,
    CONSTRAINT exam_marks_pair_key UNIQUE (exam_subject_id,student_id)
);
CREATE INDEX exam_marks_student_idx ON exam_marks(exam_id,student_id);

CREATE FUNCTION exam_marks_range_check() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE allowed numeric(6,2);
BEGIN
    SELECT full_marks INTO allowed FROM exam_subjects WHERE id=NEW.exam_subject_id AND exam_id=NEW.exam_id;
    IF allowed IS NULL OR NEW.marks > allowed THEN
        RAISE EXCEPTION 'marks exceed configured full marks' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER exam_marks_range_trigger BEFORE INSERT OR UPDATE OF marks,exam_subject_id,exam_id ON exam_marks
FOR EACH ROW EXECUTE FUNCTION exam_marks_range_check();

CREATE TABLE exam_results (
    exam_id uuid NOT NULL,
    student_id uuid NOT NULL,
    session_id uuid NOT NULL,
    class_id uuid NOT NULL,
    section_id uuid NOT NULL,
    total_marks numeric(9,2) NOT NULL CHECK (total_marks >= 0),
    full_marks numeric(9,2) NOT NULL CHECK (full_marks > 0),
    gpa numeric(3,2) NOT NULL CHECK (gpa BETWEEN 0 AND 5),
    passed boolean NOT NULL,
    class_position integer NOT NULL CHECK (class_position > 0),
    section_position integer NOT NULL CHECK (section_position > 0),
    calculated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (exam_id,student_id),
    CONSTRAINT exam_results_exam_fk FOREIGN KEY (exam_id,session_id,class_id,section_id) REFERENCES exams(id,session_id,class_id,section_id) ON DELETE CASCADE,
    CONSTRAINT exam_results_student_fk FOREIGN KEY (student_id,session_id,class_id,section_id) REFERENCES students(id,session_id,class_id,section_id) ON DELETE RESTRICT
);
CREATE INDEX exam_results_student_idx ON exam_results(student_id,exam_id);

INSERT INTO permissions(id,description) VALUES
 ('exams:manage','Manage exams, subjects and publication'),
 ('exams:read','Read exams, marks, tabulation and results for school oversight'),
 ('exams:enter_marks','Enter marks for assigned teaching subjects'),
 ('exams:read_self','Read own published results'),
 ('exams:read_children','Read linked children published results')
ON CONFLICT(id) DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id) VALUES
 ('admin','exams:manage'),('admin','exams:read'),('admin','exams:enter_marks'),
 ('head_teacher','exams:read'),('teacher','exams:enter_marks'),
 ('student','exams:read_self'),('guardian','exams:read_children')
ON CONFLICT DO NOTHING;
