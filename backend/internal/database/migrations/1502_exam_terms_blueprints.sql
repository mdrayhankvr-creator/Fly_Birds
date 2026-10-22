-- E5: exam terms, assessment blueprints (versioned), components and
-- per-student optional subjects. Existing exams are linked to terms and to a
-- legacy blueprint per session and class; no mark, grade or result changes.

-- Terms: named, ordered assessment periods of a session.
CREATE TABLE exam_terms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 120),
    sequence integer NOT NULL CHECK (sequence BETWEEN 1 AND 999),
    kind text NOT NULL DEFAULT 'term' CHECK (kind IN ('term', 'annual')),
    starts_on date,
    ends_on date,
    status text NOT NULL DEFAULT 'planning' CHECK (status IN ('planning', 'running', 'results', 'closed')),
    -- The setup wizard's saved choices while the term is being planned.
    setup jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(setup) = 'object'),
    -- Terms made by the upgrade from existing exams: lower(name)|exam_type.
    legacy_key text,
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT exam_terms_dates_ck CHECK (starts_on IS NULL OR ends_on IS NULL OR ends_on >= starts_on),
    CONSTRAINT exam_terms_sequence_key UNIQUE (session_id, sequence),
    CONSTRAINT exam_terms_id_session_key UNIQUE (id, session_id)
);
CREATE UNIQUE INDEX exam_terms_name_key ON exam_terms (session_id, lower(name));
CREATE UNIQUE INDEX exam_terms_legacy_key ON exam_terms (session_id, legacy_key) WHERE legacy_key IS NOT NULL;

-- Blueprints: one assessment design per session and class.
CREATE TABLE assessment_blueprints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 160),
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assessment_blueprints_class_key UNIQUE (session_id, class_id)
);

-- Versions: a draft is editable; activation validates and freezes it. The
-- normalised policy columns are generated from the definition on activation.
CREATE TABLE blueprint_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    blueprint_id uuid NOT NULL REFERENCES assessment_blueprints(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version >= 1),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'retired')),
    legacy boolean NOT NULL DEFAULT false,
    definition jsonb NOT NULL CHECK (jsonb_typeof(definition) = 'object'),
    definition_sha256 text NOT NULL DEFAULT '',
    grading_scheme_snapshot jsonb,
    gpa_rounding text NOT NULL DEFAULT 'half_up' CHECK (gpa_rounding IN ('half_up', 'truncate')),
    absent_policy text NOT NULL DEFAULT 'zero' CHECK (absent_policy IN ('zero', 'missing')),
    rank_mode text NOT NULL DEFAULT 'competition' CHECK (rank_mode IN ('competition', 'dense')),
    paper_rule text NOT NULL DEFAULT 'combined' CHECK (paper_rule IN ('combined', 'each_paper')),
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    activated_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    activated_at timestamptz,
    CONSTRAINT blueprint_versions_number_key UNIQUE (blueprint_id, version),
    CONSTRAINT blueprint_versions_frozen_ck CHECK (status = 'draft' OR (activated_at IS NOT NULL AND definition_sha256 <> '' AND grading_scheme_snapshot IS NOT NULL))
);
CREATE UNIQUE INDEX blueprint_versions_one_active ON blueprint_versions (blueprint_id) WHERE status = 'active';
CREATE UNIQUE INDEX blueprint_versions_one_draft ON blueprint_versions (blueprint_id) WHERE status = 'draft';

-- An activated version never changes again (only active → retired).
CREATE FUNCTION blueprint_versions_frozen() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.status <> 'draft' THEN
            RAISE EXCEPTION 'an activated blueprint version cannot be deleted' USING ERRCODE = '23514';
        END IF;
        RETURN OLD;
    END IF;
    IF OLD.status <> 'draft' THEN
        IF NEW.definition IS DISTINCT FROM OLD.definition OR NEW.definition_sha256 IS DISTINCT FROM OLD.definition_sha256
           OR NEW.grading_scheme_snapshot IS DISTINCT FROM OLD.grading_scheme_snapshot OR NEW.gpa_rounding IS DISTINCT FROM OLD.gpa_rounding
           OR NEW.absent_policy IS DISTINCT FROM OLD.absent_policy OR NEW.rank_mode IS DISTINCT FROM OLD.rank_mode
           OR NEW.paper_rule IS DISTINCT FROM OLD.paper_rule OR NEW.version IS DISTINCT FROM OLD.version
           OR NEW.blueprint_id IS DISTINCT FROM OLD.blueprint_id OR NEW.legacy IS DISTINCT FROM OLD.legacy
           OR NOT (NEW.status = OLD.status OR (OLD.status = 'active' AND NEW.status = 'retired')) THEN
            RAISE EXCEPTION 'an activated blueprint version cannot change' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER blueprint_versions_frozen_trigger BEFORE UPDATE OR DELETE ON blueprint_versions
