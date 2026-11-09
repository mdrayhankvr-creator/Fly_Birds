-- E8: term exam routine planner. One routine per exam term, kept as
-- versions: draft → submitted → approved (head teacher) → published (admin)
-- → superseded by an amendment. A version that has left draft never
-- changes; an amendment is a new version. The per-section exam routines of
-- migration 0601 stay as they are, with their endpoints.

CREATE TABLE exam_rooms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 80),
    name_bn text NOT NULL DEFAULT '' CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 240),
    kind text NOT NULL DEFAULT 'classroom' CHECK (kind IN ('classroom', 'lab', 'hall')),
    capacity integer CHECK (capacity IS NULL OR capacity BETWEEN 1 AND 5000),
    is_active boolean NOT NULL DEFAULT true,
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX exam_rooms_name_key ON exam_rooms (lower(name));

CREATE TABLE routine_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    term_id uuid NOT NULL REFERENCES exam_terms(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version >= 1),
    based_on uuid REFERENCES routine_versions(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'submitted', 'approved', 'published', 'superseded')),
    reason text CHECK (reason IS NULL OR (reason = btrim(reason) AND reason <> '' AND octet_length(reason) <= 1000)),
    starts_on date NOT NULL,
    ends_on date NOT NULL,
    instructions text NOT NULL DEFAULT '' CHECK (octet_length(instructions) <= 4000),
    returned_reason text CHECK (returned_reason IS NULL OR octet_length(returned_reason) <= 1000),
    content_sha256 text CHECK (content_sha256 IS NULL OR content_sha256 ~ '^[0-9a-f]{64}$'),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    submitted_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    submitted_at timestamptz,
    approved_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    approved_at timestamptz,
    published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at timestamptz,
    superseded_at timestamptz,
    CONSTRAINT routine_versions_number_key UNIQUE (term_id, version),
    CONSTRAINT routine_versions_dates_ck CHECK (ends_on >= starts_on AND ends_on - starts_on <= 120),
    -- An amendment names the version it amends and says why.
    CONSTRAINT routine_versions_amendment_ck CHECK ((version = 1) = (based_on IS NULL) AND (version = 1 OR reason IS NOT NULL)),
    CONSTRAINT routine_versions_published_ck CHECK (status NOT IN ('published', 'superseded') OR (published_at IS NOT NULL AND content_sha256 IS NOT NULL))
);
-- At most one version of a term in progress, and one published.
CREATE UNIQUE INDEX routine_versions_open_key ON routine_versions (term_id) WHERE status IN ('draft', 'submitted', 'approved');
CREATE UNIQUE INDEX routine_versions_published_key ON routine_versions (term_id) WHERE status = 'published';
CREATE INDEX routine_versions_status_idx ON routine_versions (status, term_id);

CREATE TABLE routine_sittings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES routine_versions(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 60),
    name_bn text NOT NULL DEFAULT '' CHECK (name_bn = btrim(name_bn) AND octet_length(name_bn) <= 180),
    start_minute integer NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute integer NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
    CONSTRAINT routine_sittings_time_ck CHECK (end_minute > start_minute),
    CONSTRAINT routine_sittings_version_key UNIQUE (id, version_id)
);
CREATE UNIQUE INDEX routine_sittings_name_key ON routine_sittings (version_id, lower(name));

CREATE TABLE routine_papers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES routine_versions(id) ON DELETE CASCADE,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    paper_kind text NOT NULL DEFAULT 'theory' CHECK (paper_kind IN ('theory', 'practical', 'viva', 'oral')),
    paper_no smallint NOT NULL DEFAULT 1 CHECK (paper_no BETWEEN 1 AND 3),
    duration_minutes integer NOT NULL CHECK (duration_minutes BETWEEN 15 AND 360),
    needs_lab boolean NOT NULL DEFAULT false,
    exam_date date,
    sitting_id uuid,
    pinned boolean NOT NULL DEFAULT false,
    CONSTRAINT routine_papers_mapping_fk FOREIGN KEY (class_id, subject_id) REFERENCES academic_class_subjects(class_id, subject_id) ON DELETE RESTRICT,
    CONSTRAINT routine_papers_sitting_fk FOREIGN KEY (sitting_id, version_id) REFERENCES routine_sittings(id, version_id) ON DELETE RESTRICT,
    CONSTRAINT routine_papers_placed_ck CHECK ((exam_date IS NULL) = (sitting_id IS NULL) AND (NOT pinned OR exam_date IS NOT NULL)),
    CONSTRAINT routine_papers_key UNIQUE (version_id, class_id, subject_id, paper_kind, paper_no)
);
CREATE INDEX routine_papers_cell_idx ON routine_papers (version_id, exam_date, sitting_id);

