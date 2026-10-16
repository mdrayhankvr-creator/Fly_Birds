CREATE TABLE student_attendance (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    attendance_date date NOT NULL,
    status text NOT NULL CHECK (status IN ('present', 'absent', 'late')),
    source text NOT NULL CHECK (source IN ('manual', 'qr')),
    note text NOT NULL DEFAULT '' CHECK (octet_length(note) <= 500),
    recorded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT student_attendance_section_context_fk FOREIGN KEY (section_id, class_id, session_id) REFERENCES academic_sections(id, class_id, session_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT student_attendance_unique_day UNIQUE (student_id, session_id, attendance_date)
);
CREATE INDEX student_attendance_roster_idx ON student_attendance (session_id, section_id, attendance_date DESC, student_id);
CREATE INDEX student_attendance_student_idx ON student_attendance (student_id, attendance_date DESC);

CREATE TABLE attendance_absence_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attendance_id uuid NOT NULL UNIQUE REFERENCES student_attendance(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    attendance_date date NOT NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT attendance_absence_unique_day UNIQUE (student_id, session_id, attendance_date)
);
CREATE INDEX attendance_absence_pending_idx ON attendance_absence_events (attendance_date DESC, id) WHERE state='pending';

INSERT INTO permissions (id, description) VALUES
    ('attendance:students_read', 'Read student attendance roster and reports'),
    ('attendance:students_manage', 'Record and correct student attendance'),
    ('attendance:students_read_self', 'Read own student attendance'),
    ('attendance:guardians_read_children', 'Read linked children attendance'),
    ('attendance:alerts_read', 'Read absence alert triggers')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id IN ('attendance:students_read','attendance:students_manage','attendance:alerts_read')
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('head_teacher','attendance:students_read'),
    ('head_teacher','attendance:students_manage'),
    ('teacher','attendance:students_read'),
    ('teacher','attendance:students_manage'),
    ('student','attendance:students_read_self'),
    ('guardian','attendance:guardians_read_children')
ON CONFLICT DO NOTHING;
