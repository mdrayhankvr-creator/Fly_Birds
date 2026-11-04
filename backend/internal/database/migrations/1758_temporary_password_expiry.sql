-- Temporary passwords expire.
--
-- A password someone else chose for an account (admission, an administrator's
-- reset, one-click or section logins, bulk admission) stops working 14 days
-- after it was issued if the account never replaced it. The administrator can
-- issue a new one by resetting the password. Changing the password clears the
-- date. Accounts already waiting to change a temporary password get 14 days
-- from now.
ALTER TABLE users ADD COLUMN temporary_password_expires_at timestamptz;
UPDATE users SET temporary_password_expires_at = now() + interval '14 days' WHERE must_change_password;
