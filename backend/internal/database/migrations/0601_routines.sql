CREATE TABLE class_routines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    teacher_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    day_of_week integer NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_minute integer NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute integer NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
    room text NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT class_routine_time_ck CHECK (end_minute > start_minute),
    CONSTRAINT class_routine_room_ck CHECK (room = btrim(room) AND room <> '' AND octet_length(room) <= 80),
    CONSTRAINT class_routine_section_fk FOREIGN KEY (section_id, class_id, session_id) REFERENCES academic_sections(id, class_id, session_id) ON DELETE RESTRICT,
    CONSTRAINT class_routine_subject_fk FOREIGN KEY (class_id, subject_id) REFERENCES academic_class_subjects(class_id, subject_id) ON DELETE RESTRICT,
    CONSTRAINT class_routine_teacher_assignment_fk FOREIGN KEY (teacher_id, session_id, class_id, subject_id) REFERENCES teacher_assignments(employee_id, session_id, class_id, subject_id) ON DELETE RESTRICT
);
CREATE INDEX class_routines_section_idx ON class_routines (session_id, section_id, day_of_week, start_minute);
CREATE INDEX class_routines_teacher_idx ON class_routines (teacher_id, session_id, day_of_week, start_minute);
CREATE INDEX class_routines_room_idx ON class_routines (session_id, day_of_week, lower(room), start_minute);

CREATE TABLE exam_routines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    is_published boolean NOT NULL DEFAULT false,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT exam_routine_name_ck CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 120),
    CONSTRAINT exam_routine_section_fk FOREIGN KEY (section_id, class_id, session_id) REFERENCES academic_sections(id, class_id, session_id) ON DELETE RESTRICT,
    CONSTRAINT exam_routine_id_class_key UNIQUE (id, class_id)
);
CREATE UNIQUE INDEX exam_routines_name_key ON exam_routines (session_id, section_id, lower(name));
CREATE INDEX exam_routines_lookup_idx ON exam_routines (session_id, class_id, section_id, is_published, name);

CREATE TABLE exam_routine_slots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    routine_id uuid NOT NULL,
    class_id uuid NOT NULL,
    subject_id uuid NOT NULL REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    exam_date date NOT NULL,
    start_minute integer NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute integer NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
    room text NOT NULL,
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT exam_routine_slot_time_ck CHECK (end_minute > start_minute),
    CONSTRAINT exam_routine_slot_room_ck CHECK (room = btrim(room) AND room <> '' AND octet_length(room) <= 80),
    CONSTRAINT exam_routine_slot_subject_key UNIQUE (routine_id, subject_id),
    CONSTRAINT exam_routine_slot_parent_fk FOREIGN KEY (routine_id, class_id) REFERENCES exam_routines(id, class_id) ON DELETE CASCADE,
    CONSTRAINT exam_routine_slot_mapping_fk FOREIGN KEY (class_id, subject_id) REFERENCES academic_class_subjects(class_id, subject_id) ON DELETE RESTRICT
);
CREATE INDEX exam_routine_slots_date_idx ON exam_routine_slots (exam_date, start_minute, room);

INSERT INTO module_definitions (key, name, is_core) VALUES ('routine', 'Class and exam routines', false) ON CONFLICT (key) DO NOTHING;
INSERT INTO school_modules (school_id, module_key, enabled)
SELECT id, 'routine', false FROM school_settings ON CONFLICT (school_id, module_key) DO NOTHING;
INSERT INTO permissions (id, description) VALUES
    ('routine:manage', 'Manage class and exam routines'),
    ('routine:read', 'Read class and exam routines for school oversight'),
    ('routine:read_self', 'Read own class or teaching routines'),
    ('routine:read_children', 'Read linked children published routines')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'routine:manage'), ('admin', 'routine:read'),
    ('head_teacher', 'routine:read'),
    ('teacher', 'routine:read_self'),
    ('student', 'routine:read_self'),
    ('guardian', 'routine:read_children')
ON CONFLICT DO NOTHING;
