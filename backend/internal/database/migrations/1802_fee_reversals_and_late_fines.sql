-- QA-09, part 1: reversing a counter payment.
--
-- An administrator reverses a payment recorded by mistake ('error') or paid
-- back to the payer ('refund'), with a reason. The payment, its allocations
-- and its ledger income stay; the dues it paid are owed again; the receipt
-- is marked reversed; and one ledger line of the opposite sign
-- (reverses_payment_id) keeps fee income balanced. Simulated and online
-- payments keep their own void (migration 1401).
ALTER TABLE fee_payments
    ADD COLUMN reversed_at timestamptz,
    ADD COLUMN reversed_by uuid REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN reversal_kind text CHECK (reversal_kind IN ('error', 'refund')),
    ADD COLUMN reversal_reason text NOT NULL DEFAULT '' CHECK (reversal_reason = btrim(reversal_reason) AND octet_length(reversal_reason) <= 240),
    ADD CONSTRAINT fee_payments_reversal_complete CHECK (
        (reversed_at IS NULL) = (reversed_by IS NULL) AND (reversed_at IS NULL) = (reversal_kind IS NULL)
        AND (reversed_at IS NULL) = (reversal_reason = '')),
    ADD CONSTRAINT fee_payments_reversal_counter_only CHECK (reversed_at IS NULL OR (source = 'counter' AND NOT simulated));

ALTER TABLE ledger_entries
    ADD COLUMN reverses_payment_id uuid UNIQUE REFERENCES fee_payments(id) ON DELETE RESTRICT,
    DROP CONSTRAINT ledger_entries_amount_check,
    ADD CONSTRAINT ledger_entries_amount_check CHECK (CASE WHEN reverses_payment_id IS NULL THEN amount > 0
        ELSE amount < 0 AND entry_type = 'income' AND category = 'fees' AND payment_id IS NULL END);

-- QA-09, part 2: an optional late fine per fee structure.
--
-- fee_structures.late_fine is a flat amount, 0 (off) by default. A due
-- copies the structure's fine when it is created, so turning a fine on (or
-- changing it) applies only to dues created afterwards. A due still owing
-- money after its due date is charged its fine once (fine_amount,
-- fine_charged_on); the balance includes it.
ALTER TABLE fee_structures
    ADD COLUMN late_fine numeric(12,2) NOT NULL DEFAULT 0 CHECK (late_fine >= 0 AND late_fine <= 100000);

ALTER TABLE student_fees
    ADD COLUMN late_fine numeric(12,2) NOT NULL DEFAULT 0 CHECK (late_fine >= 0),
    ADD COLUMN fine_amount numeric(12,2) NOT NULL DEFAULT 0,
    ADD COLUMN fine_charged_on date,
    ADD CONSTRAINT student_fees_fine_ck CHECK (
        (fine_amount = 0 AND fine_charged_on IS NULL) OR (late_fine > 0 AND fine_amount = late_fine AND fine_charged_on IS NOT NULL));
ALTER TABLE student_fees ALTER COLUMN balance SET EXPRESSION AS (amount + fine_amount - discount_amount - paid_amount);
ALTER TABLE student_fees
    DROP CONSTRAINT student_fees_no_overpayment,
    ADD CONSTRAINT student_fees_no_overpayment CHECK (discount_amount + paid_amount <= amount + fine_amount);
CREATE INDEX student_fees_fine_pending_idx ON student_fees (due_date) WHERE late_fine > 0 AND fine_amount = 0;

ALTER TABLE fee_adjustments
    DROP CONSTRAINT fee_adjustments_kind_check,
    ADD CONSTRAINT fee_adjustments_kind_check CHECK (kind IN ('discount', 'waiver', 'leaving_cancellation', 'cancellation_reverted', 'late_fine')),
    DROP CONSTRAINT fee_adjustments_actor_ck,
    ADD CONSTRAINT fee_adjustments_actor_ck CHECK (created_by IS NOT NULL OR kind IN ('leaving_cancellation', 'late_fine'));

-- A due that left with the student is not fined either.
CREATE OR REPLACE FUNCTION student_fees_fine_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.fine_amount > OLD.fine_amount AND NEW.cancellation_id IS NOT NULL THEN
        RAISE EXCEPTION 'a cancelled due is not fined' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER student_fees_fine_guard_trigger BEFORE UPDATE OF fine_amount ON student_fees
FOR EACH ROW EXECUTE FUNCTION student_fees_fine_guard();
