-- M15: enrolment history.
--
-- A student has one enrolment per academic session: the class, section and
-- roll number of that year, and how the year ended. The students row keeps
-- the current session, class, section, roll and status exactly as before
-- (every existing query and API keeps reading it); a trigger keeps the
-- current enrolment in step with it, so every writer (admission, CSV import,
-- the demo seed, provisioning) records history without code changes.
--
-- Marks and results now reference the enrolment of their session instead of
-- the student's current placement, so a student with marks can change
-- section, and later be promoted, without any recorded mark, result,
-- attendance or fee row being rewritten.
--
-- The upgrade records each student's current placement as their current
-- enrolment, and each earlier session that attendance proves as a closed
-- "inferred" enrolment. No mark, result, attendance or fee value changes.

CREATE TABLE student_enrolments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    section_id uuid NOT NULL,
    roll_no integer CHECK (roll_no IS NULL OR roll_no BETWEEN 1 AND 999999),
    -- current: the students row points here; closed: an earlier year.
    state text NOT NULL DEFAULT 'current' CHECK (state IN ('current', 'closed')),
    -- The student's status in this enrolment (the students.status values);
    -- for a closed enrolment, the status when it closed.
    student_status text NOT NULL CHECK (student_status IN ('active', 'inactive', 'graduated', 'transferred')),
    -- How a closed year ended. moved: the student was moved to another
    -- session by editing the student (no promotion decision was recorded).
    -- Inferred enrolments have none.
    outcome text CHECK (outcome IN ('promoted', 'repeated', 'graduated', 'withdrawn', 'transferred', 'moved')),
    -- admission: created with the student; backfill/inferred: the upgrade;
    -- moved: opened by an edit that moved the student to this session;
    -- promotion: opened by year-end promotion.
    source text NOT NULL CHECK (source IN ('admission', 'backfill', 'inferred', 'moved', 'promotion')),
    started_on date,
    ended_on date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT student_enrolments_section_fk FOREIGN KEY (section_id, class_id, session_id)
        REFERENCES academic_sections(id, class_id, session_id) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CONSTRAINT student_enrolments_session_key UNIQUE (student_id, session_id),
    CONSTRAINT student_enrolments_closed_ck CHECK ((state = 'current') = (ended_on IS NULL)),
    CONSTRAINT student_enrolments_outcome_ck CHECK (outcome IS NULL OR state = 'closed'),
    CONSTRAINT student_enrolments_dates_ck CHECK (started_on IS NULL OR ended_on IS NULL OR ended_on >= started_on)
);
CREATE UNIQUE INDEX student_enrolments_current_key ON student_enrolments (student_id) WHERE state = 'current';
CREATE UNIQUE INDEX student_enrolments_roll_key ON student_enrolments (section_id, roll_no) WHERE roll_no IS NOT NULL;
CREATE INDEX student_enrolments_roster_idx ON student_enrolments (session_id, class_id, section_id, state, student_id);

-- What happened to an enrolment, in order. Rows are never changed.
CREATE TABLE student_enrolment_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    enrolment_id uuid NOT NULL REFERENCES student_enrolments(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('enrolled', 'section_changed', 'class_changed', 'roll_changed', 'status_changed', 'session_corrected', 'closed')),
    from_class_id uuid REFERENCES academic_classes(id) ON DELETE RESTRICT,
    to_class_id uuid REFERENCES academic_classes(id) ON DELETE RESTRICT,
    from_section_id uuid REFERENCES academic_sections(id) ON DELETE RESTRICT,
    to_section_id uuid REFERENCES academic_sections(id) ON DELETE RESTRICT,
    from_value text NOT NULL DEFAULT '',
    to_value text NOT NULL DEFAULT '',
    actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX student_enrolment_events_enrolment_idx ON student_enrolment_events (enrolment_id, occurred_at, id);
CREATE INDEX student_enrolment_events_student_idx ON student_enrolment_events (student_id, occurred_at, id);

CREATE FUNCTION student_enrolment_events_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'enrolment events never change' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER student_enrolment_events_guard_trigger BEFORE UPDATE ON student_enrolment_events
FOR EACH ROW EXECUTE FUNCTION student_enrolment_events_guard();

-- 1. Backfill: every student's current placement is their current enrolment.
INSERT INTO student_enrolments (student_id, session_id, class_id, section_id, roll_no, state, student_status, source, started_on)
SELECT s.id, s.session_id, s.class_id, s.section_id, s.roll_no, 'current', s.status, 'backfill',
       GREATEST(se.start_date, s.admission_date)
  FROM students s JOIN academic_sessions se ON se.id = s.session_id;

