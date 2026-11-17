-- School-selected policy. No row is seeded: existing schools retain their
-- fourth-subject behavior until an administrator configures a class.
CREATE TABLE class_fourth_subject_policies (
    class_id uuid PRIMARY KEY REFERENCES academic_classes(id) ON DELETE RESTRICT,
    enabled boolean NOT NULL DEFAULT false,
    male_subject_id uuid REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    female_subject_id uuid REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    swap_group_id uuid REFERENCES academic_groups(id) ON DELETE RESTRICT,
    swap_subject_a_id uuid REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    swap_subject_b_id uuid REFERENCES academic_subjects(id) ON DELETE RESTRICT,
    updated_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT class_fourth_policy_gender_ck CHECK
      (NOT enabled OR (male_subject_id IS NOT NULL AND female_subject_id IS NOT NULL AND male_subject_id <> female_subject_id)),
    CONSTRAINT class_fourth_policy_swap_ck CHECK
      ((swap_group_id IS NULL AND swap_subject_a_id IS NULL AND swap_subject_b_id IS NULL)
       OR (swap_group_id IS NOT NULL AND swap_subject_a_id IS NOT NULL AND swap_subject_b_id IS NOT NULL
           AND swap_subject_a_id <> swap_subject_b_id)),
    CONSTRAINT class_fourth_policy_swap_gender_ck CHECK (NOT enabled OR swap_group_id IS NULL OR
      (swap_subject_a_id NOT IN (male_subject_id,female_subject_id) AND
       swap_subject_b_id NOT IN (male_subject_id,female_subject_id)))
);

-- An explicit choice wins. A configured gender default applies only when its
-- subject is mapped as fourth for the class and eligible for the student's
-- selected group. Unknown or other gender has no inferred choice.
CREATE FUNCTION student_effective_fourth_subject(p_student uuid, p_session uuid, p_class uuid)
RETURNS uuid LANGUAGE sql STABLE AS $$
    SELECT coalesce(
      (SELECT c.subject_id FROM student_subject_choices c
        WHERE c.student_id=p_student AND c.session_id=p_session AND c.role='optional'),
      (SELECT CASE st.gender WHEN 'male' THEN p.male_subject_id WHEN 'female' THEN p.female_subject_id END
         FROM class_fourth_subject_policies p JOIN students st ON st.id=p_student
         JOIN academic_class_subjects acs ON acs.class_id=p_class
           AND acs.subject_id=CASE st.gender WHEN 'male' THEN p.male_subject_id WHEN 'female' THEN p.female_subject_id END
           AND acs.is_fourth_subject
        WHERE p.class_id=p_class AND p.enabled
          AND (NOT (SELECT uses_groups FROM academic_classes WHERE id=p_class)
            OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs WHERE gs.class_id=p_class AND gs.subject_id=acs.subject_id)
            OR EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
                ON gc.group_id=gs.group_id AND gc.student_id=p_student AND gc.session_id=p_session
                WHERE gs.class_id=p_class AND gs.subject_id=acs.subject_id)))
    )
$$;

-- The second member of an approved pair is main when the first is selected
-- as fourth. Unchosen Agriculture/Home Science never becomes main.
CREATE OR REPLACE FUNCTION student_takes_subject(p_student uuid, p_session uuid, p_class uuid, p_subject uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce((SELECT
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
                         WHERE ch.session_id=p_session AND e.class_id=p_class)
             OR EXISTS (SELECT 1 FROM student_subject_choices ch
                        WHERE ch.student_id=p_student AND ch.session_id=p_session AND ch.subject_id=p_subject)
      END
      FROM academic_class_subjects acs JOIN academic_classes c ON c.id=acs.class_id
      LEFT JOIN class_fourth_subject_policies p ON p.class_id=acs.class_id
      WHERE acs.class_id=p_class AND acs.subject_id=p_subject),false)
$$;

