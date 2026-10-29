-- M15 stage B1: year-end promotion.
--
-- * A student who leaves (withdrawn, transferred, graduated) has no current
--   enrolment: their last year is closed with that outcome. Readmission
--   reopens the year (same session) or opens a new one (another session).
-- * Moving a student with records to another session needs a promotion
--   (or readmission); a generic edit gets 409 "use promotion".
-- * Withdrawal or transfer suspends the student's login; readmission
--   restores a login suspended that way. Graduates keep their login.
-- * Class progression, promotion batches and their items.
-- * Fee concessions belong to a session; existing ones move to each
--   student's current session.
-- No mark, result, attendance, fee due or payment changes.

-- 1. Enrolment vocabulary.
ALTER TABLE student_enrolments DROP CONSTRAINT student_enrolments_source_check,
    ADD CONSTRAINT student_enrolments_source_check CHECK (source IN ('admission', 'backfill', 'inferred', 'moved', 'promotion', 'readmission'));
ALTER TABLE student_enrolment_events DROP CONSTRAINT student_enrolment_events_kind_check,
    ADD CONSTRAINT student_enrolment_events_kind_check CHECK (kind IN ('enrolled', 'section_changed', 'class_changed', 'roll_changed',
        'status_changed', 'session_corrected', 'closed', 'reopened', 'reverted'));

-- The outcome a status closes a year with.
CREATE FUNCTION student_enrolment_outcome_for(p_status text) RETURNS text LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE p_status WHEN 'inactive' THEN 'withdrawn' WHEN 'transferred' THEN 'transferred' WHEN 'graduated' THEN 'graduated' END
$$;

-- The day a year closes: today, or the session's last day if it is over,
-- never before the enrolment started.
CREATE FUNCTION student_enrolment_end_date(p_session uuid, p_started date) RETURNS date LANGUAGE sql STABLE AS $$
    SELECT GREATEST(LEAST(current_date, s.end_date), coalesce(p_started, LEAST(current_date, s.end_date)))
      FROM academic_sessions s WHERE s.id = p_session
$$;

-- 2. The students row and the enrolments.
--
-- * Active student: exactly one current enrolment matching the row.
-- * Withdrawn, transferred or graduated: no current enrolment; the
--   enrolment of the row's session is closed and matches the row.
CREATE OR REPLACE FUNCTION student_enrolment_matches(p_student uuid) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    st students%ROWTYPE;
BEGIN
    SELECT * INTO st FROM students WHERE id = p_student;
    IF NOT FOUND THEN
        RETURN;
    END IF;
    IF st.status = 'active' THEN
        IF NOT EXISTS (SELECT 1 FROM student_enrolments e WHERE e.student_id = st.id AND e.state = 'current'
                         AND e.session_id = st.session_id AND e.class_id = st.class_id AND e.section_id = st.section_id
                         AND e.roll_no IS NOT DISTINCT FROM st.roll_no AND e.student_status = st.status) THEN
            RAISE EXCEPTION 'the student''s current enrolment does not match the student' USING ERRCODE = '23514';
        END IF;
    ELSIF EXISTS (SELECT 1 FROM student_enrolments e WHERE e.student_id = st.id AND e.state = 'current')
       OR NOT EXISTS (SELECT 1 FROM student_enrolments e WHERE e.student_id = st.id AND e.state = 'closed'
                        AND e.session_id = st.session_id AND e.class_id = st.class_id AND e.section_id = st.section_id
                        AND e.roll_no IS NOT DISTINCT FROM st.roll_no AND e.student_status = st.status) THEN
        RAISE EXCEPTION 'a student who left must have a closed last enrolment matching the student' USING ERRCODE = '23514';
    END IF;
END;
$$;

-- Students who already left (current enrolments since 1701) close their
-- year; the count goes to the audit event at the end.
CREATE TEMP TABLE m1702_closed ON COMMIT DROP AS
WITH closed AS (
    UPDATE student_enrolments
       SET state = 'closed', outcome = student_enrolment_outcome_for(student_status),
           ended_on = student_enrolment_end_date(session_id, started_on), updated_at = now()
     WHERE state = 'current' AND student_status <> 'active'
    RETURNING id)
