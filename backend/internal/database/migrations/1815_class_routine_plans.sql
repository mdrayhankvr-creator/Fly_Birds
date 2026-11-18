-- Class routine plans are opt-in. Existing class_routines remain the current
-- timetable until an administrator explicitly publishes a replacement.
CREATE TABLE class_routine_plan_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id uuid NOT NULL REFERENCES academic_sessions(id) ON DELETE RESTRICT,
    version integer NOT NULL CHECK (version >= 0),
    state text NOT NULL CHECK (state IN ('draft','published','legacy')),
    plan jsonb NOT NULL CHECK (jsonb_typeof(plan) = 'object'),
    created_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    CONSTRAINT class_routine_plan_version_key UNIQUE (session_id,version),
    CONSTRAINT class_routine_plan_publication_ck CHECK ((state = 'published') = (published_at IS NOT NULL))
);
CREATE UNIQUE INDEX class_routine_plan_one_draft ON class_routine_plan_versions(session_id) WHERE state = 'draft';
CREATE INDEX class_routine_plan_session_idx ON class_routine_plan_versions(session_id,version DESC);

CREATE FUNCTION guard_published_class_routine() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_session uuid; new_session uuid;
BEGIN
  IF coalesce(current_setting('app.class_planner_publish',true),'') = 'on' THEN
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP <> 'INSERT' THEN old_session := OLD.session_id; END IF;
  IF TG_OP <> 'DELETE' THEN new_session := NEW.session_id; END IF;
  IF EXISTS (SELECT 1 FROM class_routine_plan_versions WHERE state IN ('draft','published')
             AND session_id IN (old_session,new_session)) THEN
    RAISE EXCEPTION 'published class routine requires a versioned replacement' USING ERRCODE='23514';
  END IF;
  IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER class_routine_published_guard BEFORE INSERT OR UPDATE OR DELETE ON class_routines
FOR EACH ROW EXECUTE FUNCTION guard_published_class_routine();

CREATE FUNCTION guard_class_routine_plan_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.state IN ('published','legacy') THEN
    RAISE EXCEPTION 'published class routine history is immutable' USING ERRCODE='23514';
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER class_routine_plan_history_guard BEFORE UPDATE OR DELETE ON class_routine_plan_versions
FOR EACH ROW EXECUTE FUNCTION guard_class_routine_plan_history();
