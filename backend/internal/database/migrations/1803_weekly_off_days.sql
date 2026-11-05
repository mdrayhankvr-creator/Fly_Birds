-- QA-12: the school's weekly off days (default Friday). With published
-- holidays (school_events, kind 'holiday'), they block new attendance on
-- those dates. Attendance already recorded is untouched and can still be
-- corrected. Days are numbered as PostgreSQL's EXTRACT(DOW): 0 Sunday …
-- 6 Saturday. The exam routine planner (E8) shares this table.
CREATE TABLE school_weekly_off_days (
    day_of_week smallint PRIMARY KEY CHECK (day_of_week BETWEEN 0 AND 6),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO school_weekly_off_days (day_of_week) VALUES (5);
