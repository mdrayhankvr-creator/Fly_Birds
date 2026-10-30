-- Checkpoint 4: staff profiles with privacy the teacher controls.
--
-- Contact email, qualifications and, for phone, email and qualifications,
-- whether other staff and students/families may see them (the teacher
-- chooses; administrators and the head teacher always can).
ALTER TABLE employees
    ADD COLUMN email text NOT NULL DEFAULT '',
    ADD COLUMN qualifications jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN phone_to_staff boolean NOT NULL DEFAULT false,
    ADD COLUMN phone_to_families boolean NOT NULL DEFAULT false,
    ADD COLUMN email_to_staff boolean NOT NULL DEFAULT false,
    ADD COLUMN email_to_families boolean NOT NULL DEFAULT false,
    ADD COLUMN qualifications_to_staff boolean NOT NULL DEFAULT false,
    ADD COLUMN qualifications_to_families boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT employee_email_ck CHECK (email = btrim(email) AND octet_length(email) <= 254),
    ADD CONSTRAINT employee_qualifications_ck CHECK (jsonb_typeof(qualifications) = 'array' AND jsonb_array_length(qualifications) <= 20);

-- Official identity numbers live apart from employees, so no query of the
-- employee directory, its exports or its reports can pick them up. Only the
-- profile endpoint reads them, for administrators (and the head teacher for
-- the MPO/index number) and the employee; the NID only masked unless an
-- audited reveal asks for it.
CREATE TABLE employee_identity (
    employee_id uuid PRIMARY KEY REFERENCES employees(id) ON DELETE CASCADE,
    mpo_index text NOT NULL DEFAULT '',
    nid text NOT NULL DEFAULT '',
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT employee_identity_mpo_ck CHECK (mpo_index = btrim(mpo_index) AND octet_length(mpo_index) <= 40),
    -- Bangladesh NIDs have 10, 13 or 17 digits.
    CONSTRAINT employee_identity_nid_ck CHECK (nid ~ '^([0-9]{10}|[0-9]{13}|[0-9]{17})?$')
);

-- One photo per employee, stored resized and re-encoded as JPEG.
CREATE TABLE employee_photos (
    employee_id uuid PRIMARY KEY REFERENCES employees(id) ON DELETE CASCADE,
    content bytea NOT NULL CHECK (octet_length(content) BETWEEN 1 AND 524288),
    version text NOT NULL,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Every signed-in role may open a staff profile; the server decides which
-- fields each viewer sees.
INSERT INTO permissions (id, description) VALUES
    ('staff:profiles_read', 'Open staff profiles (fields filtered for the viewer)')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id)
SELECT r, 'staff:profiles_read' FROM unnest(ARRAY['admin','head_teacher','teacher','accountant','student','guardian']) AS r
ON CONFLICT DO NOTHING;