-- 2. Earlier sessions proven by attendance (students moved to another
-- session by editing them): a closed enrolment in the class and section of
-- their last attendance that year. Its outcome is unknown, so it stays empty.
INSERT INTO student_enrolments (student_id, session_id, class_id, section_id, state, student_status, source, started_on, ended_on)
SELECT DISTINCT ON (a.student_id, a.session_id) a.student_id, a.session_id, a.class_id, a.section_id, 'closed', 'active', 'inferred',
       min(a.attendance_date) OVER w, max(a.attendance_date) OVER w
  FROM student_attendance a JOIN students s ON s.id = a.student_id
 WHERE a.session_id <> s.session_id
WINDOW w AS (PARTITION BY a.student_id, a.session_id)
 ORDER BY a.student_id, a.session_id, a.attendance_date DESC, a.created_at DESC;

-- 3. Marks, results and attendance reference the enrolment of their session.
-- The old keys required the student's current placement, which made any
-- student with a mark impossible to move (23503).
ALTER TABLE exam_marks DROP CONSTRAINT exam_marks_student_fk,
    ADD CONSTRAINT exam_marks_enrolment_fk FOREIGN KEY (student_id, session_id)
        REFERENCES student_enrolments(student_id, session_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE exam_results DROP CONSTRAINT exam_results_student_fk,
    ADD CONSTRAINT exam_results_enrolment_fk FOREIGN KEY (student_id, session_id)
        REFERENCES student_enrolments(student_id, session_id) ON UPDATE RESTRICT ON DELETE RESTRICT;
ALTER TABLE students DROP CONSTRAINT students_exam_context_key;
ALTER TABLE student_attendance ADD CONSTRAINT student_attendance_enrolment_fk FOREIGN KEY (student_id, session_id)
    REFERENCES student_enrolments(student_id, session_id) ON UPDATE RESTRICT ON DELETE CASCADE;

-- A new mark or result must belong to the class of the student's enrolment
-- that year, and to its section or a section the student was moved from.
-- Rows already recorded are never checked again, so a later move never
-- invalidates them.
CREATE FUNCTION enrolment_placement_check() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM student_enrolments e
         WHERE e.student_id = NEW.student_id AND e.session_id = NEW.session_id AND e.class_id = NEW.class_id
           AND (e.section_id = NEW.section_id OR EXISTS (
                SELECT 1 FROM student_enrolment_events v
                 WHERE v.enrolment_id = e.id AND NEW.section_id IN (v.from_section_id, v.to_section_id)))) THEN
        RAISE EXCEPTION 'the student was not enrolled in this class and section in this session' USING ERRCODE = '23503';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER exam_marks_placement_trigger BEFORE INSERT OR UPDATE OF student_id, session_id, class_id, section_id ON exam_marks
FOR EACH ROW EXECUTE FUNCTION enrolment_placement_check();
CREATE TRIGGER exam_results_placement_trigger BEFORE INSERT OR UPDATE OF student_id, session_id, class_id, section_id ON exam_results
FOR EACH ROW EXECUTE FUNCTION enrolment_placement_check();

-- Whether a student has anything recorded in a session: then that year is
-- history and the student cannot simply be re-labelled into another year.
CREATE FUNCTION student_has_session_records(p_student uuid, p_session uuid) RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT EXISTS (SELECT 1 FROM student_attendance WHERE student_id = p_student AND session_id = p_session)
        OR EXISTS (SELECT 1 FROM exam_marks WHERE student_id = p_student AND session_id = p_session)
        OR EXISTS (SELECT 1 FROM exam_results WHERE student_id = p_student AND session_id = p_session)
        OR EXISTS (SELECT 1 FROM result_versions WHERE student_id = p_student AND session_id = p_session)
        OR EXISTS (SELECT 1 FROM student_fees WHERE student_id = p_student AND session_id = p_session)
        OR EXISTS (SELECT 1 FROM student_subject_choices WHERE student_id = p_student AND session_id = p_session)
        OR EXISTS (SELECT 1 FROM homework_submissions h JOIN class_diaries d ON d.id = h.diary_id
                    WHERE h.student_id = p_student AND d.session_id = p_session)
$$;

-- Keeps the current enrolment in step with the students row.
--
-- * Same session: class, section, roll and status are updated on the
--   current enrolment and each change is logged. A class change is refused
--   while the student has marks or results in another class that year.
-- * Another session: if nothing is recorded for the student in the old
--   session, the enrolment is corrected in place. Otherwise the old year is
--   closed (outcome flybirds.enrolment_outcome, default "moved") and a new
--   current enrolment opens (source flybirds.enrolment_source, default
--   "moved"). Moving back into a year the student already has is refused.
--
-- The acting user is read from flybirds.actor_id when the caller sets it.
CREATE FUNCTION students_enrolment_sync() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    cur student_enrolments%ROWTYPE;
    actor uuid := nullif(current_setting('flybirds.actor_id', true), '')::uuid;
    starts date;
    old_end date;
    new_id uuid;
