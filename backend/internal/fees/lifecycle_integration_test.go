package fees

import (
	"context"
	"testing"
	"time"
)

// Lifecycle audit: dues cancelled when a student leaves (QA-08), payment
// reversals and late fines (QA-09).

func monthFrom(base time.Time, add int) string {
	return time.Date(base.Year(), base.Month()+time.Month(add), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
}

func dueOf(t *testing.T, svc *Service, student, period string) Due {
	t.Helper()
	page, err := svc.ListDues(context.Background(), Query{StudentID: student, Period: period})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("due %s: %+v %v", period, page.Items, err)
	}
	return page.Items[0]
}

// leave changes a student's status the way the application does: inside a
// transaction that names the acting user for the database triggers.
func leave(t *testing.T, f feeFixture, student, actor, status string) {
	t.Helper()
	tx := f.db.Begin()
	if err := tx.Exec("SELECT set_config('flybirds.actor_id', ?, true)", actor).Error; err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE students SET status=?, updated_at=now() WHERE id=?", status, student).Error; err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
}

func TestPostgresLeavingCancelsLaterDues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newFeeFixture(t, ctx, "2183")
	svc := NewService(NewRepository(f.db), nil)
	// The leaving date is the database's date (the enrolment's end date).
	var now time.Time
	if err := f.db.Raw("SELECT current_date").Row().Scan(&now); err != nil {
		t.Fatal(err)
	}
	cat, err := svc.SaveCategory(ctx, f.accountant, "", CategoryInput{Code: "L" + f.stamp[len(f.stamp)-8:], NameEN: "Tuition", Frequency: "monthly"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := svc.SaveStructure(ctx, f.accountant, "", StructureInput{SessionID: f.session, ClassID: f.class, CategoryID: cat.ID, Amount: 500, DueDay: 10})
	if err != nil {
		t.Fatal(err)
	}
	months := []string{monthFrom(now, 0), monthFrom(now, 1), monthFrom(now, 2), monthFrom(now, 3)}
	for _, student := range []string{f.first, f.second} {
		for _, m := range months {
			if _, err := svc.AssignDue(ctx, f.accountant, DueInput{StudentID: student, StructureID: st.ID, Period: m}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Next month paid in full, the one after in part.
	paidMonth, partMonth := dueOf(t, svc, f.first, months[1]), dueOf(t, svc, f.first, months[2])
	if _, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 500, Method: "cash", IdempotencyKey: newKey(t, f.db), FeeIDs: []string{paidMonth.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 100, Method: "cash", IdempotencyKey: newKey(t, f.db), FeeIDs: []string{partMonth.ID}}); err != nil {
		t.Fatal(err)
	}

	leave(t, f, f.first, f.admin, "inactive")

	// The month they left, the paid month and the part-paid month stay; only
	// the later unpaid month is cancelled, and nothing is deleted.
	checks := []struct {
		month   string
		status  string
		balance float64
	}{{months[0], "unpaid", 500}, {months[1], "paid", 0}, {months[2], "partial", 400}, {months[3], "cancelled", 0}}
	for _, c := range checks {
		if d := dueOf(t, svc, f.first, c.month); d.Status != c.status || d.Balance != c.balance {
			t.Fatalf("%s after leaving: %s %.2f, want %s %.2f", c.month, d.Status, d.Balance, c.status, c.balance)
		}
	}
	cancelled := dueOf(t, svc, f.first, months[3])
	if cancelled.CancellationID == nil || cancelled.DiscountAmount != 500 {
		t.Fatalf("cancelled due: %+v", cancelled)
	}
	list, err := svc.ListCancellations(ctx, Query{StudentID: f.first})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("cancellations: %+v %v", list, err)
	}
	c := list.Items[0]
	if c.Reason != "withdrawn" || c.Dues != 1 || c.Amount != 500 || c.LeftOn != now.Format(time.DateOnly) || c.RevertedAt != nil ||
		len(c.Periods) != 1 || c.Periods[0] != months[3] || c.CreatedByName != "M8 admin" || c.StudentStatus != "inactive" {
		t.Fatalf("cancellation: %+v", c)
	}
	if adj, err := svc.Adjustments(ctx, cancelled.ID); err != nil || len(adj) != 1 || adj[0].Kind != "leaving_cancellation" || adj[0].Amount != 500 {
		t.Fatalf("cancellation adjustment: %+v %v", adj, err)
	}
	var audited int64
	f.db.Raw("SELECT count(*) FROM audit_events WHERE action='fee.dues_cancelled_on_leaving' AND target_id=? AND actor_user_id=?", f.first, f.admin).Scan(&audited)
	if audited != 1 {
		t.Fatalf("audit events: %d", audited)
	}
	expect(t, svc.DeleteDue(ctx, f.accountant, cancelled.ID), ErrConflict, "deleting a cancelled due")
	if page, err := svc.ListDues(ctx, Query{StudentID: f.first, Status: "cancelled"}); err != nil || page.Total != 1 {
		t.Fatalf("cancelled filter: %+v %v", page, err)
	}
	if _, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 10, Method: "cash", IdempotencyKey: newKey(t, f.db), FeeIDs: []string{cancelled.ID}}); err == nil {
		t.Fatal("a cancelled due took a payment")
	}

	// Reverting needs a reason, makes the due owed again, and happens once.
	_, err = svc.RevertCancellation(ctx, f.admin, c.ID, RevertInput{Reason: " "})
	expect(t, err, ErrInvalid, "revert without a reason")
	reverted, err := svc.RevertCancellation(ctx, f.admin, c.ID, RevertInput{Reason: "Withdrawal entered by mistake"})
	if err != nil || reverted.RevertedAt == nil || reverted.RevertReason != "Withdrawal entered by mistake" || reverted.RevertedByName != "M8 admin" || len(reverted.Periods) != 1 {
		t.Fatalf("revert: %+v %v", reverted, err)
	}
	if d := dueOf(t, svc, f.first, months[3]); d.Status != "unpaid" || d.Balance != 500 || d.CancellationID != nil || d.DiscountAmount != 0 {
		t.Fatalf("after revert: %+v", d)
	}
	if adj, _ := svc.Adjustments(ctx, cancelled.ID); len(adj) != 2 || adj[1].Kind != "cancellation_reverted" {
		t.Fatalf("revert adjustment: %+v", adj)
	}
	_, err = svc.RevertCancellation(ctx, f.admin, c.ID, RevertInput{Reason: "again"})
	expect(t, err, ErrConflict, "second revert")
	if open, _ := svc.ListCancellations(ctx, Query{StudentID: f.first, Status: "open"}); open.Total != 0 {
		t.Fatalf("open after revert: %d", open.Total)
	}

	// A transfer is recorded as such; three later months are cancelled.
	leave(t, f, f.second, f.admin, "transferred")
	if list, err := svc.ListCancellations(ctx, Query{StudentID: f.second}); err != nil || len(list.Items) != 1 || list.Items[0].Reason != "transferred" || list.Items[0].Dues != 3 || list.Items[0].Amount != 1500 {
		t.Fatalf("transfer: %+v %v", list.Items, err)
	}
	// Readmission does not bring the dues back by itself, and leaving again
	// finds nothing more to cancel.
	leave(t, f, f.second, f.admin, "active")
	leave(t, f, f.second, f.admin, "inactive")
	if list, _ := svc.ListCancellations(ctx, Query{StudentID: f.second}); list.Total != 1 {
		t.Fatalf("second leaving: %d cancellations", list.Total)
	}
}

