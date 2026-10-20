-- Payment safety. No bKash or Nagad integration has ever existed: every
-- payment recorded from an online checkout (source 'online') was produced by
-- the built-in simulator and moved no money. This migration labels those
-- payments instead of rewriting them, records what it found, and makes the
-- simulator unavailable unless a deployment and an administrator both enable
-- it.

-- Simulated payments stay in the history, labelled, and never count as real
-- money. An administrator may void one (an explicit, audited action) to
-- restore the dues it reduced.
ALTER TABLE fee_payments
    ADD COLUMN simulated boolean NOT NULL DEFAULT false,
    ADD COLUMN voided_at timestamptz,
    ADD COLUMN voided_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN void_reason text NOT NULL DEFAULT '' CHECK (void_reason = btrim(void_reason) AND octet_length(void_reason) <= 240);

UPDATE fee_payments SET simulated = true WHERE source = 'online';

ALTER TABLE fee_payments
    -- Until a real provider is integrated, online payments can only be simulations.
    ADD CONSTRAINT fee_payments_online_is_simulated CHECK (source <> 'online' OR simulated),
    -- Only simulated payments can be voided, and a void records who and why.
    ADD CONSTRAINT fee_payments_void_only_simulated CHECK (voided_at IS NULL OR simulated),
    ADD CONSTRAINT fee_payments_void_complete CHECK ((voided_at IS NULL) = (voided_by IS NULL) AND (voided_at IS NULL OR void_reason <> ''));

CREATE INDEX fee_payments_simulated_idx ON fee_payments (paid_on) WHERE simulated;

-- What the upgrade found, for the school's audit log.
INSERT INTO audit_events (action, target_type, target_id, outcome, metadata)
SELECT 'fee.simulated_payments_labelled', 'school', 'payments', 'success',
       jsonb_build_object('payments', count(*), 'amount', COALESCE(sum(amount), 0)::text, 'migration', 1401)
FROM fee_payments WHERE simulated
HAVING count(*) > 0;

-- New simulations end in their own status and never create a payment.
ALTER TABLE online_payment_intents DROP CONSTRAINT online_payment_intents_status_check;
ALTER TABLE online_payment_intents ADD CONSTRAINT online_payment_intents_status_check CHECK (status IN ('pending', 'completed', 'failed', 'simulated'));

-- The administrator's switch; effective only where the deployment allows
-- simulation (PAYMENT_SIMULATION_ALLOWED=true).
CREATE TABLE payment_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    simulation_enabled boolean NOT NULL DEFAULT false,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO payment_settings (id) VALUES (1);

-- Students no longer pay online; guardians keep the permission, which works
-- only while simulation is enabled (and later with a verified provider).
DELETE FROM role_permissions WHERE role_id = 'student' AND permission_id = 'fees:pay_online';
UPDATE permissions SET description = 'Use online payment for linked children when the school enables it (simulation only; no provider is integrated)'
WHERE id = 'fees:pay_online';
