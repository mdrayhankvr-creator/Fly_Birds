# Lifecycle audit fixes: leaving dues, refunds and late fines, off days

Branch `feature/audit-fixes`, from `main` `c62435b9` (E7 merged). Migrations
1801–1803. It closes three gaps from the lifecycle audit:

| Item | Gap | Fix |
|---|---|---|
| QA-08 | A student who left kept owing every later month | The later unpaid months are cancelled when they leave, never deleted; an administrator can revert |
| QA-09 | A counter payment could not be reversed or refunded; no late fines | Administrator reversal (by mistake or refund) with a reason; an optional late fine per fee structure |
| QA-12 | Attendance could be taken on Fridays and holidays | Weekly off days (Friday by default) and published holidays refuse new attendance |

## 1. Dues when a student leaves (QA-08, migration 1801)

When a student's status becomes withdrawn (`inactive`) or `transferred`, by
any path (the student form or directly in the database), a database trigger cancels
their dues that are:

- monthly, for a month **after** the month they left (the leaving date is the
  end date of the closed enrolment: the day of the change, or the session's
  last day if it was over);
- not paid at all. A month with any payment, part or full, stays as it was.

Nothing is deleted. Each due gets a `leaving_cancellation` adjustment for its
whole remaining balance (so its balance is 0 and its status is *Cancelled*)
and points at one `fee_due_cancellations` row per leaving, which records the
student, reason, leaving date, number of months, amount and who made the
change. One audit event, `fee.dues_cancelled_on_leaving`, is written.
A cancelled due cannot be deleted, paid or fined.

Leaving through a year-end promotion batch is leaving *after* the year: the
year's dues stay, as before (promotion changes no fee row).

An administrator reverts a cancellation on the Dues tab, with a reason (for
example when a withdrawal was recorded by mistake). Each due is owed again
through a `cancellation_reverted` adjustment; the cancellation stays on record
as *Reverted*, and `fee.leaving_cancellation_reverted` is audited.
Readmitting a student does not bring the dues back by itself.

API: `GET /fees/cancellations` (fees:read; `status=open|reverted`,
`student_id`, `session_id`, search) and `POST /fees/cancellations/{id}/revert`
(fees:manage, administrator only).

## 2. Payment reversals and late fines (QA-09, migration 1802)

**Reversal.** An administrator opens a counter receipt and reverses it, as
*recorded by mistake* (`error`) or *money paid back* (`refund`), with a reason
(`POST /fees/payments/{id}/reverse`). In one transaction:

- the dues the payment paid are owed again (its allocations stay);
- the payment is marked reversed (`reversed_at`, `reversed_by`,
  `reversal_kind`, `reversal_reason`); the receipt shows **Reversed** with
  who, when, which kind and why, for staff, students and guardians;
- the ledger gets one fee-income line of the opposite sign, dated the day of
  the reversal (`reverses_payment_id`), so the receipt's income nets to zero.
  Neither line can be deleted;
- `fee.payment_reversed` is audited.

A payment is reversed once. Simulated and online payments keep their own void
(migration 1401). Collection reports leave reversed payments out; the income
and expense summary shows the reversal on the day it happened.

**Late fine.** Each fee structure has an optional flat late fine, `0` (off) by
default, set in Fee setup. A due copies its structure's fine when it is
created, so turning a fine on, or changing it, applies only to dues created
afterwards. A due still owing money after its due date is charged its fine
once (`fine_amount`, `fine_charged_on`, a `late_fine` adjustment); the balance
includes it, and the counter, statements and the dues list show it. Fines are
charged by a background pass at start-up and every 30 minutes, and before a
student's statement is shown or a payment is recorded, so the counter is
always current. A cancelled due is never fined; a waiver covers the fine too.

## 3. Weekly off days and holidays (QA-12, migration 1803)

`school_weekly_off_days` holds the school's weekly off days (Friday by
default; the exam routine planner, E8, will share it). Administrators change
them in School settings (`weekly_off_days` on `GET/PUT /school/settings`, at
most six days, audited). Published events of kind *holiday* close their dates
too.

On a closed date, new attendance is refused (teacher attendance, student
attendance, the roster and QR scans) with `409 attendance_day_off`, a message
such as "no attendance on 2026-10-30: it is a weekly off day" and the day
(reason, weekday, holiday title in English and Bangla). Attendance already
recorded is untouched, is still listed and can still be corrected, also
through the roster. `GET /attendance/day?date=` tells the screens whether a
date is open; they show the reason, disable new marks and keep corrections.

## 4. Verification (2026-11-05)

See [PROGRESS.md](PROGRESS.md) for the results. New tests:

- Go: `TestPostgresLeavingCancelsLaterDues`, `TestPostgresPaymentReversal`,
  `TestPostgresLateFines` (fees); `TestPostgresAttendanceOffDays` and
  additions to the fee-route and roster tests (HTTP);
  `TestCounterTestPhoneAvoidsTheStamp` (the flaky counter search).
- Browser: `e2e/lifecycle-audit.mjs` (all three items through the screens,
  plus the four changed screens at 1440px and 375px in both languages and
  themes). `suite.mjs` opens the day for its attendance flow when it runs on a
  weekly off day and restores the off days afterwards.
