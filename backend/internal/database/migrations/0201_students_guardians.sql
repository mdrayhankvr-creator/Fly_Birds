ALTER TABLE users ADD CONSTRAINT users_id_role_key UNIQUE (id, role);

CREATE TABLE students (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    admission_no text NOT NULL,
    name_en text NOT NULL,
    name_bn text NOT NULL DEFAULT '',
    date_of_birth date,
    status text NOT NULL DEFAULT 'active',
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    roll_no integer,
    user_id uuid UNIQUE,
    user_role text GENERATED ALWAYS AS ('student') STORED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT students_section_context_fk FOREIGN KEY (section_id, class_id, session_id) REFERENCES academic_sections(id, class_id, session_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT students_user_role_fk FOREIGN KEY (user_id, user_role) REFERENCES users(id, role) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT students_admission_ck CHECK (admission_no <> '' AND admission_no = btrim(admission_no) AND octet_length(admission_no) <= 64),
    CONSTRAINT students_name_en_ck CHECK (name_en <> '' AND name_en = btrim(name_en) AND octet_length(name_en) <= 160),
    CONSTRAINT students_name_bn_ck CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 160),
    CONSTRAINT students_status_ck CHECK (status IN ('active', 'inactive', 'graduated', 'transferred')),
    CONSTRAINT students_roll_ck CHECK (roll_no IS NULL OR roll_no BETWEEN 1 AND 999999)
);
CREATE UNIQUE INDEX students_admission_key ON students (lower(admission_no));
CREATE UNIQUE INDEX students_section_roll_key ON students (section_id, roll_no) WHERE roll_no IS NOT NULL;
CREATE INDEX students_directory_idx ON students (session_id, class_id, section_id, status, admission_no, id);
CREATE INDEX students_name_idx ON students (lower(name_en));

CREATE TABLE student_photos (
    student_id uuid PRIMARY KEY REFERENCES students(id) ON DELETE CASCADE,
    content_type text NOT NULL CHECK (content_type IN ('image/jpeg', 'image/png')),
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 2097152),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE guardians (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name_en text NOT NULL,
    name_bn text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    email text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    user_id uuid UNIQUE,
    user_role text GENERATED ALWAYS AS ('guardian') STORED,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT guardians_user_role_fk FOREIGN KEY (user_id, user_role) REFERENCES users(id, role) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT guardians_name_en_ck CHECK (name_en <> '' AND name_en = btrim(name_en) AND octet_length(name_en) <= 160),
    CONSTRAINT guardians_name_bn_ck CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 160),
    CONSTRAINT guardians_phone_ck CHECK (phone = btrim(phone) AND octet_length(phone) <= 32),
    CONSTRAINT guardians_email_ck CHECK (email = btrim(email) AND octet_length(email) <= 254),
    CONSTRAINT guardians_address_ck CHECK (address = btrim(address) AND octet_length(address) <= 500)
);
CREATE INDEX guardians_name_idx ON guardians (lower(name_en), id);
CREATE INDEX guardians_phone_idx ON guardians (phone) WHERE phone <> '';

CREATE TABLE student_guardians (
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    guardian_id uuid NOT NULL REFERENCES guardians(id) ON DELETE CASCADE,
    relationship text NOT NULL,
    is_primary boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, guardian_id),
    CONSTRAINT student_guardians_relationship_ck CHECK (relationship <> '' AND relationship = btrim(relationship) AND octet_length(relationship) <= 60)
);
CREATE UNIQUE INDEX student_guardians_primary_key ON student_guardians (student_id) WHERE is_primary;
CREATE INDEX student_guardians_guardian_idx ON student_guardians (guardian_id, student_id);

INSERT INTO permissions (id, description) VALUES
    ('students:read', 'Read student directory and profiles'),
    ('students:manage', 'Manage students, photos, and imports'),
    ('students:read_self', 'Read own linked student profile'),
    ('guardians:read', 'Read guardian directory and relationships'),
    ('guardians:manage', 'Manage guardians and student relationships'),
    ('guardians:read_children', 'Read linked children as a guardian')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT 'admin', id FROM permissions WHERE id IN ('students:read','students:manage','guardians:read','guardians:manage') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
VALUES ('head_teacher','students:read'),('head_teacher','guardians:read'),('student','students:read_self'),('guardian','guardians:read_children') ON CONFLICT DO NOTHING;
