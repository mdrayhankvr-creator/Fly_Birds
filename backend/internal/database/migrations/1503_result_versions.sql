-- E6: published result versions and snapshots.
--
-- Every published result is an immutable version with a snapshot of who it
-- belongs to, the rules it was calculated with, the inputs it used and the
-- outputs that were published. A correction adds a version and supersedes
-- the previous one; nothing published is ever updated in place or deleted.
-- exam_results stays the projection of the current assessment versions that
-- analytics, exports and notifications read.
--
-- The upgrade records every result already published as version 1 exactly
-- as it is stored: nothing is recalculated and no existing value changes.

-- A publication is one batch of one scope for one class: the first
-- publication, a correction or a position update. Term and annual batches
-- are approved first; the approval binds to the exact inputs (inputs_sha256)
-- and publication is refused if anything changed since.
CREATE TABLE result_publications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scope text NOT NULL CHECK (scope IN ('assessment', 'term', 'annual')),
    -- The exam (assessment), the term (term) or the session (annual).
    scope_id uuid NOT NULL,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid REFERENCES academic_sections(id) ON DELETE RESTRICT,
    -- Numbered when published: 1 is the first publication.
    revision integer CHECK (revision >= 1),
    kind text NOT NULL CHECK (kind IN ('publication', 'correction', 'position_update')),
    status text NOT NULL CHECK (status IN ('approved', 'published', 'discarded')),
    source text NOT NULL DEFAULT 'engine' CHECK (source IN ('engine', 'migration')),
    blueprint_version_id uuid REFERENCES blueprint_versions(id) ON DELETE RESTRICT,
    rules jsonb NOT NULL,
    rules_sha256 text NOT NULL,
    inputs_sha256 text NOT NULL CHECK (inputs_sha256 ~ '^[0-9a-f]{64}$'),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    students integer NOT NULL DEFAULT 0 CHECK (students >= 0),
    changed integer NOT NULL DEFAULT 0 CHECK (changed >= 0),
    approved_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    approved_at timestamptz,
    published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT result_publications_published_ck CHECK (status <> 'published' OR (revision IS NOT NULL AND published_at IS NOT NULL)),
    CONSTRAINT result_publications_approved_ck CHECK (status <> 'approved' OR approved_at IS NOT NULL),
    CONSTRAINT result_publications_revision_key UNIQUE (scope, scope_id, class_id, revision)
);
-- At most one approval waits for publication per scope and class.
CREATE UNIQUE INDEX result_publications_open_idx ON result_publications (scope, scope_id, class_id) WHERE status = 'approved';
CREATE INDEX result_publications_lookup_idx ON result_publications (session_id, scope, class_id, status);

-- One student's result in one scope, in one version.
CREATE TABLE result_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    publication_id uuid NOT NULL REFERENCES result_publications(id) ON DELETE RESTRICT,
    scope text NOT NULL CHECK (scope IN ('assessment', 'term', 'annual')),
    scope_id uuid NOT NULL,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    -- The student's section when the version was published.
    section_id uuid NOT NULL REFERENCES academic_sections(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version >= 1),
    status text NOT NULL CHECK (status IN ('published', 'superseded', 'withdrawn')),
    outcome text NOT NULL CHECK (outcome IN ('passed', 'failed', 'withheld')),
    total_marks numeric(9,2) CHECK (total_marks >= 0),
    full_marks numeric(9,2) CHECK (full_marks > 0),
    gpa numeric(3,2) CHECK (gpa BETWEEN 0 AND 5),
    class_position integer CHECK (class_position > 0),
    section_position integer CHECK (section_position > 0),
    -- SHA-256 of the snapshot's inputs as PostgreSQL stores them (jsonb
    -- text), computed by a trigger so every version hashes the same way.
    inputs_sha256 text NOT NULL,
    snapshot jsonb NOT NULL,
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    supersedes_id uuid REFERENCES result_versions(id) ON DELETE RESTRICT,
    published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_at timestamptz NOT NULL,
    -- Who superseded or withdrew it, when and why.
    closed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    closed_at timestamptz,
    close_reason text NOT NULL DEFAULT '' CHECK (char_length(close_reason) <= 500),
    CONSTRAINT result_versions_version_key UNIQUE (scope, scope_id, student_id, version),
    CONSTRAINT result_versions_withheld_ck CHECK ((outcome = 'withheld') = (gpa IS NULL)
        AND (outcome = 'withheld' OR (total_marks IS NOT NULL AND full_marks IS NOT NULL))
        AND (outcome <> 'withheld' OR (total_marks IS NULL AND class_position IS NULL AND section_position IS NULL))),
    CONSTRAINT result_versions_closed_ck CHECK ((status = 'published') = (closed_at IS NULL))
);
-- One current (published) version per student and scope.
CREATE UNIQUE INDEX result_versions_current_idx ON result_versions (scope, scope_id, student_id) WHERE status = 'published';
CREATE INDEX result_versions_student_idx ON result_versions (student_id, scope, status);
CREATE INDEX result_versions_publication_idx ON result_versions (publication_id);
CREATE INDEX result_versions_scope_idx ON result_versions (scope, scope_id, class_id, status);