SELECT count(*) AS n FROM closed;

-- Keeps the enrolments in step with the students row. Settings read for the
-- transaction (set_config(..., true)):
--   flybirds.actor_id           acting user for the change log;
--   flybirds.enrolment_outcome  outcome when a year with records closes by
--                               moving session (promoted, repeated);
--   flybirds.enrolment_source   source of the year that opens;
--   flybirds.enrolment_revert   'on' while reverting a promotion.
CREATE OR REPLACE FUNCTION students_enrolment_sync() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    cur student_enrolments%ROWTYPE;
    prev student_enrolments%ROWTYPE;
    actor uuid := nullif(current_setting('flybirds.actor_id', true), '')::uuid;
    outcome_set text := nullif(current_setting('flybirds.enrolment_outcome', true), '');
    source_set text := nullif(current_setting('flybirds.enrolment_source', true), '');
    reverting boolean := coalesce(current_setting('flybirds.enrolment_revert', true), '') = 'on';
    starts date;
    new_id uuid;
BEGIN
    SELECT GREATEST(start_date, NEW.admission_date) INTO starts FROM academic_sessions WHERE id = NEW.session_id;
    IF TG_OP = 'INSERT' THEN
        INSERT INTO student_enrolments (student_id, session_id, class_id, section_id, roll_no, state, student_status, outcome, source, started_on, ended_on)
        VALUES (NEW.id, NEW.session_id, NEW.class_id, NEW.section_id, NEW.roll_no,
                CASE WHEN NEW.status = 'active' THEN 'current' ELSE 'closed' END, NEW.status,
                student_enrolment_outcome_for(NEW.status), coalesce(source_set, 'admission'), starts,
                CASE WHEN NEW.status = 'active' THEN NULL ELSE student_enrolment_end_date(NEW.session_id, starts) END)
        RETURNING id INTO new_id;
        INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, to_class_id, to_section_id, to_value, actor_id)
        VALUES (new_id, NEW.id, 'enrolled', NEW.class_id, NEW.section_id, NEW.status, actor);
        RETURN NEW;
    END IF;

    -- The enrolment of the year the row pointed at.
    SELECT * INTO cur FROM student_enrolments WHERE student_id = NEW.id AND session_id = OLD.session_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'the student has no enrolment for their session' USING ERRCODE = '23514';
    END IF;

    IF NEW.session_id = OLD.session_id THEN
        IF NEW.class_id <> cur.class_id AND (
            EXISTS (SELECT 1 FROM exam_marks WHERE student_id = NEW.id AND session_id = cur.session_id AND class_id <> NEW.class_id)
            OR EXISTS (SELECT 1 FROM exam_results WHERE student_id = NEW.id AND session_id = cur.session_id AND class_id <> NEW.class_id)) THEN
            RAISE EXCEPTION 'the student has marks in another class this session; the class cannot change' USING ERRCODE = 'FB001';
        END IF;
        UPDATE student_enrolments
           SET class_id = NEW.class_id, section_id = NEW.section_id, roll_no = NEW.roll_no, student_status = NEW.status,
               state = CASE WHEN NEW.status = 'active' THEN 'current' ELSE 'closed' END,
               outcome = CASE WHEN NEW.status = 'active' THEN NULL ELSE student_enrolment_outcome_for(NEW.status) END,
               ended_on = CASE WHEN NEW.status = 'active' THEN NULL
                               WHEN cur.state = 'closed' THEN cur.ended_on
                               ELSE student_enrolment_end_date(cur.session_id, cur.started_on) END,
               updated_at = now()
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
            VALUES (cur.id, NEW.id, CASE WHEN cur.state = 'current' AND NEW.status <> 'active' THEN 'closed'
                                         WHEN cur.state = 'closed' AND NEW.status = 'active' THEN 'reopened'
                                         ELSE 'status_changed' END,
                    cur.student_status, NEW.status, actor);
        END IF;
        RETURN NEW;
    END IF;

    -- Another session.
    SELECT * INTO prev FROM student_enrolments WHERE student_id = NEW.id AND session_id = NEW.session_id FOR UPDATE;
    IF FOUND THEN
        -- Reverting a promotion: the year it opened has nothing recorded;
        -- remove it and reopen the year it closed.
        IF reverting AND cur.state = 'current' AND prev.state = 'closed' AND prev.outcome IN ('promoted', 'repeated')
           AND NEW.status = 'active' AND NOT student_has_session_records(NEW.id, cur.session_id) THEN
            DELETE FROM student_enrolments WHERE id = cur.id;
            UPDATE student_enrolments SET state = 'current', outcome = NULL, ended_on = NULL, class_id = NEW.class_id, section_id = NEW.section_id,
                   roll_no = NEW.roll_no, student_status = NEW.status, updated_at = now()
             WHERE id = prev.id;
            INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_value, to_value, actor_id)
            VALUES (prev.id, NEW.id, 'reverted', prev.outcome, NEW.status, actor);
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'the student already has an enrolment in that session' USING ERRCODE = 'FB001';
    END IF;

    IF cur.state = 'current' THEN
        IF NOT student_has_session_records(NEW.id, cur.session_id) THEN
            -- Nothing recorded that year: a correction in place.
            UPDATE student_enrolments SET session_id = NEW.session_id, class_id = NEW.class_id, section_id = NEW.section_id,
                   roll_no = NEW.roll_no, student_status = NEW.status,
                   state = CASE WHEN NEW.status = 'active' THEN 'current' ELSE 'closed' END,
                   outcome = student_enrolment_outcome_for(NEW.status), started_on = starts,
                   ended_on = CASE WHEN NEW.status = 'active' THEN NULL ELSE student_enrolment_end_date(NEW.session_id, starts) END,
                   updated_at = now()
             WHERE id = cur.id;
            INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_class_id, to_class_id, from_section_id, to_section_id,
                                                  from_value, to_value, actor_id)
            VALUES (cur.id, NEW.id, 'session_corrected', cur.class_id, NEW.class_id, cur.section_id, NEW.section_id,
                    cur.session_id::text, NEW.session_id::text, actor);
            RETURN NEW;
        END IF;
        IF outcome_set IS NULL OR outcome_set NOT IN ('promoted', 'repeated') OR NEW.status <> 'active' THEN
            RAISE EXCEPTION 'the student has records this year; use promotion to move them to another session' USING ERRCODE = 'FB002';
        END IF;
        UPDATE student_enrolments SET state = 'closed', outcome = outcome_set,
               ended_on = student_enrolment_end_date(cur.session_id, cur.started_on), updated_at = now()
         WHERE id = cur.id;
        INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, from_value, to_value, actor_id)
        VALUES (cur.id, NEW.id, 'closed', OLD.status, outcome_set, actor);
    ELSIF NEW.status <> 'active' THEN
        -- A student who left can only come back as active (readmission).
        RAISE EXCEPTION 'a student who left can only be readmitted as active' USING ERRCODE = 'FB002';
    END IF;

    INSERT INTO student_enrolments (student_id, session_id, class_id, section_id, roll_no, state, student_status, source, started_on)
    VALUES (NEW.id, NEW.session_id, NEW.class_id, NEW.section_id, NEW.roll_no, 'current', NEW.status,
            coalesce(source_set, CASE WHEN cur.state = 'current' THEN 'promotion' ELSE 'readmission' END), starts)
    RETURNING id INTO new_id;
    INSERT INTO student_enrolment_events (enrolment_id, student_id, kind, to_class_id, to_section_id, to_value, actor_id)
    VALUES (new_id, NEW.id, 'enrolled', NEW.class_id, NEW.section_id, NEW.status, actor);
    RETURN NEW;
