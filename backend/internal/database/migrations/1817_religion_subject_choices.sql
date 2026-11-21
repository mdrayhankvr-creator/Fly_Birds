-- Opt-in academic course choices, not religious affiliation. No school rows
-- or student choices are inserted by this migration.
CREATE TABLE class_religion_subjects (
    class_id uuid NOT NULL,
    subject_id uuid NOT NULL,
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (class_id, subject_id),
    CONSTRAINT class_religion_subject_mapping_fk FOREIGN KEY (class_id, subject_id)
        REFERENCES academic_class_subjects(class_id, subject_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);

ALTER TABLE student_subject_choices DROP CONSTRAINT student_subject_choices_role_check;
ALTER TABLE student_subject_choices ADD CONSTRAINT student_subject_choices_role_check
    CHECK (role IN ('optional', 'religion'));

-- One choice of each role per student/session is already enforced by the
-- original primary key. Religion choices must match the session's enrolment
-- and an explicitly configured, active required mapping.
CREATE FUNCTION validate_religion_choice() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cls uuid; eligible boolean;
BEGIN
  IF TG_OP = 'UPDATE' AND (NEW.student_id,NEW.session_id,NEW.role) IS DISTINCT FROM
                         (OLD.student_id,OLD.session_id,OLD.role) THEN
    RAISE EXCEPTION 'subject choice identity cannot change' USING ERRCODE='23514';
  END IF;
  IF TG_OP = 'DELETE' THEN
    IF OLD.role = 'religion' AND EXISTS (SELECT 1 FROM result_versions
      WHERE student_id=OLD.student_id AND session_id=OLD.session_id) THEN
      RAISE EXCEPTION 'published result review required before changing Religion Education choice' USING ERRCODE='23514';
    END IF;
    RETURN OLD;
  END IF;
  IF TG_OP = 'UPDATE' AND OLD.role='religion' AND NEW.subject_id IS DISTINCT FROM OLD.subject_id
    AND EXISTS (SELECT 1 FROM result_versions WHERE student_id=OLD.student_id AND session_id=OLD.session_id) THEN
    RAISE EXCEPTION 'published result review required before changing Religion Education choice' USING ERRCODE='23514';
  END IF;
  IF NEW.role <> 'religion' THEN RETURN NEW; END IF;
  IF TG_OP = 'UPDATE' AND NEW.subject_id = OLD.subject_id THEN RETURN NEW; END IF;
  IF EXISTS (SELECT 1 FROM result_versions WHERE student_id=NEW.student_id AND session_id=NEW.session_id) THEN
    RAISE EXCEPTION 'published result review required before changing Religion Education choice' USING ERRCODE='23514';
  END IF;
  SELECT en.class_id INTO cls FROM student_enrolments en
    WHERE en.student_id=NEW.student_id AND en.session_id=NEW.session_id;
  IF cls IS NULL THEN RAISE EXCEPTION 'Religion Education choice requires an enrolment' USING ERRCODE='23514'; END IF;
  SELECT EXISTS (SELECT 1 FROM class_religion_subjects cr
    JOIN academic_class_subjects acs ON acs.class_id=cr.class_id AND acs.subject_id=cr.subject_id
    JOIN academic_subjects sub ON sub.id=cr.subject_id
    JOIN academic_classes c ON c.id=cr.class_id
    WHERE cr.class_id=cls AND cr.subject_id=NEW.subject_id AND acs.is_required AND NOT acs.is_fourth_subject
      AND sub.is_active AND c.is_active AND
      (NOT c.uses_groups OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs
        WHERE gs.class_id=cls AND gs.subject_id=NEW.subject_id) OR
       EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
         ON gc.group_id=gs.group_id AND gc.student_id=NEW.student_id AND gc.session_id=NEW.session_id
         WHERE gs.class_id=cls AND gs.subject_id=NEW.subject_id))) INTO eligible;
  IF NOT eligible THEN RAISE EXCEPTION 'Religion Education subject is not eligible for this enrolment' USING ERRCODE='23514'; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER student_religion_choice_guard BEFORE INSERT OR UPDATE OR DELETE ON student_subject_choices
FOR EACH ROW EXECUTE FUNCTION validate_religion_choice();

