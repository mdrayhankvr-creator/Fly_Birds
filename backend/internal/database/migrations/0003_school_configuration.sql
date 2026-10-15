CREATE TABLE grading_schemes (
    id text PRIMARY KEY,
    name text NOT NULL
);
INSERT INTO grading_schemes (id, name) VALUES ('bd_default', 'Bangladesh national grading') ON CONFLICT (id) DO NOTHING;

CREATE TABLE school_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    name_en text NOT NULL,
    name_bn text NOT NULL,
    logo_url text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    email text NOT NULL DEFAULT '',
    preferred_language text NOT NULL CHECK (preferred_language IN ('en', 'bn')),
    primary_color text NOT NULL,
    secondary_color text NOT NULL,
    dashboard_theme text NOT NULL CHECK (dashboard_theme IN ('light', 'dark')),
    grading_scheme_id text NOT NULL REFERENCES grading_schemes(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO school_settings (id, name_en, name_bn, preferred_language, primary_color, secondary_color, dashboard_theme, grading_scheme_id)
VALUES (1, 'School', 'বিদ্যালয়', 'en', '#0F766E', '#334155', 'light', 'bd_default')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE grading_bands (
    scheme_id text NOT NULL REFERENCES grading_schemes(id) ON DELETE CASCADE,
    grade text NOT NULL,
    min_marks integer NOT NULL CHECK (min_marks BETWEEN 0 AND 100),
    max_marks integer NOT NULL CHECK (max_marks BETWEEN 0 AND 100 AND max_marks >= min_marks),
    gpa numeric(3,2) NOT NULL CHECK (gpa BETWEEN 0 AND 5),
    PRIMARY KEY (scheme_id, grade)
);
INSERT INTO grading_bands (scheme_id, grade, min_marks, max_marks, gpa) VALUES
    ('bd_default', 'A+', 80, 100, 5.00),
    ('bd_default', 'A', 70, 79, 4.00),
    ('bd_default', 'A-', 60, 69, 3.50),
    ('bd_default', 'B', 50, 59, 3.00),
    ('bd_default', 'C', 40, 49, 2.00),
    ('bd_default', 'D', 33, 39, 1.00),
    ('bd_default', 'F', 0, 32, 0.00)
ON CONFLICT (scheme_id, grade) DO NOTHING;

CREATE TABLE module_definitions (
    key text PRIMARY KEY,
    name text NOT NULL,
    is_core boolean NOT NULL DEFAULT false
);
INSERT INTO module_definitions (key, name, is_core) VALUES
    ('authentication', 'Authentication', true),
    ('permissions', 'Permissions', true),
    ('school_settings', 'School settings', true),
    ('academics', 'Academic structure', false),
    ('students', 'Students and guardians', false),
    ('staff', 'Teachers and staff', false),
    ('attendance', 'Attendance', false),
    ('routine', 'Routines', false),
    ('examinations', 'Examinations and results', false),
    ('fees', 'Fees and accounts', false),
    ('notices', 'Notices and notifications', false),
    ('analytics', 'Analytics', false)
ON CONFLICT (key) DO NOTHING;

CREATE TABLE school_modules (
    school_id smallint NOT NULL REFERENCES school_settings(id) ON DELETE CASCADE,
    module_key text NOT NULL REFERENCES module_definitions(key) ON DELETE RESTRICT,
    enabled boolean NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (school_id, module_key)
);
INSERT INTO school_modules (school_id, module_key, enabled)
SELECT 1, key, is_core FROM module_definitions ON CONFLICT (school_id, module_key) DO NOTHING;

INSERT INTO permissions (id, description) VALUES
    ('grading:read', 'Read school grading bands'),
    ('grading:update', 'Update school grading bands'),
    ('modules:read', 'Read module availability'),
    ('modules:manage', 'Enable and disable optional modules')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id IN ('grading:read', 'grading:update', 'modules:read', 'modules:manage')
ON CONFLICT DO NOTHING;