END;
$$;

-- 3. Login follows withdrawal and transfer (QA-03). Only a login this
-- suspended is restored on readmission; one an administrator disabled
-- stays disabled.
ALTER TABLE students ADD COLUMN login_suspended_at timestamptz;

CREATE FUNCTION students_login_follow_status() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    actor uuid := nullif(current_setting('flybirds.actor_id', true), '')::uuid;
    n integer;
BEGIN
    IF NEW.user_id IS NULL THEN
        RETURN NULL;
    END IF;
    IF NEW.status IN ('inactive', 'transferred') AND OLD.status IN ('active', 'graduated') THEN
        UPDATE users SET is_active = false, updated_at = now() WHERE id = NEW.user_id AND is_active;
        GET DIAGNOSTICS n = ROW_COUNT;
        IF n > 0 THEN
            UPDATE auth_sessions SET revoked_at = now(), updated_at = now() WHERE user_id = NEW.user_id AND revoked_at IS NULL;
            UPDATE students SET login_suspended_at = now() WHERE id = NEW.id;
            INSERT INTO audit_events (actor_user_id, action, target_type, target_id, outcome, metadata)
            VALUES (actor, 'student.login_suspended', 'student', NEW.id::text, 'success', jsonb_build_object('user_id', NEW.user_id, 'status', NEW.status));
        END IF;
    ELSIF NEW.status IN ('active', 'graduated') AND OLD.status IN ('inactive', 'transferred') AND NEW.login_suspended_at IS NOT NULL THEN
        UPDATE users SET is_active = true, updated_at = now() WHERE id = NEW.user_id;
        UPDATE students SET login_suspended_at = NULL WHERE id = NEW.id;
        INSERT INTO audit_events (actor_user_id, action, target_type, target_id, outcome, metadata)
        VALUES (actor, 'student.login_restored', 'student', NEW.id::text, 'success', jsonb_build_object('user_id', NEW.user_id, 'status', NEW.status));
    END IF;
    RETURN NULL;
