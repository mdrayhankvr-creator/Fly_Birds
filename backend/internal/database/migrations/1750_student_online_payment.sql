-- Families now use the student's account instead of a separate guardian
-- login, so online payment moves to the student role. A student can only
-- ever pay their own fees: the server resolves the student from the signed-in
-- account and refuses any other student ID. Every payment-safety rule from
-- 1401 still applies: online payment works only while the deployment allows
-- simulation and an administrator has switched it on, and a simulated
-- checkout never records a payment or moves money.
INSERT INTO role_permissions (role_id, permission_id) VALUES ('student', 'fees:pay_online')
ON CONFLICT DO NOTHING;

UPDATE permissions SET description = 'Use online payment for your own fees (students) or linked children (guardians) when the school enables it (simulation only; no provider is integrated)'
WHERE id = 'fees:pay_online';