BEGIN
    SELECT GREATEST(start_date, NEW.admission_date) INTO starts FROM academic_sessions WHERE id = NEW.session_id;
    IF TG_OP = 'INSERT' THEN
        INSERT INTO student_enrolments (student_id, session_id, class_id, section_id, roll_no, state, student_status, source, started_on)
        VALUES (NEW.id, NEW.session_id, NEW.class_id, NEW.section_id, NEW.roll_no, 'current', NEW.status,
                coalesce(nullif(current_setting('flybirds.enrolment_source', true), ''), 'admission'), starts)
        RETURNING id INTO new_id;
        INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, to_class_id, to_section_id, to_value, actor_id)
        VALUES (new_id, NEW.id, 'enrolled', NEW.class_id, NEW.section_id, NEW.status, actor);
        RETURN NEW;
    END IF;

    SELECT * INTO cur FROM student_enrolments WHERE student_id = NEW.id AND state = 'current' FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'the student has no current enrolment' USING ERRCODE = '23514';
    END IF;

    IF NEW.session_id = cur.session_id THEN
        IF NEW.class_id <> cur.class_id AND (
            EXISTS (SELECT 1 FROM exam_marks WHERE student_id = NEW.id AND session_id = cur.session_id AND class_id <> NEW.class_id)
            OR EXISTS (SELECT 1 FROM exam_results WHERE student_id = NEW.id AND session_id = cur.session_id AND class_id <> NEW.class_id)) THEN
            RAISE EXCEPTION 'the student has marks in another class this session; the class cannot change' USING ERRCODE = 'FB001';
        END IF;
        UPDATE student_enrolments SET class_id = NEW.class_id, section_id = NEW.section_id, roll_no = NEW.roll_no,
               student_status = NEW.status, updated_at = now()
         WHERE id = cur.id;
        IF NEW.class_id <> cur.class_id THEN
            INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_class_id, to_class_id, from_section_id, to_section_id, actor_id)
            VALUES (cur.id, NEW.id, 'class_changed', cur.class_id, NEW.class_id, cur.section_id, NEW.section_id, actor);
        ELSIF NEW.section_id <> cur.section_id THEN
            INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_section_id, to_section_id, actor_id)
            VALUES (cur.id, NEW.id, 'section_changed', cur.section_id, NEW.section_id, actor);
        END IF;
        IF NEW.roll_no IS DISTINCT FROM cur.roll_no THEN
            INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_value, to_value, actor_id)
            VALUES (cur.id, NEW.id, 'roll_changed', coalesce(cur.roll_no::text, ''), coalesce(NEW.roll_no::text, ''), actor);
        END IF;
        IF NEW.status <> cur.student_status THEN
            INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_value, to_value, actor_id)
            VALUES (cur.id, NEW.id, 'status_changed', cur.student_status, NEW.status, actor);
        END IF;
        RETURN NEW;
    END IF;

    IF EXISTS (SELECT 1 FROM student_enrolments WHERE student_id = NEW.id AND session_id = NEW.session_id) THEN
        RAISE EXCEPTION 'the student already has an enrolment in that session' USING ERRCODE = 'FB001';
    END IF;

    IF NOT student_has_session_records(NEW.id, cur.session_id) THEN
        UPDATE student_enrolments SET session_id = NEW.session_id, class_id = NEW.class_id, section_id = NEW.section_id,
               roll_no = NEW.roll_no, student_status = NEW.status, started_on = starts, updated_at = now()
         WHERE id = cur.id;
        INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_class_id, to_class_id, from_section_id, to_section_id,
                                              from_value, to_value, actor_id)
        VALUES (cur.id, NEW.id, 'session_corrected', cur.class_id, NEW.class_id, cur.section_id, NEW.section_id,
                cur.session_id::text, NEW.session_id::text, actor);
        RETURN NEW;
    END IF;

    SELECT LEAST(current_date, end_date) INTO old_end FROM academic_sessions WHERE id = cur.session_id;
    UPDATE student_enrolments SET state = 'closed', student_status = OLD.status,
           outcome = coalesce(nullif(current_setting('flybirds.enrolment_outcome', true), ''), 'moved'),
           ended_on = GREATEST(old_end, coalesce(cur.started_on, old_end)), updated_at = now()
     WHERE id = cur.id;
    INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_value, to_value, actor_id)
    VALUES (cur.id, NEW.id, 'closed', OLD.status, coalesce(nullif(current_setting('flybirds.enrolment_outcome', true), ''), 'moved'), actor);
    INSERT INTO student_enrolments (student_id, session_id, class_id, section_id, roll_no, state, student_status, source, started_on)
    VALUES (NEW.id, NEW.session_id, NEW.class_id, NEW.section_id, NEW.roll_no, 'current', NEW.status,
            coalesce(nullif(current_setting('flybirds.enrolment_source', true), ''), 'moved'), starts)
    RETURNING id INTO new_id;
    INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, to_class_id, to_section_id, to_value, actor_id)
    VALUES (new_id, NEW.id, 'enrolled', NEW.class_id, NEW.section_id, NEW.status, actor);
    RETURN NEW;