CREATE FUNCTION student_subject_is_fourth(p_student uuid, p_session uuid, p_class uuid, p_subject uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT coalesce((SELECT CASE WHEN coalesce(p.enabled,false)
      THEN acs.is_fourth_subject AND p_subject=student_effective_fourth_subject(p_student,p_session,p_class)
      ELSE acs.is_fourth_subject END
      FROM academic_class_subjects acs LEFT JOIN class_fourth_subject_policies p ON p.class_id=acs.class_id
      WHERE acs.class_id=p_class AND acs.subject_id=p_subject),false)
$$;

-- A published result is a versioned snapshot. Changing the source choice
-- afterwards must go through a future reviewed recalculation workflow.
CREATE FUNCTION block_published_subject_choice_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; sess uuid;
BEGIN
  IF TG_OP='DELETE' THEN
    sid := OLD.student_id;
    sess := OLD.session_id;
  ELSE
    sid := NEW.student_id;
    sess := NEW.session_id;
  END IF;
  IF TG_OP='UPDATE' AND NEW.subject_id=OLD.subject_id THEN RETURN NEW; END IF;
  IF EXISTS (SELECT 1 FROM result_versions v JOIN class_fourth_subject_policies p ON p.class_id=v.class_id AND p.enabled
             WHERE v.student_id=sid AND v.session_id=sess)
     AND coalesce(current_setting('app.subject_choice_review',true),'') <> 'on' THEN
    RAISE EXCEPTION 'published result review required before changing fourth-subject choice' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER student_subject_choices_published_guard
BEFORE INSERT OR UPDATE OR DELETE ON student_subject_choices
FOR EACH ROW EXECUTE FUNCTION block_published_subject_choice_change();

CREATE FUNCTION block_published_policy_context_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; sess uuid; cls uuid;
BEGIN
  IF TG_TABLE_NAME='students' THEN
    IF NEW.gender=OLD.gender THEN RETURN NEW; END IF;
    sid := NEW.id;
    sess := NEW.session_id;
    cls := NEW.class_id;
    -- Gender is stored on the person, while default choices are read for a
    -- session. A later promotion must not change an older published result's
    -- effective input through a gender edit in the new session.
    IF EXISTS (SELECT 1 FROM result_versions v JOIN class_fourth_subject_policies p
               ON p.class_id=v.class_id AND p.enabled WHERE v.student_id=NEW.id) THEN
      RAISE EXCEPTION 'published result review required before changing fourth-subject context' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
  ELSE
    IF TG_OP='DELETE' THEN sid := OLD.student_id; sess := OLD.session_id;
    ELSE sid := NEW.student_id; sess := NEW.session_id; END IF;
    IF TG_OP='UPDATE' AND NEW.group_id=OLD.group_id THEN RETURN NEW; END IF;
    SELECT class_id INTO cls FROM student_enrolments WHERE student_id=sid AND session_id=sess LIMIT 1;
  END IF;
  IF EXISTS (SELECT 1 FROM class_fourth_subject_policies WHERE class_id=cls AND enabled)
     AND EXISTS (SELECT 1 FROM result_versions WHERE student_id=sid AND session_id=sess) THEN
    RAISE EXCEPTION 'published result review required before changing fourth-subject context' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER students_fourth_policy_published_guard BEFORE UPDATE OF gender ON students
FOR EACH ROW EXECUTE FUNCTION block_published_policy_context_change();
CREATE TRIGGER student_group_choices_fourth_policy_published_guard
BEFORE INSERT OR UPDATE OR DELETE ON student_group_choices
FOR EACH ROW EXECUTE FUNCTION block_published_policy_context_change();

CREATE FUNCTION guard_fourth_policy_mappings() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME='academic_class_subjects' THEN
    IF TG_OP='UPDATE' AND NEW.class_id=OLD.class_id AND NEW.subject_id=OLD.subject_id AND NEW.is_fourth_subject=OLD.is_fourth_subject THEN RETURN NEW; END IF;
    IF EXISTS (SELECT 1 FROM class_fourth_subject_policies p WHERE p.class_id=OLD.class_id AND p.enabled
      AND OLD.subject_id IN (p.male_subject_id,p.female_subject_id,p.swap_subject_a_id,p.swap_subject_b_id))
      OR EXISTS (SELECT 1 FROM student_enrolments en JOIN student_subject_choices ch
        ON ch.student_id=en.student_id AND ch.session_id=en.session_id AND ch.subject_id=OLD.subject_id AND ch.role='optional'
        JOIN class_fourth_subject_policies p ON p.class_id=en.class_id AND p.enabled WHERE en.class_id=OLD.class_id) THEN
      RAISE EXCEPTION 'disable fourth-subject policy before changing a referenced class mapping' USING ERRCODE='23514';
    END IF;
  ELSIF TG_TABLE_NAME='class_group_subjects' THEN
    IF TG_OP <> 'INSERT' THEN
      IF EXISTS (SELECT 1 FROM class_fourth_subject_policies p WHERE p.class_id=OLD.class_id AND p.enabled
        AND OLD.subject_id IN (p.male_subject_id,p.female_subject_id,p.swap_subject_a_id,p.swap_subject_b_id))
        OR EXISTS (SELECT 1 FROM student_enrolments en JOIN student_subject_choices ch
          ON ch.student_id=en.student_id AND ch.session_id=en.session_id AND ch.subject_id=OLD.subject_id AND ch.role='optional'
          JOIN class_fourth_subject_policies p ON p.class_id=en.class_id AND p.enabled WHERE en.class_id=OLD.class_id) THEN
        RAISE EXCEPTION 'disable fourth-subject policy before changing a referenced group mapping' USING ERRCODE='23514';
      END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
      IF EXISTS (SELECT 1 FROM class_fourth_subject_policies p WHERE p.class_id=NEW.class_id AND p.enabled
        AND NEW.subject_id IN (p.male_subject_id,p.female_subject_id,p.swap_subject_a_id,p.swap_subject_b_id))
        OR EXISTS (SELECT 1 FROM student_enrolments en JOIN student_subject_choices ch
          ON ch.student_id=en.student_id AND ch.session_id=en.session_id AND ch.subject_id=NEW.subject_id AND ch.role='optional'
          JOIN class_fourth_subject_policies p ON p.class_id=en.class_id AND p.enabled WHERE en.class_id=NEW.class_id) THEN
        RAISE EXCEPTION 'disable fourth-subject policy before changing a referenced group mapping' USING ERRCODE='23514';
      END IF;
    END IF;
  ELSIF TG_TABLE_NAME='academic_subjects' THEN
    IF OLD.is_active AND NOT NEW.is_active AND EXISTS (SELECT 1 FROM class_fourth_subject_policies p WHERE p.enabled
      AND OLD.id IN (p.male_subject_id,p.female_subject_id,p.swap_subject_a_id,p.swap_subject_b_id)) THEN
      RAISE EXCEPTION 'disable fourth-subject policy before deactivating a referenced subject' USING ERRCODE='23514';
    END IF;
    IF OLD.is_active AND NOT NEW.is_active AND EXISTS (SELECT 1 FROM student_enrolments en JOIN student_subject_choices ch
      ON ch.student_id=en.student_id AND ch.session_id=en.session_id AND ch.subject_id=OLD.id AND ch.role='optional'
      JOIN class_fourth_subject_policies p ON p.class_id=en.class_id AND p.enabled) THEN
      RAISE EXCEPTION 'review explicit fourth-subject choices before deactivating their subject' USING ERRCODE='23514';
    END IF;
  ELSIF TG_TABLE_NAME='academic_groups' THEN
    IF OLD.is_active AND NOT NEW.is_active AND EXISTS (SELECT 1 FROM class_fourth_subject_policies p WHERE p.enabled AND p.swap_group_id=OLD.id) THEN
      RAISE EXCEPTION 'disable fourth-subject policy before deactivating a referenced group' USING ERRCODE='23514';
    END IF;
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER academic_class_subjects_fourth_policy_guard BEFORE UPDATE OR DELETE ON academic_class_subjects
FOR EACH ROW EXECUTE FUNCTION guard_fourth_policy_mappings();
CREATE TRIGGER class_group_subjects_fourth_policy_guard BEFORE INSERT OR UPDATE OR DELETE ON class_group_subjects
FOR EACH ROW EXECUTE FUNCTION guard_fourth_policy_mappings();
CREATE TRIGGER academic_subjects_fourth_policy_guard BEFORE UPDATE OF is_active ON academic_subjects
FOR EACH ROW EXECUTE FUNCTION guard_fourth_policy_mappings();
CREATE TRIGGER academic_groups_fourth_policy_guard BEFORE UPDATE OF is_active ON academic_groups
FOR EACH ROW EXECUTE FUNCTION guard_fourth_policy_mappings();

-- Group edits can happen in the same transaction as an administrator's choice
-- edit. Check the final state at commit so the pair remains consistent.
CREATE FUNCTION validate_policy_choice_group() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sid uuid; sess uuid;
BEGIN
  IF TG_OP='DELETE' THEN sid := OLD.student_id; sess := OLD.session_id;
  ELSE sid := NEW.student_id; sess := NEW.session_id; END IF;
  IF EXISTS (SELECT 1 FROM student_subject_choices ch JOIN student_enrolments en
    ON en.student_id=ch.student_id AND en.session_id=ch.session_id
    JOIN class_fourth_subject_policies p ON p.class_id=en.class_id AND p.enabled
    WHERE ch.student_id=sid AND ch.session_id=sess AND ch.role='optional'
      AND NOT student_takes_subject(sid,sess,en.class_id,ch.subject_id)) THEN
    RAISE EXCEPTION 'group change would make an explicit fourth-subject choice ineligible' USING ERRCODE='23514';
  END IF;
  RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER student_group_choices_fourth_policy_choice_guard
AFTER INSERT OR UPDATE OR DELETE ON student_group_choices DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION validate_policy_choice_group();