CREATE FUNCTION guard_religion_option() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cls uuid; sub uuid;
BEGIN
  IF TG_OP='UPDATE' THEN RAISE EXCEPTION 'replace Religion Education options by explicit remove and add' USING ERRCODE='23514'; END IF;
  IF TG_OP='DELETE' THEN cls:=OLD.class_id; sub:=OLD.subject_id;
  ELSE cls:=NEW.class_id; sub:=NEW.subject_id; END IF;
  IF EXISTS (SELECT 1 FROM result_versions WHERE class_id=cls) THEN
    RAISE EXCEPTION 'published result review required before changing Religion Education options' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN
    IF EXISTS (SELECT 1 FROM student_subject_choices ch JOIN student_enrolments en
      ON en.student_id=ch.student_id AND en.session_id=ch.session_id
      WHERE en.class_id=cls AND ch.subject_id=sub AND ch.role='religion') THEN
      RAISE EXCEPTION 'Religion Education option has student choices' USING ERRCODE='23514';
    END IF;
    RETURN OLD;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM academic_class_subjects acs JOIN academic_subjects s ON s.id=acs.subject_id
    JOIN academic_classes c ON c.id=acs.class_id
    WHERE acs.class_id=cls AND acs.subject_id=sub AND acs.is_required AND NOT acs.is_fourth_subject
      AND s.is_active AND c.is_active) THEN
    RAISE EXCEPTION 'Religion Education option needs an active required class subject' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER class_religion_option_guard BEFORE INSERT OR UPDATE OR DELETE ON class_religion_subjects
FOR EACH ROW EXECUTE FUNCTION guard_religion_option();

CREATE FUNCTION guard_religion_mapping() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME='academic_class_subjects' THEN
    IF TG_OP='DELETE' THEN
      IF EXISTS (SELECT 1 FROM class_religion_subjects WHERE class_id=OLD.class_id AND subject_id=OLD.subject_id) THEN
        RAISE EXCEPTION 'remove Religion Education option before changing its mapping' USING ERRCODE='23514';
      END IF;
      RETURN OLD;
    END IF;
    IF NEW.class_id IS DISTINCT FROM OLD.class_id OR NEW.subject_id IS DISTINCT FROM OLD.subject_id
      OR NEW.is_required IS DISTINCT FROM OLD.is_required OR NEW.is_fourth_subject IS DISTINCT FROM OLD.is_fourth_subject THEN
      IF EXISTS (SELECT 1 FROM class_religion_subjects WHERE class_id=OLD.class_id AND subject_id=OLD.subject_id) THEN
        RAISE EXCEPTION 'remove Religion Education option before changing its mapping' USING ERRCODE='23514';
      END IF;
    END IF;
  ELSIF OLD.is_active AND NOT NEW.is_active AND
    EXISTS (SELECT 1 FROM class_religion_subjects WHERE subject_id=OLD.id) THEN
    RAISE EXCEPTION 'remove Religion Education option before deactivating its subject' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER academic_class_subjects_religion_guard BEFORE UPDATE OR DELETE ON academic_class_subjects
FOR EACH ROW EXECUTE FUNCTION guard_religion_mapping();
CREATE TRIGGER academic_subjects_religion_guard BEFORE UPDATE OF is_active ON academic_subjects
FOR EACH ROW EXECUTE FUNCTION guard_religion_mapping();

CREATE FUNCTION guard_religion_group_mapping() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF EXISTS (SELECT 1 FROM class_religion_subjects cr
      WHERE cr.class_id=NEW.class_id AND cr.subject_id=NEW.subject_id) THEN
      RAISE EXCEPTION 'review Religion Education choices before changing group eligibility' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  END IF;
  IF EXISTS (SELECT 1 FROM class_religion_subjects cr
    WHERE cr.class_id=OLD.class_id AND cr.subject_id=OLD.subject_id) THEN
    RAISE EXCEPTION 'review Religion Education choices before changing group eligibility' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  IF EXISTS (SELECT 1 FROM class_religion_subjects cr
    WHERE cr.class_id=NEW.class_id AND cr.subject_id=NEW.subject_id) THEN
    RAISE EXCEPTION 'review Religion Education choices before changing group eligibility' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER class_group_subjects_religion_guard BEFORE INSERT OR UPDATE OR DELETE ON class_group_subjects
FOR EACH ROW EXECUTE FUNCTION guard_religion_group_mapping();