func TestPostgresPaymentReversal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newFeeFixture(t, ctx, "2184")
	svc := NewService(NewRepository(f.db), nil)
	cat, err := svc.SaveCategory(ctx, f.accountant, "", CategoryInput{Code: "R" + f.stamp[len(f.stamp)-8:], NameEN: "Tuition", Frequency: "monthly"})
	if err != nil {
		t.Fatal(err)
	}
	st, err := svc.SaveStructure(ctx, f.accountant, "", StructureInput{SessionID: f.session, ClassID: f.class, CategoryID: cat.ID, Amount: 600, DueDay: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"2199-01", "2199-02"} {
		if _, err := svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.first, StructureID: st.ID, Period: m}); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 900, Method: "bkash", Reference: "TRX" + f.stamp[len(f.stamp)-8:], IdempotencyKey: newKey(t, f.db)})
	if err != nil || receipt.BalanceAfter != 300 {
		t.Fatalf("payment: %+v %v", receipt, err)
	}
	id := receipt.Payment.ID
	_, err = svc.ReversePayment(ctx, f.admin, id, ReverseInput{Kind: "refund", Reason: ""})
	expect(t, err, ErrInvalid, "reversal without a reason")
	_, err = svc.ReversePayment(ctx, f.admin, id, ReverseInput{Kind: "void", Reason: "x"})
	expect(t, err, ErrInvalid, "unknown kind")

	p, err := svc.ReversePayment(ctx, f.admin, id, ReverseInput{Kind: "refund", Reason: "Paid twice; money returned"})
	if err != nil || p.ReversedAt == nil || p.ReversalKind == nil || *p.ReversalKind != "refund" || p.ReversalReason != "Paid twice; money returned" || p.ReversedByName != "M8 admin" {
		t.Fatalf("reverse: %+v %v", p, err)
	}
	// The dues are owed again; the payment and its allocations stay.
	if s, err := svc.Statement(ctx, "staff", f.admin, f.first); err != nil || s.Totals.Balance != 1200 || s.Totals.Paid != 0 {
		t.Fatalf("statement: %+v %v", s.Totals, err)
	}
	var allocations int64
	f.db.Raw("SELECT count(*) FROM fee_payment_allocations WHERE payment_id=?", id).Scan(&allocations)
	if allocations != 2 {
		t.Fatalf("allocations: %d", allocations)
	}
	// The ledger stays balanced: +900 income and a -900 line for the refund.
	var lines []struct {
		Amount            float64
		PaymentID         *string
		ReversesPaymentID *string
		Description       string
	}
	f.db.Raw("SELECT amount,payment_id,reverses_payment_id,description FROM ledger_entries WHERE payment_id=? OR reverses_payment_id=? ORDER BY amount DESC", id, id).Scan(&lines)
	if len(lines) != 2 || lines[0].Amount != 900 || lines[1].Amount != -900 || lines[1].ReversesPaymentID == nil || lines[1].Description != "Refund of fee receipt "+receipt.Payment.ReceiptNo {
		t.Fatalf("ledger: %+v", lines)
	}
	var entryID string
	f.db.Raw("SELECT id FROM ledger_entries WHERE reverses_payment_id=?", id).Scan(&entryID)
	expect(t, svc.DeleteEntry(ctx, f.admin, entryID), ErrConflict, "deleting the reversal line")
	again, err := svc.Receipt(ctx, "staff", f.admin, "", id)
	if err != nil || again.Payment.ReversedAt == nil || again.BalanceAfter != 1200 {
		t.Fatalf("receipt: %+v %v", again.Payment, err)
	}
	_, err = svc.ReversePayment(ctx, f.admin, id, ReverseInput{Kind: "error", Reason: "again"})
	expect(t, err, ErrConflict, "second reversal")
	today := time.Now().In(Dhaka).Format(time.DateOnly)
	if report, err := svc.Collections(ctx, Query{From: today, To: today, ClassID: f.class}); err != nil || report.Total != 0 {
		t.Fatalf("collections count a reversed payment: %+v %v", report, err)
	}
	if page, err := svc.ListPayments(ctx, Query{StudentID: f.first, Status: "reversed"}); err != nil || page.Total != 1 {
		t.Fatalf("reversed filter: %+v %v", page, err)
	}
	var audited int64
	f.db.Raw("SELECT count(*) FROM audit_events WHERE action='fee.payment_reversed' AND target_id=? AND actor_user_id=?", id, f.admin).Scan(&audited)
	if audited != 1 {
		t.Fatalf("audit: %d", audited)
	}
	// The dues can be paid again.
	if r, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 1200, Method: "cash", IdempotencyKey: newKey(t, f.db)}); err != nil || r.BalanceAfter != 0 {
		t.Fatalf("paying again: %+v %v", r, err)
	}
	// Simulated and online payments have their own void.
	var simulated string
	if err := f.db.Raw("INSERT INTO fee_payments(student_id,amount,method,source,reference,paid_on,idempotency_key,received_by,simulated) VALUES (?,10,'bkash','online',?,current_date,gen_random_uuid(),?,true) RETURNING id",
		f.second, "SIM"+f.stamp[len(f.stamp)-8:], f.admin).Row().Scan(&simulated); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReversePayment(ctx, f.admin, simulated, ReverseInput{Kind: "error", Reason: "x"})
	expect(t, err, ErrForbidden, "reversing a simulated payment")
}

