-- Checkpoint 5: students sign in with their Student ID.
--
-- Most students have no email address. A student login gets a sign-in ID:
-- the student's Student ID (student_code) when the school uses one and no
-- other student or login has it, otherwise the admission number. It is unique
-- regardless of case, belongs to student logins only, and does not change
-- when the student record is later edited, so a printed slip stays valid.
--
-- A login made without an email gets a placeholder address under the
-- reserved .invalid domain (RFC 2606): mail never reaches it and the API
-- refuses it at sign-in, so the sign-in ID is the only way in.
ALTER TABLE users
    ADD COLUMN login_id text,
    ADD CONSTRAINT users_login_id_ck CHECK (login_id IS NULL OR (login_id ~ '^[^[:space:]@]{1,64}$' AND role = 'student'));
CREATE UNIQUE INDEX users_login_id_key ON users (lower(login_id)) WHERE login_id IS NOT NULL;

-- Existing student logins get an ID too, by the same rule, oldest first.
DO $$
DECLARE r record;
DECLARE chosen text;
BEGIN
    FOR r IN SELECT s.id, s.user_id, s.student_code, s.admission_no FROM students s JOIN users u ON u.id = s.user_id AND u.role = 'student' ORDER BY s.created_at, s.id LOOP
        chosen := NULL;
        IF r.student_code ~ '^[^[:space:]@]{1,64}$'
           AND NOT EXISTS (SELECT 1 FROM students x WHERE x.id <> r.id AND (lower(x.student_code) = lower(r.student_code) OR lower(x.admission_no) = lower(r.student_code)))
           AND NOT EXISTS (SELECT 1 FROM users u WHERE lower(u.login_id) = lower(r.student_code)) THEN
            chosen := r.student_code;
        ELSIF r.admission_no ~ '^[^[:space:]@]{1,64}$'
           AND NOT EXISTS (SELECT 1 FROM users u WHERE lower(u.login_id) = lower(r.admission_no)) THEN
            chosen := r.admission_no;
        END IF;
        IF chosen IS NOT NULL THEN
            UPDATE users SET login_id = chosen WHERE id = r.user_id;
        END IF;
    END LOOP;
END $$;
