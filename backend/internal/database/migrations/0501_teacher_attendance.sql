CREATE TABLE teacher_attendance (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    attendance_date date NOT NULL,
    status text NOT NULL CHECK (status IN ('present', 'absent', 'leave')),
    note text NOT NULL DEFAULT '' CHECK (octet_length(note) <= 500),
    recorded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT teacher_attendance_unique_day UNIQUE (session_id, employee_id, attendance_date)
);
CREATE INDEX teacher_attendance_day_idx ON teacher_attendance (session_id, attendance_date DESC, employee_id);
CREATE INDEX teacher_attendance_employee_idx ON teacher_attendance (employee_id, attendance_date DESC, id);

INSERT INTO permissions (id, description) VALUES
    ('attendance:teachers_read', 'Read teacher attendance and daily roster'),
    ('attendance:teachers_manage', 'Record and correct teacher attendance'),
    ('attendance:teachers_read_self', 'Read own teacher attendance'),
    ('attendance:teachers_reports', 'Read teacher monthly attendance reports')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id LIKE 'attendance:teachers_%'
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('head_teacher', 'attendance:teachers_read'),
    ('head_teacher', 'attendance:teachers_manage'),
    ('head_teacher', 'attendance:teachers_read_self'),
    ('head_teacher', 'attendance:teachers_reports'),
    ('teacher', 'attendance:teachers_read_self')
ON CONFLICT DO NOTHING;
