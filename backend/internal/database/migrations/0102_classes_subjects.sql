CREATE TABLE academic_classes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    display_order integer NOT NULL DEFAULT 0,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_classes_name_ck CHECK (name <> '' AND name = btrim(name) AND octet_length(name) <= 120),
    CONSTRAINT academic_classes_order_ck CHECK (display_order BETWEEN 0 AND 100000)
);
CREATE UNIQUE INDEX academic_classes_name_key ON academic_classes (lower(name));
CREATE INDEX academic_classes_order_idx ON academic_classes (display_order, name, id);
CREATE INDEX academic_classes_active_idx ON academic_classes (is_active, display_order, id);

CREATE TABLE academic_subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name_en text NOT NULL,
    name_bn text NOT NULL,
    code text NOT NULL,
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_subjects_name_en_ck CHECK (name_en <> '' AND name_en = btrim(name_en) AND octet_length(name_en) <= 160),
    CONSTRAINT academic_subjects_name_bn_ck CHECK (name_bn <> '' AND name_bn = btrim(name_bn) AND octet_length(name_bn) <= 160),
    CONSTRAINT academic_subjects_code_ck CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{0,31}$')
);
CREATE UNIQUE INDEX academic_subjects_code_key ON academic_subjects (upper(code));
CREATE INDEX academic_subjects_active_idx ON academic_subjects (is_active, code, id);

CREATE TABLE academic_class_subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES academic_subjects(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    is_required boolean NOT NULL,
    is_fourth_subject boolean NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT academic_class_subjects_designation_ck CHECK (is_required <> is_fourth_subject),
    CONSTRAINT academic_class_subjects_pair_key UNIQUE (class_id, subject_id)
);
CREATE INDEX academic_class_subjects_subject_idx ON academic_class_subjects (subject_id);

INSERT INTO module_definitions (key, name, is_core)
VALUES ('academics', 'Academic structure', false) ON CONFLICT (key) DO NOTHING;
INSERT INTO school_modules (school_id, module_key, enabled)
SELECT id, 'academics', false FROM school_settings ON CONFLICT (school_id, module_key) DO NOTHING;
INSERT INTO permissions (id, description) VALUES
    ('academics:read', 'Read classes, subjects, and class-subject assignments'),
    ('academics:manage', 'Manage classes, subjects, and class-subject assignments')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id IN ('academics:read', 'academics:manage')
ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
VALUES ('head_teacher', 'academics:read') ON CONFLICT DO NOTHING;