-- The room each section of a class sits a paper in.
CREATE TABLE routine_paper_rooms (
    paper_id uuid NOT NULL REFERENCES routine_papers(id) ON DELETE CASCADE,
    section_id uuid NOT NULL REFERENCES academic_sections(id) ON DELETE RESTRICT,
    room_id uuid NOT NULL REFERENCES exam_rooms(id) ON DELETE RESTRICT,
    PRIMARY KEY (paper_id, section_id)
);
CREATE INDEX routine_paper_rooms_room_idx ON routine_paper_rooms (room_id);

-- Invigilation: one member of staff in one room for one sitting. Nobody is
-- in two rooms at once.
CREATE TABLE routine_duties (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES routine_versions(id) ON DELETE CASCADE,
    exam_date date NOT NULL,
    sitting_id uuid NOT NULL,
    room_id uuid NOT NULL REFERENCES exam_rooms(id) ON DELETE RESTRICT,
    employee_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    CONSTRAINT routine_duties_sitting_fk FOREIGN KEY (sitting_id, version_id) REFERENCES routine_sittings(id, version_id) ON DELETE RESTRICT,
    CONSTRAINT routine_duties_person_key UNIQUE (version_id, exam_date, sitting_id, employee_id)
);
CREATE INDEX routine_duties_employee_idx ON routine_duties (employee_id, exam_date);

-- A version that has left draft never changes: its content is frozen, and
-- only its status moves forward.
CREATE FUNCTION routine_version_is_draft(p_version uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce((SELECT status = 'draft' FROM routine_versions WHERE id = p_version), true)
$$;

CREATE FUNCTION routine_content_frozen() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    v uuid;
BEGIN
    IF TG_TABLE_NAME = 'routine_paper_rooms' THEN
        SELECT version_id INTO v FROM routine_papers WHERE id = coalesce(NEW.paper_id, OLD.paper_id);
    ELSIF TG_OP = 'DELETE' THEN
        v := OLD.version_id;
    ELSE
        v := NEW.version_id;
    END IF;
    IF NOT routine_version_is_draft(v) THEN
        RAISE EXCEPTION 'a routine version that has left draft cannot change' USING ERRCODE = '23514';
    END IF;
    RETURN coalesce(NEW, OLD);
END $$;
CREATE TRIGGER routine_sittings_frozen BEFORE INSERT OR UPDATE OR DELETE ON routine_sittings FOR EACH ROW EXECUTE FUNCTION routine_content_frozen();
CREATE TRIGGER routine_papers_frozen BEFORE INSERT OR UPDATE OR DELETE ON routine_papers FOR EACH ROW EXECUTE FUNCTION routine_content_frozen();
CREATE TRIGGER routine_paper_rooms_frozen BEFORE INSERT OR UPDATE OR DELETE ON routine_paper_rooms FOR EACH ROW EXECUTE FUNCTION routine_content_frozen();
CREATE TRIGGER routine_duties_frozen BEFORE INSERT OR UPDATE OR DELETE ON routine_duties FOR EACH ROW EXECUTE FUNCTION routine_content_frozen();

CREATE FUNCTION routine_versions_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'draft' THEN
            RAISE EXCEPTION 'only a draft routine version can be deleted' USING ERRCODE = '23514';
        END IF;
        RETURN OLD;
    END IF;
    IF NEW.term_id IS DISTINCT FROM OLD.term_id OR NEW.version IS DISTINCT FROM OLD.version OR NEW.based_on IS DISTINCT FROM OLD.based_on THEN
        RAISE EXCEPTION 'a routine version keeps its term and number' USING ERRCODE = '23514';
    END IF;
    IF OLD.status <> 'draft' AND (NEW.starts_on IS DISTINCT FROM OLD.starts_on OR NEW.ends_on IS DISTINCT FROM OLD.ends_on
        OR NEW.instructions IS DISTINCT FROM OLD.instructions OR NEW.reason IS DISTINCT FROM OLD.reason) THEN
        RAISE EXCEPTION 'a routine version that has left draft cannot change' USING ERRCODE = '23514';
    END IF;
    IF OLD.status IN ('published', 'superseded') AND NEW.content_sha256 IS DISTINCT FROM OLD.content_sha256 THEN
        RAISE EXCEPTION 'a published routine keeps its content hash' USING ERRCODE = '23514';
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status AND NOT (
        (OLD.status = 'draft' AND NEW.status = 'submitted') OR
        (OLD.status = 'submitted' AND NEW.status IN ('draft', 'approved')) OR
        (OLD.status = 'approved' AND NEW.status IN ('draft', 'published')) OR
        (OLD.status = 'published' AND NEW.status = 'superseded')) THEN
        RAISE EXCEPTION 'routine version cannot move from % to %', OLD.status, NEW.status USING ERRCODE = '23514';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END $$;
CREATE TRIGGER routine_versions_guard BEFORE UPDATE OR DELETE ON routine_versions FOR EACH ROW EXECUTE FUNCTION routine_versions_guard();

INSERT INTO permissions (id, description) VALUES
    ('routine:approve', 'Approve or return term exam routines')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('head_teacher', 'routine:approve')
ON CONFLICT DO NOTHING;
