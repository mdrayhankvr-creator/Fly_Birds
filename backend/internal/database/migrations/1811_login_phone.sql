-- Sign-in by phone number. One nullable login phone per account, stored in
-- the canonical form +8801XXXXXXXXX (operator prefixes 013–019).
--
-- A staff account's phone identifies that account, so it is unique. Student
-- accounts deliberately may share one: there is no separate guardian
-- dashboard, so a guardian signs in to a child's account with their own
-- number, and siblings of one family share it. Sign-in checks the password
-- against every account with the number and takes the one it matches.
ALTER TABLE users
    ADD COLUMN login_phone text
    CONSTRAINT users_login_phone_ck CHECK (login_phone IS NULL OR login_phone ~ '^\+8801[3-9][0-9]{8}$');

CREATE UNIQUE INDEX users_login_phone_key ON users (login_phone) WHERE login_phone IS NOT NULL AND role <> 'student';
CREATE INDEX users_login_phone_idx ON users (login_phone) WHERE login_phone IS NOT NULL;