func TestPostgresLateFines(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newFeeFixture(t, ctx, "2185")
	svc := NewService(NewRepository(f.db), nil)
	cat, err := svc.SaveCategory(ctx, f.accountant, "", CategoryInput{Code: "F" + f.stamp[len(f.stamp)-8:], NameEN: "Tuition", Frequency: "monthly"})
	if err != nil {
		t.Fatal(err)
	}
	in := StructureInput{SessionID: f.session, ClassID: f.class, CategoryID: cat.ID, Amount: 500, DueDay: 10}
	st, err := svc.SaveStructure(ctx, f.accountant, "", in)
	if err != nil || st.LateFine != 0 {
		t.Fatalf("structure: %+v %v", st, err)
	}
	// Created while the fine is off: never fined.
	if _, err := svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.first, StructureID: st.ID, Period: "2026-01"}); err != nil {
		t.Fatal(err)
	}
	in.LateFine = -5
	_, err = svc.SaveStructure(ctx, f.accountant, st.ID, in)
	expect(t, err, ErrInvalid, "negative fine")
	in.LateFine = 50
	if st, err = svc.SaveStructure(ctx, f.accountant, st.ID, in); err != nil || st.LateFine != 50 {
		t.Fatalf("fine on: %+v %v", st, err)
	}
	if d := dueOf(t, svc, f.first, "2026-01"); d.LateFine != 0 {
		t.Fatalf("an earlier due took the new fine: %+v", d)
	}
	// Created after: late (February), paid on time (March), not yet due.
	for _, m := range []string{"2026-02", "2026-03", "2199-01"} {
		if _, err := svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.first, StructureID: st.ID, Period: m}); err != nil {
			t.Fatal(err)
		}
	}
	march := dueOf(t, svc, f.first, "2026-03")
	if march.LateFine != 50 {
		t.Fatalf("new due: %+v", march)
	}
	// Paying March directly (before any pass) charges February's fine first
	// but not March's: it is settled.
	if err := f.db.Exec("UPDATE student_fees SET paid_amount=amount WHERE id=?", march.ID).Error; err != nil {
		t.Fatal(err)
	}
	result, err := svc.ApplyLateFines(ctx)
	if err != nil || result.Charged < 1 {
		t.Fatalf("pass: %+v %v", result, err)
	}
	feb := dueOf(t, svc, f.first, "2026-02")
	if feb.FineAmount != 50 || feb.Balance != 550 || feb.FineChargedOn == nil || feb.Status != "unpaid" {
		t.Fatalf("february: %+v", feb)
	}
	for _, m := range []string{"2026-01", "2026-03", "2199-01"} {
		if d := dueOf(t, svc, f.first, m); d.FineAmount != 0 {
			t.Fatalf("%s was fined: %+v", m, d)
		}
	}
	// Charged once only.
	before := feb.Balance
	if _, err := svc.ApplyLateFines(ctx); err != nil {
		t.Fatal(err)
	}
	if d := dueOf(t, svc, f.first, "2026-02"); d.Balance != before {
		t.Fatalf("fined twice: %+v", d)
	}
	if adj, _ := svc.Adjustments(ctx, feb.ID); len(adj) != 1 || adj[0].Kind != "late_fine" || adj[0].Amount != 50 {
		t.Fatalf("fine history: %+v", adj)
	}
	// The statement shows the fine, and a payment can cover it.
	s, err := svc.Statement(ctx, "staff", f.admin, f.first)
	if err != nil || s.Totals.Fines != 50 {
		t.Fatalf("statement fines: %+v %v", s.Totals, err)
	}
	if _, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 550, Method: "cash", IdempotencyKey: newKey(t, f.db), FeeIDs: []string{feb.ID}}); err != nil {
		t.Fatal(err)
	}
	if d := dueOf(t, svc, f.first, "2026-02"); d.Balance != 0 || d.Status != "paid" {
		t.Fatalf("february paid: %+v", d)
	}
	// A statement charges a student's own fines that fell due.
	if _, err := svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.second, StructureID: st.ID, Period: "2026-04"}); err != nil {
		t.Fatal(err)
	}
	if s, err := svc.Statement(ctx, "staff", f.admin, f.second); err != nil || s.Totals.Fines != 50 || s.Totals.Balance != 550 {
		t.Fatalf("second student's statement: %+v %v", s.Totals, err)
	}
	// A cancelled due is never fined.
	if _, err := svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.second, StructureID: st.ID, Period: "2199-02"}); err != nil {
		t.Fatal(err)
	}
	leave(t, f, f.second, f.admin, "inactive")
	if err := f.db.Exec("UPDATE student_fees SET fine_amount=late_fine,fine_charged_on=current_date WHERE student_id=? AND period='2199-02-01'", f.second).Error; err == nil {
		t.Fatal("a cancelled due was fined")
	}
}
