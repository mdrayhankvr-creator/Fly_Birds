-- QA-08: when a student withdraws or transfers, the dues for the months after
-- the month they left are cancelled. Nothing is deleted:
--
-- * Each cancelled due gets a 'leaving_cancellation' adjustment for its whole
--   remaining balance (the balance becomes 0) and points at one
--   fee_due_cancellations row per leaving.
-- * Only monthly dues of later months with nothing paid are cancelled. The
--   month the student left and every month with a payment stay as they are.
-- * An administrator can revert a cancellation (with a reason): each due is
--   owed again through a 'cancellation_reverted' adjustment.
-- * The leaving date is the end date of the closed enrolment (the day of the
--   status change, or the session's last day if it was over).
-- * Leaving through a year-end promotion batch is leaving after the year:
--   nothing is cancelled.
CREATE TABLE fee_due_cancellations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    reason text NOT NULL CHECK (reason IN ('withdrawn', 'transferred')),
    left_on date NOT NULL,
    dues integer NOT NULL CHECK (dues > 0),
    amount numeric(12,2) NOT NULL CHECK (amount > 0),
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    reverted_at timestamptz,
    reverted_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    revert_reason text NOT NULL DEFAULT '' CHECK (revert_reason = btrim(revert_reason) AND octet_length(revert_reason) <= 240),
    CONSTRAINT fee_due_cancellations_revert_ck CHECK (
        (reverted_at IS NULL) = (reverted_by IS NULL) AND (reverted_at IS NULL) = (revert_reason = ''))
);
CREATE INDEX fee_due_cancellations_student_idx ON fee_due_cancellations (student_id, created_at DESC);
CREATE INDEX fee_due_cancellations_open_idx ON fee_due_cancellations (created_at DESC) WHERE reverted_at IS NULL;

ALTER TABLE student_fees ADD COLUMN cancellation_id uuid REFERENCES fee_due_cancellations(id) ON DELETE RESTRICT;
CREATE INDEX student_fees_cancellation_idx ON student_fees (cancellation_id) WHERE cancellation_id IS NOT NULL;

-- The trigger's cancellation has no acting user when the status changed
-- outside the application; every other adjustment keeps one.
ALTER TABLE fee_adjustments
    DROP CONSTRAINT fee_adjustments_kind_check,
    ADD CONSTRAINT fee_adjustments_kind_check CHECK (kind IN ('discount', 'waiver', 'leaving_cancellation', 'cancellation_reverted')),
    ADD COLUMN cancellation_id uuid REFERENCES fee_due_cancellations(id) ON DELETE RESTRICT,
    ADD CONSTRAINT fee_adjustments_cancellation_ck CHECK ((cancellation_id IS NOT NULL) = (kind IN ('leaving_cancellation', 'cancellation_reverted'))),
    ALTER COLUMN created_by DROP NOT NULL,
    ADD CONSTRAINT fee_adjustments_actor_ck CHECK (created_by IS NOT NULL OR kind = 'leaving_cancellation');

CREATE FUNCTION students_cancel_dues_on_leaving() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    actor uuid := nullif(current_setting('flybirds.actor_id', true), '')::uuid;
    leaving date;
    cid uuid;
    n integer;
    total numeric(12,2);
BEGIN
    IF NOT (NEW.status IN ('inactive', 'transferred') AND OLD.status NOT IN ('inactive', 'transferred')) THEN
        RETURN NULL;
    END IF;
    -- Leaving through a year-end promotion batch is leaving after the year
    -- (flybirds.enrolment_completed): the year's dues stay, as promotion
    -- changes no fee row.
    IF coalesce(current_setting('flybirds.enrolment_completed', true), '') = 'on' THEN
        RETURN NULL;
    END IF;
    SELECT ended_on INTO leaving FROM student_enrolments WHERE student_id = NEW.id AND session_id = NEW.session_id;
    leaving := coalesce(leaving, current_date);
    -- The same lock as every balance change of this student (fees package).
    PERFORM pg_advisory_xact_lock(802, hashtext(lower(NEW.id::text)));
    SELECT count(*), coalesce(sum(balance), 0) INTO n, total FROM student_fees
     WHERE student_id = NEW.id AND period > date_trunc('month', leaving)::date
       AND paid_amount = 0 AND balance > 0 AND cancellation_id IS NULL;
    IF n = 0 THEN
        RETURN NULL;
    END IF;
    INSERT INTO fee_due_cancellations (student_id, session_id, reason, left_on, dues, amount, created_by)
    VALUES (NEW.id, NEW.session_id, CASE NEW.status WHEN 'transferred' THEN 'transferred' ELSE 'withdrawn' END, leaving, n, total, actor)
    RETURNING id INTO cid;
    INSERT INTO fee_adjustments (student_fee_id, kind, amount, reason, created_by, cancellation_id)
    SELECT id, 'leaving_cancellation', balance, 'Cancelled: left on ' || to_char(leaving, 'YYYY-MM-DD'), actor, cid
      FROM student_fees
     WHERE student_id = NEW.id AND period > date_trunc('month', leaving)::date
       AND paid_amount = 0 AND balance > 0 AND cancellation_id IS NULL;
    UPDATE student_fees SET discount_amount = discount_amount + balance, cancellation_id = cid, updated_at = now()
     WHERE student_id = NEW.id AND period > date_trunc('month', leaving)::date
       AND paid_amount = 0 AND balance > 0 AND cancellation_id IS NULL;
    INSERT INTO audit_events (actor_user_id, action, target_type, target_id, outcome, metadata)
    VALUES (actor, 'fee.dues_cancelled_on_leaving', 'student', NEW.id::text, 'success',
            jsonb_build_object('cancellation_id', cid, 'reason', CASE NEW.status WHEN 'transferred' THEN 'transferred' ELSE 'withdrawn' END,
                               'left_on', to_char(leaving, 'YYYY-MM-DD'), 'dues', n, 'amount', total::text));
    RETURN NULL;
END
$$;

-- Runs after students_enrolment_update_trigger (triggers fire in name
-- order), which closes the enrolment with its end date.
CREATE TRIGGER students_fee_dues_on_leaving_trigger AFTER UPDATE OF status ON students
FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION students_cancel_dues_on_leaving();
