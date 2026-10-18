-- M11 analytics reads the existing attendance, examination, fee, student and
-- staff tables; it adds no copy of them. This migration adds the rule
-- thresholds for at-risk and dropout alerts, role-scoped read permissions and
-- two indexes for school-wide aggregation.

-- Every analytics view starts from the student register.
UPDATE module_definitions SET depends_on = ARRAY['students'] WHERE key = 'analytics';

-- One row of school-configurable alert thresholds. The rules are explicit and
-- explainable; nothing is predicted.
CREATE TABLE analytics_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    attendance_threshold numeric(5,2) NOT NULL DEFAULT 75 CHECK (attendance_threshold BETWEEN 1 AND 100),
    attendance_window_days smallint NOT NULL DEFAULT 30 CHECK (attendance_window_days BETWEEN 7 AND 120),
    min_marked_days smallint NOT NULL DEFAULT 5 CHECK (min_marked_days BETWEEN 1 AND 60),
    gpa_threshold numeric(3,2) NOT NULL DEFAULT 2.00 CHECK (gpa_threshold BETWEEN 0 AND 5),
    consecutive_absences smallint NOT NULL DEFAULT 5 CHECK (consecutive_absences BETWEEN 2 AND 60),
    inactivity_days smallint NOT NULL DEFAULT 21 CHECK (inactivity_days BETWEEN 7 AND 120),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT analytics_settings_window_ck CHECK (min_marked_days <= attendance_window_days)
);
INSERT INTO analytics_settings (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

CREATE INDEX IF NOT EXISTS student_attendance_session_date_idx ON student_attendance (session_id, attendance_date);
CREATE INDEX IF NOT EXISTS exam_results_context_idx ON exam_results (session_id, class_id, section_id);

INSERT INTO permissions (id, description) VALUES
    ('analytics:read', 'Read school-wide attendance, examination and alert analytics and reports'),
    ('analytics:read_assigned', 'Read analytics for assigned classes and subjects'),
    ('analytics:finance', 'Read fee collection, dues, income and expense analytics'),
    ('analytics:read_self', 'Read own academic summary'),
    ('analytics:read_children', 'Read linked children''s academic summaries'),
    ('analytics:manage', 'Change at-risk and dropout alert thresholds')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'analytics:read'), ('admin', 'analytics:finance'), ('admin', 'analytics:manage'),
    ('head_teacher', 'analytics:read'),
    ('teacher', 'analytics:read_assigned'),
    ('accountant', 'analytics:finance'),
    ('student', 'analytics:read_self'),
    ('guardian', 'analytics:read_children')
ON CONFLICT DO NOTHING;