END;
$$;
CREATE TRIGGER students_login_status_trigger AFTER UPDATE OF status ON students
FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION students_login_follow_status();

-- 4. Class progression: the class students of a class are promoted to.
-- Empty means the top class: its students graduate.
ALTER TABLE academic_classes ADD COLUMN next_class_id uuid REFERENCES academic_classes(id) ON DELETE RESTRICT,
    ADD CONSTRAINT academic_classes_next_ck CHECK (next_class_id IS DISTINCT FROM id);

-- 5. Promotion batches: one class of one session moved to the next session.
-- draft → approved (head teacher, bound to approved_sha256) → applied
-- (administrator) → reverted (only while the new year has nothing recorded).
-- Any edit after approval returns the batch to draft.
CREATE TABLE promotion_batches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    from_session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    to_session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'approved', 'applied', 'reverted', 'discarded')),
    roll_policy text NOT NULL DEFAULT 'keep' CHECK (roll_policy IN ('keep', 'merit', 'name', 'manual')),
    approved_sha256 text CHECK (approved_sha256 ~ '^[0-9a-f]{64}$'),
    approved_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    approved_at timestamptz,
    applied_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    applied_at timestamptz,
    reverted_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    reverted_at timestamptz,
    revert_reason text NOT NULL DEFAULT '' CHECK (char_length(revert_reason) <= 500),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT promotion_batches_sessions_ck CHECK (from_session_id <> to_session_id),
    CONSTRAINT promotion_batches_approved_ck CHECK (status NOT IN ('approved', 'applied', 'reverted') OR approved_sha256 IS NOT NULL),
    CONSTRAINT promotion_batches_applied_ck CHECK (status NOT IN ('applied', 'reverted') OR applied_at IS NOT NULL),
    CONSTRAINT promotion_batches_reverted_ck CHECK ((status = 'reverted') = (reverted_at IS NOT NULL))
);
-- One open batch per class and year.
CREATE UNIQUE INDEX promotion_batches_open_key ON promotion_batches (from_session_id, class_id) WHERE status IN ('draft', 'approved', 'applied');
CREATE INDEX promotion_batches_list_idx ON promotion_batches (from_session_id, class_id, created_at DESC);