CREATE FUNCTION validate_religion_group_context() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; sess uuid;
BEGIN
  IF TG_OP='DELETE' THEN sid:=OLD.student_id; sess:=OLD.session_id;
  ELSE sid:=NEW.student_id; sess:=NEW.session_id; END IF;
  IF TG_OP='UPDATE' AND NEW.group_id=OLD.group_id THEN RETURN NULL; END IF;
  IF EXISTS (SELECT 1 FROM student_subject_choices ch WHERE ch.student_id=sid AND ch.session_id=sess AND ch.role='religion') THEN
    IF EXISTS (SELECT 1 FROM result_versions WHERE student_id=sid AND session_id=sess) THEN
      RAISE EXCEPTION 'published result review required before changing Religion Education group' USING ERRCODE='23514';
    END IF;
    IF EXISTS (SELECT 1 FROM student_subject_choices ch JOIN student_enrolments en
      ON en.student_id=ch.student_id AND en.session_id=ch.session_id
      JOIN academic_classes c ON c.id=en.class_id
      WHERE ch.student_id=sid AND ch.session_id=sess AND ch.role='religion' AND c.uses_groups
        AND EXISTS (SELECT 1 FROM class_group_subjects gs WHERE gs.class_id=en.class_id AND gs.subject_id=ch.subject_id)
        AND NOT EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
          ON gc.group_id=gs.group_id AND gc.student_id=sid AND gc.session_id=sess
          WHERE gs.class_id=en.class_id AND gs.subject_id=ch.subject_id)) THEN
      RAISE EXCEPTION 'group change makes Religion Education choice ineligible' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER student_group_choices_religion_guard
AFTER INSERT OR UPDATE OR DELETE ON student_group_choices DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_religion_group_context();

-- Session/class correction must explicitly review any existing academic
-- choice; it cannot silently attach the old course to a new class.
CREATE FUNCTION guard_religion_enrolment_move() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF (NEW.class_id,NEW.session_id) IS DISTINCT FROM (OLD.class_id,OLD.session_id)
    AND EXISTS (SELECT 1 FROM student_subject_choices ch
      WHERE ch.student_id=OLD.student_id AND ch.session_id=OLD.session_id AND ch.role='religion') THEN
    RAISE EXCEPTION 'review Religion Education choice before changing enrolment class/session' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER student_enrolments_religion_guard BEFORE UPDATE OF class_id,session_id ON student_enrolments
FOR EACH ROW EXECUTE FUNCTION guard_religion_enrolment_move();

-- A family can ask for a course correction without changing an academic
-- choice. The Admin must review the request and use the normal confirmed
-- choice workflow; requests carry no religious affiliation or free text.
CREATE TABLE student_religion_choice_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    proposed_subject_id uuid NOT NULL REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    requested_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','resolved','rejected')),
    reviewed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz,
    CONSTRAINT religion_request_review_ck CHECK ((status='pending')=(reviewed_at IS NULL)),
    CONSTRAINT religion_request_reviewer_ck CHECK ((status='pending')=(reviewed_by IS NULL))
);
CREATE UNIQUE INDEX religion_request_one_pending ON student_religion_choice_requests(student_id,session_id)
WHERE status='pending';
CREATE INDEX religion_request_status_idx ON student_religion_choice_requests(status,created_at DESC);

-- The existing marks/result engine asks this function which class subjects
-- apply. Explicit Religion Education options do not appear until chosen.
CREATE OR REPLACE FUNCTION student_takes_subject(p_student uuid, p_session uuid, p_class uuid, p_subject uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN EXISTS (SELECT 1 FROM class_religion_subjects cr
      WHERE cr.class_id=p_class AND cr.subject_id=p_subject)
    THEN EXISTS (SELECT 1 FROM student_subject_choices ch
      JOIN academic_classes c ON c.id=p_class
      WHERE ch.student_id=p_student AND ch.session_id=p_session AND ch.role='religion' AND ch.subject_id=p_subject
        AND (NOT c.uses_groups OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs
          WHERE gs.class_id=p_class AND gs.subject_id=p_subject)
          OR EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
            ON gc.group_id=gs.group_id AND gc.student_id=p_student AND gc.session_id=p_session
            WHERE gs.class_id=p_class AND gs.subject_id=p_subject)))
      AND EXISTS (SELECT 1 FROM student_enrolments en
        WHERE en.student_id=p_student AND en.session_id=p_session AND en.class_id=p_class)
    ELSE coalesce((SELECT
      (NOT c.uses_groups
       OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs WHERE gs.class_id=p_class AND gs.subject_id=p_subject)
       OR EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
          ON gc.group_id=gs.group_id AND gc.student_id=p_student AND gc.session_id=p_session
          WHERE gs.class_id=p_class AND gs.subject_id=p_subject))
      AND CASE
        WHEN acs.is_required THEN true
        WHEN coalesce(p.enabled,false) THEN
          acs.subject_id=student_effective_fourth_subject(p_student,p_session,p_class)
          OR (p.swap_group_id IS NOT NULL
              AND acs.subject_id IN (p.swap_subject_a_id,p.swap_subject_b_id)
              AND student_effective_fourth_subject(p_student,p_session,p_class) IN (p.swap_subject_a_id,p.swap_subject_b_id)
              AND EXISTS (SELECT 1 FROM student_group_choices gc
                          WHERE gc.student_id=p_student AND gc.session_id=p_session AND gc.group_id=p.swap_group_id))
        ELSE NOT EXISTS (SELECT 1 FROM student_subject_choices ch
                         JOIN student_enrolments e ON e.student_id=ch.student_id AND e.session_id=ch.session_id
                         WHERE ch.session_id=p_session AND ch.role='optional' AND e.class_id=p_class)
             OR EXISTS (SELECT 1 FROM student_subject_choices ch
                        WHERE ch.student_id=p_student AND ch.session_id=p_session AND ch.role='optional' AND ch.subject_id=p_subject)
      END
      FROM academic_class_subjects acs JOIN academic_classes c ON c.id=acs.class_id
      LEFT JOIN class_fourth_subject_policies p ON p.class_id=acs.class_id
      WHERE acs.class_id=p_class AND acs.subject_id=p_subject),false) END