-- The projection row of an assessment result points at its version.
ALTER TABLE exam_results ADD COLUMN result_version_id uuid REFERENCES result_versions(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX exam_results_version_key ON exam_results (result_version_id) WHERE result_version_id IS NOT NULL;

-- A correction reopens one student's marks in one published exam. It is
-- applied by publishing a correction (a new version) or cancelled.
CREATE TABLE result_corrections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    exam_id uuid NOT NULL REFERENCES exams(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    reason text NOT NULL CHECK (reason = btrim(reason) AND char_length(reason) BETWEEN 3 AND 500),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'applied', 'cancelled')),
    opened_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    opened_at timestamptz NOT NULL DEFAULT now(),
    closed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    closed_at timestamptz,
    publication_id uuid REFERENCES result_publications(id) ON DELETE RESTRICT,
    CONSTRAINT result_corrections_closed_ck CHECK ((status = 'open') = (closed_at IS NULL))
);
CREATE UNIQUE INDEX result_corrections_open_idx ON result_corrections (exam_id, student_id) WHERE status = 'open';
CREATE INDEX result_corrections_exam_idx ON result_corrections (exam_id, status);

-- Hashes are computed by the database, from the stored jsonb.
CREATE FUNCTION result_publications_hash() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.rules_sha256 := encode(sha256(convert_to(NEW.rules::text, 'UTF8')), 'hex');
    RETURN NEW;
END;
$$;
CREATE TRIGGER result_publications_hash_trigger BEFORE INSERT ON result_publications
FOR EACH ROW EXECUTE FUNCTION result_publications_hash();

-- A publication waiting for publication may be published or discarded; a
-- published or discarded one never changes, and none is ever deleted.
CREATE FUNCTION result_publications_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'result publications are never deleted' USING ERRCODE = '23514';
    END IF;
    IF OLD.status <> 'approved' OR NEW.status NOT IN ('published', 'discarded') THEN
        RAISE EXCEPTION 'a published or discarded result publication cannot change' USING ERRCODE = '23514';
    END IF;
    IF (to_jsonb(NEW) - ARRAY['status', 'revision', 'published_by', 'published_at', 'students', 'changed'])
       IS DISTINCT FROM (to_jsonb(OLD) - ARRAY['status', 'revision', 'published_by', 'published_at', 'students', 'changed']) THEN
        RAISE EXCEPTION 'an approved result publication can only be published or discarded' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER result_publications_guard_trigger BEFORE UPDATE OR DELETE ON result_publications
FOR EACH ROW EXECUTE FUNCTION result_publications_guard();

CREATE FUNCTION result_versions_hash() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.snapshot->>'format' IS DISTINCT FROM '1' OR NOT (NEW.snapshot ? 'inputs') OR NOT (NEW.snapshot ? 'outputs') THEN
        RAISE EXCEPTION 'a result snapshot needs format 1, inputs and outputs' USING ERRCODE = '23514';
    END IF;
    NEW.inputs_sha256 := encode(sha256(convert_to((NEW.snapshot->'inputs')::text, 'UTF8')), 'hex');
    RETURN NEW;
END;
$$;
CREATE TRIGGER result_versions_hash_trigger BEFORE INSERT ON result_versions
FOR EACH ROW EXECUTE FUNCTION result_versions_hash();

-- A published version can only be closed (superseded or withdrawn), once;
-- its values and snapshot never change and it is never deleted.
CREATE FUNCTION result_versions_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'result versions are never deleted' USING ERRCODE = '23514';
    END IF;
    IF OLD.status <> 'published' OR NEW.status NOT IN ('superseded', 'withdrawn') THEN
        RAISE EXCEPTION 'only a published result version can be superseded or withdrawn' USING ERRCODE = '23514';
    END IF;
    IF (to_jsonb(NEW) - ARRAY['status', 'closed_by', 'closed_at', 'close_reason'])
       IS DISTINCT FROM (to_jsonb(OLD) - ARRAY['status', 'closed_by', 'closed_at', 'close_reason']) THEN
        RAISE EXCEPTION 'a published result version never changes' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER result_versions_guard_trigger BEFORE UPDATE OR DELETE ON result_versions
