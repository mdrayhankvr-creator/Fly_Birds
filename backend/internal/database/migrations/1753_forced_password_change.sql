-- Checkpoint 4: account safety.
--
-- An account created with a password someone else chose (a student login
-- made at admission, or a password reset by an administrator) must set its
-- own password before it can use anything else. The API refuses every other
-- request while the flag is set; changing the password clears it.
ALTER TABLE users ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
