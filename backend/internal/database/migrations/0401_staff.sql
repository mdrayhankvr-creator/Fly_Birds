CREATE TABLE employees (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_code text NOT NULL,
    user_id uuid UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('teacher', 'non_teaching')),
    name_en text NOT NULL,
    name_bn text NOT NULL DEFAULT '',
    designation text NOT NULL,
    phone text NOT NULL DEFAULT '',
    joined_on date NOT NULL,
    left_on date,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT employee_dates_ck CHECK (left_on IS NULL OR left_on >= joined_on),
    CONSTRAINT employee_code_ck CHECK (employee_code ~ '^[A-Z0-9][A-Z0-9_-]{1,31}$'),
    CONSTRAINT employee_name_ck CHECK (name_en = btrim(name_en) AND name_en <> '' AND octet_length(name_en) <= 160 AND octet_length(name_bn) <= 160),
    CONSTRAINT employee_designation_ck CHECK (designation = btrim(designation) AND designation <> '' AND octet_length(designation) <= 120),
    CONSTRAINT employee_phone_ck CHECK (octet_length(phone) <= 32)
);
CREATE UNIQUE INDEX employees_code_key ON employees (upper(employee_code));
CREATE INDEX employees_directory_idx ON employees (kind, is_active, name_en, id);

CREATE TABLE teacher_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT teacher_assignments_mapping_fk FOREIGN KEY (class_id, subject_id) REFERENCES academic_class_subjects(class_id, subject_id) ON DELETE RESTRICT,
    CONSTRAINT teacher_assignments_key UNIQUE (employee_id, session_id, class_id, subject_id)
);
CREATE INDEX teacher_assignments_class_idx ON teacher_assignments (session_id, class_id, subject_id);

CREATE TABLE employee_leaves (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    starts_on date NOT NULL,
    ends_on date NOT NULL,
    leave_type text NOT NULL CHECK (leave_type IN ('casual', 'sick', 'annual', 'other')),
    reason text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    reviewed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT employee_leave_dates_ck CHECK (ends_on >= starts_on),
    CONSTRAINT employee_leave_reason_ck CHECK (octet_length(reason) <= 1000),
    CONSTRAINT employee_leave_review_ck CHECK ((reviewed_by IS NULL AND reviewed_at IS NULL) OR (reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL))
);
CREATE INDEX employee_leaves_employee_idx ON employee_leaves (employee_id, starts_on DESC, id);

CREATE TABLE employee_salary_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    effective_on date NOT NULL,
    basic_bdt numeric(12,2) NOT NULL CHECK (basic_bdt >= 0),
    allowance_bdt numeric(12,2) NOT NULL DEFAULT 0 CHECK (allowance_bdt >= 0),
    deduction_bdt numeric(12,2) NOT NULL DEFAULT 0 CHECK (deduction_bdt >= 0),
    note text NOT NULL DEFAULT '' CHECK (octet_length(note) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT employee_salary_net_ck CHECK (deduction_bdt <= basic_bdt + allowance_bdt),
    CONSTRAINT employee_salary_effective_key UNIQUE (employee_id, effective_on)
);
CREATE INDEX employee_salary_employee_idx ON employee_salary_records (employee_id, effective_on DESC);

INSERT INTO permissions (id, description) VALUES
    ('staff:read', 'Read employee directory and profiles'),
    ('staff:manage', 'Manage employee profiles and assignments'),
    ('staff:read_self', 'Read own employee profile and assignments'),
    ('staff_leaves:read', 'Read employee leave records'),
    ('staff_leaves:manage', 'Manage leave decisions'),
    ('staff_leaves:request_self', 'Request and read own leave'),
    ('staff_salary:read', 'Read private salary records'),
    ('staff_salary:manage', 'Manage private salary records')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id LIKE 'staff:%' OR id LIKE 'staff_leaves:%' OR id LIKE 'staff_salary:%'
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('head_teacher', 'staff:read'),
    ('head_teacher', 'staff:read_self'),
    ('head_teacher', 'staff_leaves:request_self'),
    ('head_teacher', 'staff_leaves:read'),
    ('teacher', 'staff:read_self'),
    ('teacher', 'staff_leaves:request_self')
ON CONFLICT DO NOTHING;