FOR EACH ROW EXECUTE FUNCTION blueprint_versions_frozen();

-- Normalised from the definition when a version is activated, for queries.
CREATE TABLE blueprint_assessments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES blueprint_versions(id) ON DELETE CASCADE,
    key text NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9_]{0,15}$'),
    name text NOT NULL CHECK (name = btrim(name) AND name <> '' AND octet_length(name) <= 80),
    term_id uuid NOT NULL REFERENCES exam_terms(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('class_test', 'continuous', 'performance', 'term_exam')),
    sequence integer NOT NULL CHECK (sequence >= 1),
    -- Who among students and guardians sees this assessment's own results.
    -- A display policy, not part of the calculation: it may change after
    -- activation (audited).
    student_visibility text NOT NULL CHECK (student_visibility IN ('hidden', 'after_term_publication', 'with_assessment_publication')),
    CONSTRAINT blueprint_assessments_key UNIQUE (version_id, key),
    CONSTRAINT blueprint_assessments_term_key UNIQUE (version_id, key, term_id)
);

CREATE TABLE blueprint_subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES blueprint_versions(id) ON DELETE CASCADE,
    subject_id uuid NOT NULL REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    role text NOT NULL CHECK (role IN ('required', 'optional', 'non_graded')),
    combined_key text CHECK (combined_key IS NULL OR combined_key ~ '^[A-Z][A-Z0-9_]{0,15}$'),
    display_order integer NOT NULL DEFAULT 0,
    CONSTRAINT blueprint_subjects_key UNIQUE (version_id, subject_id)
);

CREATE TABLE blueprint_components (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    blueprint_subject_id uuid NOT NULL REFERENCES blueprint_subjects(id) ON DELETE CASCADE,
    assessment_key text NOT NULL,
    component_key text NOT NULL CHECK (component_key ~ '^[A-Z][A-Z0-9_]{0,15}$'),
    full_marks numeric(6,2) NOT NULL CHECK (full_marks > 0 AND full_marks <= 1000),
    pass_marks numeric(6,2) CHECK (pass_marks IS NULL OR (pass_marks >= 0 AND pass_marks <= full_marks)),
    display_order integer NOT NULL DEFAULT 0,
    CONSTRAINT blueprint_components_key UNIQUE (blueprint_subject_id, assessment_key, component_key)
);

-- The normalised rows belong to one draft being activated; afterwards only
-- an assessment's student visibility may change.
CREATE FUNCTION blueprint_rows_frozen() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v uuid; st text;
BEGIN
    IF TG_TABLE_NAME = 'blueprint_components' THEN
        SELECT bs.version_id INTO v FROM blueprint_subjects bs WHERE bs.id = coalesce(NEW.blueprint_subject_id, OLD.blueprint_subject_id);
    ELSIF TG_OP = 'DELETE' THEN
        v := OLD.version_id;
    ELSE
        v := NEW.version_id;
    END IF;
    SELECT status INTO st FROM blueprint_versions WHERE id = v;
    IF st IS NULL OR st = 'draft' THEN
        RETURN coalesce(NEW, OLD);
    END IF;
    IF TG_OP = 'UPDATE' AND TG_TABLE_NAME = 'blueprint_assessments'
       AND (to_jsonb(NEW) - 'student_visibility') = (to_jsonb(OLD) - 'student_visibility') THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'an activated blueprint version cannot change' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER blueprint_assessments_frozen BEFORE INSERT OR UPDATE OR DELETE ON blueprint_assessments