$$;

-- Cohort lessons extend the existing class planner publication, rather than
-- duplicating a teacher across section routine rows. Old published rows stay
-- for attendance and version history; only their current flag can change.
CREATE TABLE class_cohort_lessons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_version_id uuid NOT NULL REFERENCES class_routine_plan_versions(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    class_id uuid NOT NULL REFERENCES academic_classes(id) ON DELETE RESTRICT,
    subject_id uuid NOT NULL,
    teacher_id uuid NOT NULL REFERENCES employees(id) ON DELETE RESTRICT,
    day_of_week integer NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_minute integer NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute integer NOT NULL CHECK (end_minute BETWEEN 1 AND 1440 AND end_minute > start_minute),
    room text NOT NULL CHECK (room=btrim(room) AND room<>'' AND octet_length(room)<=80),
    is_current boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT class_cohort_subject_fk FOREIGN KEY (class_id,subject_id)
      REFERENCES academic_class_subjects(class_id,subject_id) ON DELETE RESTRICT,
    CONSTRAINT class_cohort_teacher_assignment_fk FOREIGN KEY (teacher_id,session_id,class_id,subject_id)
      REFERENCES teacher_assignments(employee_id,session_id,class_id,subject_id) ON DELETE RESTRICT
);
CREATE INDEX class_cohort_current_idx ON class_cohort_lessons(session_id,day_of_week,start_minute)
  WHERE is_current;
CREATE INDEX class_cohort_teacher_idx ON class_cohort_lessons(teacher_id,day_of_week,start_minute)
  WHERE is_current;