FOR EACH ROW EXECUTE FUNCTION result_versions_guard();

-- An exam or term with result versions keeps existing.
CREATE FUNCTION result_scope_keep() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM result_versions v WHERE v.scope = TG_ARGV[0] AND v.scope_id = OLD.id) THEN
        RAISE EXCEPTION 'results were published for it; it cannot be deleted' USING ERRCODE = '23503';
    END IF;
    RETURN OLD;
END;
$$;
CREATE TRIGGER exams_keep_versioned BEFORE DELETE ON exams
FOR EACH ROW EXECUTE FUNCTION result_scope_keep('assessment');
CREATE TRIGGER exam_terms_keep_versioned BEFORE DELETE ON exam_terms
FOR EACH ROW EXECUTE FUNCTION result_scope_keep('term');

-- Students may see an exam's own result per its blueprint visibility:
-- "after the term result" now means once the class's term result is
-- published (E6). Class tests outside a blueprint stay hidden (E1-06).
CREATE OR REPLACE FUNCTION exam_visible_to_students(p_exam uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce(
        (SELECT CASE ba.student_visibility
                    WHEN 'with_assessment_publication' THEN true
                    WHEN 'after_term_publication' THEN EXISTS (
                        SELECT 1 FROM result_publications p
                         WHERE p.scope = 'term' AND p.scope_id = e.term_id AND p.class_id = e.class_id AND p.status = 'published')
                    ELSE false END
           FROM exams e JOIN blueprint_assessments ba ON ba.version_id = e.blueprint_version_id AND ba.key = e.assessment_key
          WHERE e.id = p_exam),
        (SELECT e.exam_type <> 'class_test' FROM exams e WHERE e.id = p_exam),
        false)
$$;

-- Records every result already published as version 1, from the stored
-- marks, grades and results, without recalculating anything. The rules are
-- the school's current grading bands (the ones in force) and the exam's
-- policy. Returns the counts.
CREATE FUNCTION result_versions_migrate() RETURNS jsonb LANGUAGE plpgsql AS $$
DECLARE
    bands jsonb;
    ex record;
    rules jsonb;
    rows jsonb;
    batch text;
    pub uuid;
    n_pub integer := 0;
    n_ver integer := 0;
    n_withheld integer := 0;
BEGIN
    SELECT coalesce(jsonb_agg(jsonb_build_object('grade', b.grade, 'min', b.min_marks, 'max', b.max_marks, 'gpa', b.gpa) ORDER BY b.min_marks DESC), '[]'::jsonb)
      INTO bands
      FROM grading_bands b JOIN school_settings ss ON ss.grading_scheme_id = b.scheme_id AND ss.id = 1;

    FOR ex IN
        SELECT e.id, e.name, e.exam_type, e.session_id, e.class_id, e.section_id, e.term_id, e.assessment_key, e.blueprint_version_id,
               e.updated_by, coalesce(e.published_at, e.updated_at) AS published_at,
               se.name AS session_name, c.name AS class_name, s.name AS section_name, t.name AS term_name,
               v.gpa_rounding, v.absent_policy, v.paper_rule, v.rank_mode
          FROM exams e
          JOIN academic_sessions se ON se.id = e.session_id
          JOIN academic_classes c ON c.id = e.class_id
          JOIN academic_sections s ON s.id = e.section_id
          LEFT JOIN exam_terms t ON t.id = e.term_id
          LEFT JOIN blueprint_versions v ON v.id = e.blueprint_version_id
         WHERE e.status IN ('published', 'locked')
           AND NOT EXISTS (SELECT 1 FROM result_publications p WHERE p.scope = 'assessment' AND p.scope_id = e.id)
         ORDER BY coalesce(e.published_at, e.updated_at), e.id
    LOOP
        rules := jsonb_build_object(
            'format', 1, 'scope', 'assessment', 'source', 'migration',
            'bands', bands, 'bands_source', 'school',
            'gpa_rounding', coalesce(ex.gpa_rounding, 'half_up'),
            'absent', coalesce(ex.absent_policy, 'zero'),
            'paper_rule', coalesce(ex.paper_rule, 'combined'),
            'ranking', coalesce(ex.rank_mode, 'competition'),
            'blueprint_version_id', ex.blueprint_version_id,
            'combined', coalesce((SELECT jsonb_object_agg(bs.subject_id::text, bs.combined_key)
                                    FROM blueprint_subjects bs WHERE bs.version_id = ex.blueprint_version_id AND bs.combined_key IS NOT NULL), '{}'::jsonb));

        -- Each student with a stored result, and each student whose result
        -- is withheld (no stored result), as published.
        SELECT coalesce(jsonb_agg(x ORDER BY x->>'student_id'), '[]'::jsonb) INTO rows FROM (
            SELECT jsonb_build_object(
                'student_id', st.id,
                'outcome', CASE WHEN er.student_id IS NULL THEN 'withheld' WHEN er.passed THEN 'passed' ELSE 'failed' END,
                'total', er.total_marks, 'full', er.full_marks, 'gpa', er.gpa,
                'class_position', er.class_position, 'section_position', er.section_position,
                'snapshot', jsonb_build_object(
                    'format', 1, 'source', 'migration', 'scope', 'assessment',
                    'student', jsonb_build_object('id', st.id, 'name_en', st.name_en, 'name_bn', coalesce(st.name_bn, ''), 'admission_no', st.admission_no,
                        'roll_no', st.roll_no, 'section_id', ex.section_id, 'section_name', ex.section_name, 'class_id', ex.class_id,
                        'class_name', ex.class_name, 'session_id', ex.session_id, 'session_name', ex.session_name),
                    'context', jsonb_build_object('exam_id', ex.id, 'exam_name', ex.name, 'exam_type', ex.exam_type,
                        'term_id', ex.term_id, 'term_name', coalesce(ex.term_name, ''), 'assessment_key', coalesce(ex.assessment_key, '')),
                    'inputs', jsonb_build_object('lines', coalesce((
                        SELECT jsonb_agg(jsonb_build_object(
                                   'mark_id', m.id, 'sheet_id', es.id, 'subject_id', es.subject_id, 'code', sub.code,
                                   'name_en', sub.name_en, 'name_bn', sub.name_bn,
                                   'full', es.full_marks::text, 'pass', es.pass_marks::text, 'fourth', acs.is_fourth_subject,
                                   'combined', coalesce(rules->'combined'->>es.subject_id::text, ''),
                                   'status', m.status, 'marks', m.marks::text, 'reason', m.status_reason, 'row_version', m.row_version,
                                   'components', coalesce((
                                       SELECT jsonb_agg(jsonb_build_object('key', c.component_key, 'marks', cm.marks::text, 'full', c.full_marks::text, 'pass', c.pass_marks::text)
                                                        ORDER BY c.display_order, c.component_key)
                                         FROM exam_component_marks cm
                                         JOIN exam_subject_components c ON c.exam_subject_id = cm.exam_subject_id AND c.component_key = cm.component_key
                                        WHERE cm.exam_mark_id = m.id), '[]'::jsonb))
                                 ORDER BY es.subject_id)
                          FROM exam_marks m
                          JOIN exam_subjects es ON es.id = m.exam_subject_id
                          JOIN academic_subjects sub ON sub.id = es.subject_id
                          JOIN academic_class_subjects acs ON acs.class_id = es.class_id AND acs.subject_id = es.subject_id
                         WHERE m.exam_id = ex.id AND m.student_id = st.id
                           AND student_takes_subject(st.id, ex.session_id, ex.class_id, es.subject_id)), '[]'::jsonb)),
                    'outputs', jsonb_build_object(
                        'outcome', CASE WHEN er.student_id IS NULL THEN 'withheld' WHEN er.passed THEN 'passed' ELSE 'failed' END,
                        'total', er.total_marks::text, 'full', er.full_marks::text, 'gpa', er.gpa::text, 'passed', coalesce(er.passed, false),
                        'class_position', er.class_position, 'section_position', er.section_position,
                        'subjects', CASE WHEN er.student_id IS NULL THEN '[]'::jsonb ELSE coalesce((
                            SELECT jsonb_agg(jsonb_build_object(
                                       'key', es.subject_id, 'sheet_id', es.id, 'subject_ids', jsonb_build_array(es.subject_id), 'code', sub.code,
                                       'name_en', sub.name_en, 'name_bn', sub.name_bn, 'optional', acs.is_fourth_subject,
                                       'combined', coalesce(rules->'combined'->>es.subject_id::text, ''),
                                       'status', m.status, 'marks', m.marks::text, 'full', es.full_marks::text, 'pass', es.pass_marks::text,
                                       'grade', m.grade, 'point', m.grade_point::text, 'passed', m.passed, 'reasons', '[]'::jsonb,
                                       'components', coalesce((
                                           SELECT jsonb_agg(jsonb_build_object('key', c.component_key, 'marks', cm.marks::text, 'full', c.full_marks::text, 'pass', c.pass_marks::text)
                                                            ORDER BY c.display_order, c.component_key)
                                             FROM exam_component_marks cm
                                             JOIN exam_subject_components c ON c.exam_subject_id = cm.exam_subject_id AND c.component_key = cm.component_key
                                            WHERE cm.exam_mark_id = m.id), '[]'::jsonb))
                                     ORDER BY sub.code, es.subject_id)
                              FROM exam_marks m
                              JOIN exam_subjects es ON es.id = m.exam_subject_id
                              JOIN academic_subjects sub ON sub.id = es.subject_id
                              JOIN academic_class_subjects acs ON acs.class_id = es.class_id AND acs.subject_id = es.subject_id
                             WHERE m.exam_id = ex.id AND m.student_id = st.id), '[]'::jsonb) END))) AS x
              FROM students st
              LEFT JOIN exam_results er ON er.exam_id = ex.id AND er.student_id = st.id
             WHERE st.id IN (SELECT r.student_id FROM exam_results r WHERE r.exam_id = ex.id
                             UNION
                             SELECT m.student_id FROM exam_marks m
                              WHERE m.exam_id = ex.id AND m.status = 'withheld'
                                AND NOT EXISTS (SELECT 1 FROM exam_results r WHERE r.exam_id = ex.id AND r.student_id = m.student_id))
        ) q;

        IF jsonb_array_length(rows) = 0 THEN
            CONTINUE;
        END IF;

        -- The batch hash: every student's inputs hash, in student order.
        SELECT string_agg(encode(sha256(convert_to((r->'snapshot'->'inputs')::text, 'UTF8')), 'hex'), '' ORDER BY r->>'student_id')
          INTO batch FROM jsonb_array_elements(rows) r;

        INSERT INTO result_publications (scope, scope_id, session_id, class_id, section_id, revision, kind, status, source,
                                         blueprint_version_id, rules, inputs_sha256, reason, students, changed, published_by, published_at)
        VALUES ('assessment', ex.id, ex.session_id, ex.class_id, ex.section_id, 1, 'publication', 'published', 'migration',
                ex.blueprint_version_id, rules, encode(sha256(convert_to(batch, 'UTF8')), 'hex'),
                'Recorded by the E6 upgrade from the stored result', jsonb_array_length(rows), jsonb_array_length(rows), ex.updated_by, ex.published_at)
        RETURNING id INTO pub;

        INSERT INTO result_versions (publication_id, scope, scope_id, session_id, class_id, section_id, student_id, version, status, outcome,
                                     total_marks, full_marks, gpa, class_position, section_position, inputs_sha256, snapshot, reason,
                                     published_by, published_at)
        SELECT pub, 'assessment', ex.id, ex.session_id, ex.class_id, ex.section_id, (r->>'student_id')::uuid, 1, 'published', r->>'outcome',
               (r->>'total')::numeric, (r->>'full')::numeric, (r->>'gpa')::numeric,
               (r->>'class_position')::integer, (r->>'section_position')::integer, '', r->'snapshot', '',
               ex.updated_by, ex.published_at
          FROM jsonb_array_elements(rows) r;

        UPDATE exam_results er SET result_version_id = v.id
          FROM result_versions v
         WHERE v.publication_id = pub AND er.exam_id = ex.id AND er.student_id = v.student_id;

        n_pub := n_pub + 1;
        n_ver := n_ver + jsonb_array_length(rows);
        n_withheld := n_withheld + (SELECT count(*) FROM jsonb_array_elements(rows) r WHERE r->>'outcome' = 'withheld');
    END LOOP;
    RETURN jsonb_build_object('publications', n_pub, 'versions', n_ver, 'withheld', n_withheld);
END;
$$;

INSERT INTO audit_events (action, target_type, target_id, outcome, metadata)
SELECT 'exam.result_versions_migrated', 'school', 'exams', 'success', counts || jsonb_build_object('migration', 1503)
FROM (SELECT result_versions_migrate() AS counts) x
WHERE EXISTS (SELECT 1 FROM exams WHERE status IN ('published', 'locked'));