FOR EACH ROW EXECUTE FUNCTION blueprint_rows_frozen();
CREATE TRIGGER blueprint_subjects_frozen BEFORE INSERT OR UPDATE OR DELETE ON blueprint_subjects
FOR EACH ROW EXECUTE FUNCTION blueprint_rows_frozen();
CREATE TRIGGER blueprint_components_frozen BEFORE INSERT OR UPDATE OR DELETE ON blueprint_components
FOR EACH ROW EXECUTE FUNCTION blueprint_rows_frozen();

-- Exams (one assessment sitting for one section) link to their term, their
-- assessment and the blueprint version they were created from. Exams created
-- outside a term keep all three empty and work as before.
ALTER TABLE exams
    ADD COLUMN term_id uuid,
    ADD COLUMN assessment_key text,
    ADD COLUMN blueprint_version_id uuid REFERENCES blueprint_versions(id) ON DELETE RESTRICT,
    ADD COLUMN marks_due_on date,
    ADD CONSTRAINT exams_term_fk FOREIGN KEY (term_id, session_id) REFERENCES exam_terms(id, session_id) ON DELETE RESTRICT,
    ADD CONSTRAINT exams_assessment_fk FOREIGN KEY (blueprint_version_id, assessment_key, term_id) REFERENCES blueprint_assessments(version_id, key, term_id) ON DELETE RESTRICT,
    ADD CONSTRAINT exams_term_link_ck CHECK ((term_id IS NULL) = (assessment_key IS NULL)),
    ADD CONSTRAINT exams_blueprint_link_ck CHECK (blueprint_version_id IS NULL OR term_id IS NOT NULL);
CREATE UNIQUE INDEX exams_term_section_key ON exams (term_id, assessment_key, section_id) WHERE term_id IS NOT NULL;
CREATE INDEX exams_term_class_idx ON exams (session_id, class_id, term_id, assessment_key);

-- An exam's blueprint version must be its own session's and class's.
CREATE FUNCTION exams_blueprint_check() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.blueprint_version_id IS NOT NULL AND NOT EXISTS (
        SELECT 1 FROM blueprint_versions v JOIN assessment_blueprints b ON b.id = v.blueprint_id
        WHERE v.id = NEW.blueprint_version_id AND v.status <> 'draft' AND b.session_id = NEW.session_id AND b.class_id = NEW.class_id) THEN
        RAISE EXCEPTION 'the blueprint version does not belong to this exam''s session and class' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER exams_blueprint_trigger BEFORE INSERT OR UPDATE OF blueprint_version_id, session_id, class_id ON exams
FOR EACH ROW EXECUTE FUNCTION exams_blueprint_check();

-- Components of one sheet (CQ, MCQ, Practical …), copied from the blueprint
-- when the sheet is created, like exam_subjects copies full and pass marks.
-- A sheet without rows here has one implicit total, exactly as before.
CREATE TABLE exam_subject_components (
    exam_subject_id uuid NOT NULL REFERENCES exam_subjects(id) ON DELETE CASCADE,
    component_key text NOT NULL CHECK (component_key ~ '^[A-Z][A-Z0-9_]{0,15}$'),
    full_marks numeric(6,2) NOT NULL CHECK (full_marks > 0 AND full_marks <= 1000),
    pass_marks numeric(6,2) CHECK (pass_marks IS NULL OR (pass_marks >= 0 AND pass_marks <= full_marks)),
    display_order integer NOT NULL DEFAULT 0,
    PRIMARY KEY (exam_subject_id, component_key)
);

-- A component sheet's full marks are always the sum of its components, so a
-- later edit of the sheet (the exam page) cannot set them apart.
CREATE FUNCTION exam_subjects_component_full() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE total numeric(7,2);
BEGIN
    SELECT sum(full_marks) INTO total FROM exam_subject_components WHERE exam_subject_id = NEW.id;
    IF total IS NOT NULL AND NEW.full_marks <> total THEN
        RAISE EXCEPTION 'a component sheet''s full marks are the sum of its components' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER exam_subjects_component_full_trigger BEFORE UPDATE OF full_marks ON exam_subjects