CREATE TABLE class_cohort_members (
    lesson_id uuid NOT NULL REFERENCES class_cohort_lessons(id) ON DELETE RESTRICT,
    student_id uuid NOT NULL REFERENCES students(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL,
    class_id uuid NOT NULL,
    section_id uuid NOT NULL,
    PRIMARY KEY (lesson_id,student_id),
    CONSTRAINT class_cohort_member_enrolment_fk FOREIGN KEY (student_id,session_id)
      REFERENCES student_enrolments(student_id,session_id) ON DELETE RESTRICT,
    CONSTRAINT class_cohort_member_section_fk FOREIGN KEY (section_id,class_id,session_id)
      REFERENCES academic_sections(id,class_id,session_id) ON UPDATE RESTRICT ON DELETE RESTRICT
);
CREATE INDEX class_cohort_member_student_idx ON class_cohort_members(student_id,session_id,lesson_id);

CREATE TABLE class_cohort_attendance (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    lesson_id uuid NOT NULL,
    student_id uuid NOT NULL,
    attendance_date date NOT NULL,
    status text NOT NULL CHECK (status IN ('present','absent','late')),
    recorded_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT class_cohort_attendance_member_fk FOREIGN KEY (lesson_id,student_id)
      REFERENCES class_cohort_members(lesson_id,student_id) ON DELETE RESTRICT,
    CONSTRAINT class_cohort_attendance_unique UNIQUE (lesson_id,student_id,attendance_date)
);
CREATE INDEX class_cohort_attendance_date_idx ON class_cohort_attendance(attendance_date,lesson_id);

CREATE FUNCTION guard_class_cohort_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' THEN
    IF coalesce(current_setting('app.class_planner_publish',true),'')<>'on' THEN
      RAISE EXCEPTION 'cohort lesson must use versioned planner publication' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  END IF;
  IF TG_OP='UPDATE' AND coalesce(current_setting('app.class_planner_publish',true),'')='on'
    AND OLD.is_current AND NOT NEW.is_current AND
    (to_jsonb(NEW)-'is_current')=(to_jsonb(OLD)-'is_current') THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'published cohort lesson is immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER class_cohort_lessons_guard BEFORE INSERT OR UPDATE OR DELETE ON class_cohort_lessons
FOR EACH ROW EXECUTE FUNCTION guard_class_cohort_publication();

CREATE FUNCTION guard_class_cohort_member() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='INSERT' AND coalesce(current_setting('app.class_planner_publish',true),'')='on' THEN
    IF NOT EXISTS (SELECT 1 FROM class_cohort_lessons l JOIN student_enrolments en
      ON en.student_id=NEW.student_id AND en.session_id=l.session_id AND en.class_id=l.class_id
      AND en.section_id=NEW.section_id AND en.state='current' AND en.student_status='active'
      WHERE l.id=NEW.lesson_id AND l.session_id=NEW.session_id AND l.class_id=NEW.class_id
        AND student_takes_subject(NEW.student_id,l.session_id,l.class_id,l.subject_id)) THEN
      RAISE EXCEPTION 'cohort member must be an eligible current enrolment' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'published cohort roster is immutable' USING ERRCODE='23514';
END $$;
CREATE TRIGGER class_cohort_members_guard BEFORE INSERT OR UPDATE OR DELETE ON class_cohort_members
FOR EACH ROW EXECUTE FUNCTION guard_class_cohort_member();

-- Choice and enrolment edits in other modules must not silently invalidate
-- an already published current lesson. Publish a reviewed replacement first.
CREATE FUNCTION guard_current_cohort_student_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; sess uuid;
BEGIN
  IF TG_OP='INSERT' THEN sid:=NEW.student_id; sess:=NEW.session_id;
  ELSE sid:=OLD.student_id; sess:=OLD.session_id; END IF;
  IF TG_OP='UPDATE' THEN
    IF TG_TABLE_NAME='student_subject_choices' THEN
      IF NEW.subject_id IS NOT DISTINCT FROM OLD.subject_id THEN RETURN NEW; END IF;
    ELSIF TG_TABLE_NAME='student_enrolments' THEN
      IF (NEW.class_id,NEW.section_id,NEW.session_id,NEW.state,NEW.student_status)
        IS NOT DISTINCT FROM (OLD.class_id,OLD.section_id,OLD.session_id,OLD.state,OLD.student_status) THEN RETURN NEW; END IF;
    END IF;
  END IF;
  IF EXISTS (SELECT 1 FROM class_cohort_members m JOIN class_cohort_lessons l ON l.id=m.lesson_id
    WHERE m.student_id=sid AND m.session_id=sess AND l.is_current) THEN
    RAISE EXCEPTION 'publish a replacement cohort plan before changing a member choice or enrolment' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER student_choice_current_cohort_guard BEFORE INSERT OR UPDATE OR DELETE ON student_subject_choices
FOR EACH ROW EXECUTE FUNCTION guard_current_cohort_student_change();
CREATE TRIGGER student_enrolment_current_cohort_guard BEFORE UPDATE OF class_id,section_id,session_id,state,student_status OR DELETE ON student_enrolments
FOR EACH ROW EXECUTE FUNCTION guard_current_cohort_student_change();

CREATE FUNCTION guard_current_cohort_room_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE changed boolean;
BEGIN
  IF TG_OP='DELETE' THEN changed:=true;
  ELSE changed:=NEW.name IS DISTINCT FROM OLD.name OR NEW.is_active IS DISTINCT FROM OLD.is_active
    OR NEW.capacity IS DISTINCT FROM OLD.capacity; END IF;
  IF changed THEN
    IF EXISTS (SELECT 1 FROM class_cohort_lessons l WHERE l.is_current AND lower(l.room)=lower(OLD.name)) THEN
      RAISE EXCEPTION 'publish a replacement cohort plan before changing a used room' USING ERRCODE='23514';
    END IF;
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER exam_room_current_cohort_guard BEFORE UPDATE OR DELETE ON exam_rooms
FOR EACH ROW EXECUTE FUNCTION guard_current_cohort_room_change();