CREATE TABLE promotion_items (
    batch_id uuid NOT NULL REFERENCES promotion_batches(id) ON DELETE CASCADE,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    -- The year being closed, as it was when the batch was made.
    enrolment_id uuid NOT NULL REFERENCES student_enrolments(id) ON DELETE RESTRICT,
    source_section_id uuid NOT NULL REFERENCES academic_sections(id) ON DELETE RESTRICT,
    source_roll_no integer,
    -- The published annual result the recommendation used, if any.
    annual_version_id uuid REFERENCES result_versions(id) ON DELETE RESTRICT,
    annual_outcome text CHECK (annual_outcome IN ('passed', 'failed', 'withheld')),
    annual_class_position integer,
    recommendation text NOT NULL CHECK (recommendation IN ('promote', 'repeat', 'graduate', 'blocked')),
    decision text CHECK (decision IN ('promote', 'repeat', 'graduate', 'withdraw', 'transfer')),
    reason text NOT NULL DEFAULT '' CHECK (reason = btrim(reason) AND char_length(reason) <= 500),
    decided_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    decided_at timestamptz,
    target_class_id uuid REFERENCES academic_classes(id) ON DELETE RESTRICT,
    target_section_id uuid REFERENCES academic_sections(id) ON DELETE RESTRICT,
    target_roll_no integer CHECK (target_roll_no IS NULL OR target_roll_no BETWEEN 1 AND 999999),
    -- The year the apply opened (promote, repeat); cleared by a revert.
    applied_enrolment_id uuid REFERENCES student_enrolments(id) ON DELETE SET NULL,
    PRIMARY KEY (batch_id, student_id),
    -- A decision that differs from the recommendation, or replaces a blocked
    -- one, records why.
    CONSTRAINT promotion_items_reason_ck CHECK (decision IS NULL OR decision = recommendation OR reason <> '')
);
CREATE INDEX promotion_items_student_idx ON promotion_items (student_id);

-- 6. Fee concessions belong to a session. Existing concessions move to each
-- student's current session, so dues already generated are unchanged and
-- nothing carries into a new session by itself.
ALTER TABLE fee_concessions ADD COLUMN session_id uuid REFERENCES academic_sessions(id) ON DELETE RESTRICT;
UPDATE fee_concessions fc SET session_id = s.session_id FROM students s WHERE s.id = fc.student_id;
ALTER TABLE fee_concessions ALTER COLUMN session_id SET NOT NULL,
    DROP CONSTRAINT fee_concessions_key,
    ADD CONSTRAINT fee_concessions_key UNIQUE (student_id, session_id, category_id);
CREATE INDEX fee_concessions_session_idx ON fee_concessions (session_id, student_id);

-- 7. Permissions: the administrator prepares and applies; the head teacher
-- approves; both read.
INSERT INTO permissions (id, description) VALUES
    ('promotions:read', 'Read promotion batches and class progression'),
    ('promotions:manage', 'Prepare promotion batches, decisions, placement and class progression'),
    ('promotions:approve', 'Approve promotion batches'),
    ('promotions:apply', 'Apply and revert approved promotion batches')
ON CONFLICT (id) DO NOTHING;
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('admin', 'promotions:read'), ('admin', 'promotions:manage'), ('admin', 'promotions:apply'),
    ('head_teacher', 'promotions:read'), ('head_teacher', 'promotions:approve')
ON CONFLICT DO NOTHING;

INSERT INTO audit_events (action, target_type, target_id, outcome, metadata)
SELECT 'student.promotion_migrated', 'school', 'students', 'success',
       jsonb_build_object('closed_for_leavers', (SELECT n FROM m1702_closed),
                          'concessions', (SELECT count(*) FROM fee_concessions), 'migration', 1702)
HAVING (SELECT count(*) FROM students) > 0;