FOR EACH ROW EXECUTE FUNCTION exam_subjects_component_full();

-- Component marks of a present mark; exam_marks.marks is their sum, written
-- in the same transaction, so analytics keep reading exam_marks unchanged.
CREATE TABLE exam_component_marks (
    exam_mark_id uuid NOT NULL REFERENCES exam_marks(id) ON DELETE CASCADE,
    exam_subject_id uuid NOT NULL,
    component_key text NOT NULL,
    marks numeric(6,2) NOT NULL CHECK (marks >= 0),
    PRIMARY KEY (exam_mark_id, component_key),
    CONSTRAINT exam_component_marks_component_fk FOREIGN KEY (exam_subject_id, component_key)
        REFERENCES exam_subject_components(exam_subject_id, component_key) ON DELETE CASCADE
);

CREATE FUNCTION exam_component_marks_range_check() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE allowed numeric(6,2);
BEGIN
    SELECT full_marks INTO allowed FROM exam_subject_components
    WHERE exam_subject_id = NEW.exam_subject_id AND component_key = NEW.component_key;
    IF allowed IS NULL OR NEW.marks > allowed THEN
        RAISE EXCEPTION 'component marks exceed configured full marks' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER exam_component_marks_range_trigger BEFORE INSERT OR UPDATE ON exam_component_marks
FOR EACH ROW EXECUTE FUNCTION exam_component_marks_range_check();

-- Each student's optional (fourth) subject for a session. While no student
-- of a class has a choice, the class's fourth subject applies to everyone,
-- as before.
CREATE TABLE student_subject_choices (
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    role text NOT NULL DEFAULT 'optional' CHECK (role = 'optional'),
    updated_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (student_id, session_id, role)
);
CREATE INDEX student_subject_choices_subject_idx ON student_subject_choices (session_id, subject_id);