END;
$$;
CREATE TRIGGER students_enrolment_insert_trigger AFTER INSERT ON students
FOR EACH ROW EXECUTE FUNCTION students_enrolment_sync();
CREATE TRIGGER students_enrolment_update_trigger AFTER UPDATE OF session_id, class_id, section_id, roll_no, status ON students
FOR EACH ROW WHEN ((OLD.session_id, OLD.class_id, OLD.section_id, OLD.roll_no, OLD.status)
                   IS DISTINCT FROM (NEW.session_id, NEW.class_id, NEW.section_id, NEW.roll_no, NEW.status))
EXECUTE FUNCTION students_enrolment_sync();

-- Safety net, checked at commit: a student's current enrolment always
-- matches the students row.
CREATE FUNCTION student_enrolment_matches(p_student uuid) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM students s WHERE s.id = p_student) AND NOT EXISTS (
        SELECT 1 FROM students s JOIN student_enrolments e ON e.student_id = s.id AND e.state = 'current'
         WHERE s.id = p_student AND e.session_id = s.session_id AND e.class_id = s.class_id AND e.section_id = s.section_id
           AND e.roll_no IS NOT DISTINCT FROM s.roll_no AND e.student_status = s.status) THEN
        RAISE EXCEPTION 'the student''s current enrolment does not match the student' USING ERRCODE = '23514';
    END IF;
END;
$$;
CREATE FUNCTION students_enrolment_consistency() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM student_enrolment_matches(NEW.id);
    RETURN NULL;
END;
$$;
CREATE FUNCTION student_enrolments_consistency() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM student_enrolment_matches(OLD.student_id);
    ELSE
        PERFORM student_enrolment_matches(NEW.student_id);
    END IF;
    RETURN NULL;
END;
$$;
-- Named to sort after students_enrolment_update_trigger: triggers on the
-- same event fire in name order, which matters once constraints are immediate.
CREATE CONSTRAINT TRIGGER students_enrolment_verify_trigger AFTER INSERT OR UPDATE OF session_id, class_id, section_id, roll_no, status ON students
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION students_enrolment_consistency();
CREATE CONSTRAINT TRIGGER student_enrolments_consistency_trigger AFTER INSERT OR UPDATE OR DELETE ON student_enrolments
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION student_enrolments_consistency();

-- The students counted on a session's roll: current enrolments of active
-- students, and closed years the student completed (promoted, repeated,
-- graduated, or moved on by an edit). Withdrawn and transferred years and
-- inferred enrolments are history only. Before any promotion this is
-- exactly "students whose current placement is here and who are active".
CREATE VIEW student_rolls AS
SELECT e.id AS enrolment_id, e.student_id, e.session_id, e.class_id, e.section_id, e.roll_no, e.state
  FROM student_enrolments e
 WHERE (e.state = 'current' AND e.student_status = 'active')
    OR (e.state = 'closed' AND e.outcome IN ('promoted', 'repeated', 'graduated', 'moved'));

-- Whether a class uses per-student optional subjects in a session now reads
-- the session's enrolments, not where students are today.
CREATE OR REPLACE FUNCTION student_takes_subject(p_student uuid, p_session uuid, p_class uuid, p_subject uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT NOT coalesce((SELECT acs.is_fourth_subject FROM academic_class_subjects acs WHERE acs.class_id = p_class AND acs.subject_id = p_subject), false)
        OR NOT EXISTS (SELECT 1 FROM student_subject_choices c
                         JOIN student_enrolments e ON e.student_id = c.student_id AND e.session_id = c.session_id
                        WHERE c.session_id = p_session AND e.class_id = p_class)
        OR EXISTS (SELECT 1 FROM student_subject_choices c
                   WHERE c.student_id = p_student AND c.session_id = p_session AND c.subject_id = p_subject)
$$;

-- Read the enrolment history: whoever may read the student.
INSERT INTO permissions (id, description) VALUES
    ('students:history_read', 'Read student enrolment history')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'students:history_read'), ('head_teacher', 'students:history_read')
ON CONFLICT DO NOTHING;

-- What the upgrade did, for the school's audit log.
INSERT INTO audit_events (action, target_type, target_id, outcome, metadata)
SELECT 'student.enrolments_migrated', 'school', 'students', 'success',
       jsonb_build_object('current', count(*) FILTER (WHERE source = 'backfill'),
                          'inferred', count(*) FILTER (WHERE source = 'inferred'), 'migration', 1701)
  FROM student_enrolments
HAVING count(*) > 0;