-- Whether a student takes a subject: every subject except a fourth subject
-- the student did not choose, once the class uses choices.
CREATE FUNCTION student_takes_subject(p_student uuid, p_session uuid, p_class uuid, p_subject uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT NOT coalesce((SELECT acs.is_fourth_subject FROM academic_class_subjects acs WHERE acs.class_id = p_class AND acs.subject_id = p_subject), false)
        OR NOT EXISTS (SELECT 1 FROM student_subject_choices c JOIN students s ON s.id = c.student_id
                       WHERE c.session_id = p_session AND s.session_id = p_session AND s.class_id = p_class)
        OR EXISTS (SELECT 1 FROM student_subject_choices c
                   WHERE c.student_id = p_student AND c.session_id = p_session AND c.subject_id = p_subject)
$$;

-- Whether students and guardians may see an exam's own published result.
-- Class tests are hidden (E1-06); other assessments follow their blueprint.
CREATE FUNCTION exam_visible_to_students(p_exam uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce(
        (SELECT ba.student_visibility = 'with_assessment_publication'
           FROM exams e JOIN blueprint_assessments ba ON ba.version_id = e.blueprint_version_id AND ba.key = e.assessment_key
          WHERE e.id = p_exam),
        (SELECT e.exam_type <> 'class_test' FROM exams e WHERE e.id = p_exam),
        false)
$$;

-- Links exams that have no term to terms and legacy blueprints:
--   * a term per distinct (session, exam name, exam type), ordered by the
--     earliest exam;
--   * a legacy blueprint version per session and class, with one assessment
--     per term and one total per subject, final-only (today's model);
--   * term, assessment and version on every exam.
-- It writes no mark, grade or result. Returns the counts.
CREATE FUNCTION exam_link_legacy() RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE
    terms_made integer := 0;
    versions_made integer := 0;
    exams_linked integer := 0;
    bands jsonb;
BEGIN
    SELECT coalesce(jsonb_agg(jsonb_build_object('grade', b.grade, 'min', b.min_marks, 'max', b.max_marks, 'gpa', b.gpa) ORDER BY b.min_marks DESC), '[]'::jsonb)
      INTO bands
      FROM grading_bands b JOIN school_settings ss ON ss.grading_scheme_id = b.scheme_id AND ss.id = 1;

    -- 1. Terms.
    WITH groups AS (
        SELECT e.session_id, lower(e.name) || '|' || e.exam_type AS legacy_key,
               (array_agg(e.name ORDER BY e.created_at, e.id))[1] AS name, e.exam_type,
               min(e.created_at) AS first_at,
               bool_and(e.status = 'locked') AS all_locked, bool_or(e.status <> 'draft') AS any_published
          FROM exams e
         WHERE e.term_id IS NULL
         GROUP BY e.session_id, lower(e.name), e.exam_type
    ), fresh AS (
        SELECT g.*, row_number() OVER (PARTITION BY g.session_id ORDER BY g.first_at, g.legacy_key) AS n
          FROM groups g
         WHERE NOT EXISTS (SELECT 1 FROM exam_terms t WHERE t.session_id = g.session_id AND t.legacy_key = g.legacy_key)
    ), inserted AS (
        INSERT INTO exam_terms (session_id, name, sequence, kind, status, legacy_key)
        SELECT f.session_id,
               CASE WHEN EXISTS (SELECT 1 FROM exam_terms t WHERE t.session_id = f.session_id AND lower(t.name) = lower(f.name))
                      OR EXISTS (SELECT 1 FROM fresh o WHERE o.session_id = f.session_id AND lower(o.name) = lower(f.name) AND o.n < f.n)
                    THEN left(f.name, 100) || ' (' || replace(f.exam_type, '_', ' ') || ')'
                    ELSE f.name END,
               coalesce((SELECT max(t.sequence) FROM exam_terms t WHERE t.session_id = f.session_id), 0) + f.n::integer,
               CASE WHEN f.exam_type = 'annual' THEN 'annual' ELSE 'term' END,
               CASE WHEN f.all_locked THEN 'closed' WHEN f.any_published THEN 'results' ELSE 'running' END,
               f.legacy_key
          FROM fresh f
        RETURNING 1
    )
    SELECT count(*) INTO terms_made FROM inserted;

    -- 2. A legacy blueprint and its first version for each session and class
    --    whose blueprint has no version yet.
    INSERT INTO assessment_blueprints (session_id, class_id, name)
    SELECT DISTINCT e.session_id, e.class_id, left(c.name || ' · ' || s.name, 140) || ' (existing exams)'
      FROM exams e JOIN academic_classes c ON c.id = e.class_id JOIN academic_sessions s ON s.id = e.session_id
     WHERE e.term_id IS NULL
    ON CONFLICT (session_id, class_id) DO NOTHING;

    CREATE TEMP TABLE legacy_versions ON COMMIT DROP AS
    SELECT b.id AS blueprint_id, gen_random_uuid() AS version_id, b.session_id, b.class_id
      FROM assessment_blueprints b
     WHERE NOT EXISTS (SELECT 1 FROM blueprint_versions v WHERE v.blueprint_id = b.id)
       AND EXISTS (SELECT 1 FROM exams e WHERE e.session_id = b.session_id AND e.class_id = b.class_id AND e.term_id IS NULL);

    INSERT INTO blueprint_versions (id, blueprint_id, version, status, legacy, definition)
    SELECT lv.version_id, lv.blueprint_id, 1, 'draft', true, '{"legacy": true}'::jsonb FROM legacy_versions lv;

    -- One assessment per term the class's exams belong to.
    INSERT INTO blueprint_assessments (version_id, key, name, term_id, kind, sequence, student_visibility)
    SELECT lv.version_id, 'A' || dense_rank() OVER (PARTITION BY lv.version_id ORDER BY t.sequence),
           left(t.name, 80), t.id,
           CASE WHEN t.legacy_key LIKE '%|class_test' THEN 'class_test' ELSE 'term_exam' END,
           dense_rank() OVER (PARTITION BY lv.version_id ORDER BY t.sequence),
           CASE WHEN t.legacy_key LIKE '%|class_test' THEN 'hidden' ELSE 'with_assessment_publication' END
      FROM legacy_versions lv
      JOIN exam_terms t ON t.session_id = lv.session_id
     WHERE EXISTS (SELECT 1 FROM exams e WHERE e.session_id = lv.session_id AND e.class_id = lv.class_id AND e.term_id IS NULL
                     AND t.legacy_key = lower(e.name) || '|' || e.exam_type);

    -- Every subject used by the class's exams, with its class role.
    INSERT INTO blueprint_subjects (version_id, subject_id, role, display_order)
    SELECT lv.version_id, es.subject_id, CASE WHEN bool_or(acs.is_fourth_subject) THEN 'optional' ELSE 'required' END,
           row_number() OVER (PARTITION BY lv.version_id ORDER BY min(sub.code))
      FROM legacy_versions lv
      JOIN exams e ON e.session_id = lv.session_id AND e.class_id = lv.class_id AND e.term_id IS NULL
      JOIN exam_subjects es ON es.exam_id = e.id
      JOIN academic_subjects sub ON sub.id = es.subject_id
      JOIN academic_class_subjects acs ON acs.class_id = es.class_id AND acs.subject_id = es.subject_id
     GROUP BY lv.version_id, es.subject_id;

    -- One total per subject and assessment, with the full and pass marks of
    -- the earliest exam (each exam keeps its own sheet values).
    INSERT INTO blueprint_components (blueprint_subject_id, assessment_key, component_key, full_marks, pass_marks)
    SELECT DISTINCT ON (bs.id, ba.key) bs.id, ba.key, 'TOTAL', es.full_marks, es.pass_marks
      FROM legacy_versions lv
      JOIN blueprint_subjects bs ON bs.version_id = lv.version_id
      JOIN blueprint_assessments ba ON ba.version_id = lv.version_id
      JOIN exam_terms t ON t.id = ba.term_id
      JOIN exams e ON e.session_id = lv.session_id AND e.class_id = lv.class_id AND e.term_id IS NULL
                  AND t.legacy_key = lower(e.name) || '|' || e.exam_type
      JOIN exam_subjects es ON es.exam_id = e.id AND es.subject_id = bs.subject_id
     ORDER BY bs.id, ba.key, e.created_at, e.id;

    UPDATE blueprint_versions v
       SET status = 'active', activated_at = now(), grading_scheme_snapshot = bands,
           definition_sha256 = encode(sha256(convert_to(v.definition::text, 'UTF8')), 'hex')
      FROM legacy_versions lv WHERE v.id = lv.version_id;
    GET DIAGNOSTICS versions_made = ROW_COUNT;

    -- 3. Link the exams.
    --    An exam whose class already had a blueprint keeps no version; it is
    --    still linked to its term, which is what class positions need.
    UPDATE exams e
       SET term_id = t.id,
           assessment_key = coalesce((SELECT ba.key FROM blueprint_assessments ba JOIN legacy_versions lv ON lv.version_id = ba.version_id
                                       WHERE ba.term_id = t.id AND lv.session_id = e.session_id AND lv.class_id = e.class_id), 'A' || t.sequence),
           blueprint_version_id = (SELECT ba.version_id FROM blueprint_assessments ba JOIN legacy_versions lv ON lv.version_id = ba.version_id
                                    WHERE ba.term_id = t.id AND lv.session_id = e.session_id AND lv.class_id = e.class_id)
      FROM exam_terms t
     WHERE e.term_id IS NULL
       AND t.session_id = e.session_id AND t.legacy_key = lower(e.name) || '|' || e.exam_type;
    GET DIAGNOSTICS exams_linked = ROW_COUNT;

    DROP TABLE legacy_versions;
    -- Published class tests students could open before and no longer see on
    -- their own (E1-06); recorded so the change is on the audit trail.
    RETURN jsonb_build_object('terms', terms_made, 'blueprint_versions', versions_made, 'exams', exams_linked,
        'class_tests_hidden', (SELECT count(*) FROM exams WHERE exam_type = 'class_test' AND status IN ('published', 'locked') AND term_id IS NOT NULL));
END;
$$;

INSERT INTO audit_events (action, target_type, target_id, outcome, metadata)
SELECT 'exam.blueprints_migrated', 'school', 'exams', 'success', counts || jsonb_build_object('migration', 1502)
FROM (SELECT exam_link_legacy() AS counts) x
WHERE EXISTS (SELECT 1 FROM exams);
