package fees

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"gorm.io/gorm"
)

type feeFixture struct {
	db                                                 *gorm.DB
	admin, accountant, studentUser, guardian, stranger string
	session, class, section                            string
	first, second, inactive                            string
	stamp                                              string
}

// newFeeFixture commits its rows so concurrent payments see real transactions,
// and removes them again in dependency order.
func newFeeFixture(t *testing.T, ctx context.Context, sessionName string) feeFixture {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	for range 2 {
		if err := database.Migrate(ctx, db); err != nil {
			t.Fatal(err)
		}
	}
	var applied int64
	if err := db.Table("schema_migrations").Where("version=801").Count(&applied).Error; err != nil || applied != 1 {
		t.Fatalf("migration 801: %d %v", applied, err)
	}
	f := &feeFixture{db: db, stamp: fmt.Sprintf("%d", time.Now().UnixNano())}
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := db.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	users := []string{}
	user := func(role string) string {
		id := insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", fmt.Sprintf("m8-%s-%d-%s@example.test", role, len(users), f.stamp), "unused", role, "M8 "+role)
		users = append(users, id)
		return id
	}
	var guardianID, strangerID string
	t.Cleanup(func() {
		students := nonEmpty(f.first, f.second, f.inactive)
		users := nonEmpty(users...)
		for _, statement := range []string{
			"DELETE FROM ledger_entries WHERE created_by IN ?",
			"DELETE FROM online_payment_intents WHERE payer_user_id IN ?",
		} {
			_ = db.Exec(statement, users).Error
		}
		for _, statement := range []string{
			"DELETE FROM fee_payment_allocations WHERE payment_id IN (SELECT id FROM fee_payments WHERE student_id IN ?)",
			"DELETE FROM fee_payments WHERE student_id IN ?",
			"DELETE FROM student_fees WHERE student_id IN ?",
			"DELETE FROM fee_concessions WHERE student_id IN ?",
			"DELETE FROM student_guardians WHERE student_id IN ?",
			"DELETE FROM students WHERE id IN ?",
		} {
			if err := db.Exec(statement, students).Error; err != nil {
				t.Errorf("cleanup %s: %v", statement, err)
			}
		}
		_ = db.Exec("DELETE FROM guardians WHERE id IN ?", nonEmpty(guardianID, strangerID)).Error
		_ = db.Exec("DELETE FROM fee_structures WHERE class_id=?", f.class).Error
		_ = db.Exec("DELETE FROM fee_categories WHERE created_by IN ?", users).Error
		_ = db.Exec("DELETE FROM academic_sections WHERE id=?", f.section).Error
		_ = db.Exec("DELETE FROM academic_classes WHERE id=?", f.class).Error
		_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", f.session).Error
		_ = db.Exec("DELETE FROM users WHERE id IN ?", users).Error
	})
	f.admin, f.accountant, f.studentUser, f.guardian, f.stranger = user("admin"), user("accountant"), user("student"), user("guardian"), user("guardian")
	f.session = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,'2026-01-01','2199-12-31') RETURNING id", sessionName)
	f.class = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "Class M8 "+f.stamp)
	f.section = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", f.session, f.class)
	student := func(n int, status string, account any) string {
		return insert("INSERT INTO students(admission_no,name_en,name_bn,status,session_id,class_id,section_id,roll_no,user_id) VALUES (?,?,?,?,?,?,?,?,?) RETURNING id",
			fmt.Sprintf("M8-%s-%d", f.stamp, n), fmt.Sprintf("Fee Student %d", n), "শিক্ষার্থী", status, f.session, f.class, f.section, n, account)
	}
	f.first, f.second, f.inactive = student(1, "active", f.studentUser), student(2, "active", nil), student(3, "inactive", nil)
	guardianID = insert("INSERT INTO guardians(name_en,user_id) VALUES ('Fee Guardian',?) RETURNING id", f.guardian)
	strangerID = insert("INSERT INTO guardians(name_en,user_id) VALUES ('Other Guardian',?) RETURNING id", f.stranger)
	if err := db.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship,is_primary) VALUES (?,?,'mother',true)", f.second, guardianID).Error; err != nil {
		t.Fatal(err)
	}
	return *f
}

func nonEmpty(values ...string) []string {
	out := []string{}
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func newKey(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var id string
	if err := db.Raw("SELECT gen_random_uuid()").Row().Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func expect(t *testing.T, err, want error, label string) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s: got %v want %v", label, err, want)
	}
}

func TestPostgresFeesLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	f := newFeeFixture(t, ctx, "2181")
	svc := NewService(NewRepository(f.db), nil)
	code := "T" + f.stamp[len(f.stamp)-8:]

	tuition, err := svc.SaveCategory(ctx, f.accountant, "", CategoryInput{Code: code, NameEN: "Tuition", NameBN: "বেতন", Frequency: "monthly"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SaveCategory(ctx, f.accountant, "", CategoryInput{Code: code, NameEN: "Copy", Frequency: "monthly"})
	expect(t, err, ErrConflict, "duplicate category code")
	exam, err := svc.SaveCategory(ctx, f.accountant, "", CategoryInput{Code: "E" + f.stamp[len(f.stamp)-8:], NameEN: "Exam fee", Frequency: "one_time"})
	if err != nil {
		t.Fatal(err)
	}
	monthly, err := svc.SaveStructure(ctx, f.accountant, "", StructureInput{SessionID: f.session, ClassID: f.class, CategoryID: tuition.ID, Amount: 1500, DueDay: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveStructure(ctx, f.accountant, "", StructureInput{SessionID: f.session, ClassID: f.class, CategoryID: exam.ID, Amount: 800, DueDay: 10}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.SaveStructure(ctx, f.accountant, "", StructureInput{SessionID: f.session, ClassID: f.class, CategoryID: tuition.ID, Amount: 900})
	expect(t, err, ErrConflict, "duplicate structure")
	_, err = svc.SaveCategory(ctx, f.accountant, tuition.ID, CategoryInput{Code: code, NameEN: "Tuition", Frequency: "one_time"})
	expect(t, err, ErrConflict, "frequency change after use")
	if _, err = svc.SaveConcession(ctx, f.accountant, ConcessionInput{StudentID: f.second, CategoryID: tuition.ID, Percent: 50, Reason: "Sibling"}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.GenerateDues(ctx, f.accountant, GenerateInput{SessionID: f.session, ClassID: f.class, Period: "2026-01", IncludeOneTime: true})
	if err != nil || result.Created != 4 || result.Skipped != 0 {
		t.Fatalf("generate: %+v %v", result, err)
	}
	if result, err = svc.GenerateDues(ctx, f.accountant, GenerateInput{SessionID: f.session, ClassID: f.class, Period: "2026-01", IncludeOneTime: true}); err != nil || result.Created != 0 || result.Skipped != 4 {
		t.Fatalf("regenerate: %+v %v", result, err)
	}
	_, err = svc.GenerateDues(ctx, f.accountant, GenerateInput{SessionID: f.session, Period: "2025-12"})
	expect(t, err, ErrInvalid, "period outside session")
	if result, err = svc.GenerateDues(ctx, f.accountant, GenerateInput{SessionID: f.session, ClassID: f.class, Period: "2199-12"}); err != nil || result.Created != 2 {
		t.Fatalf("future month: %+v %v", result, err)
	}
	_, err = svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.first, StructureID: monthly.ID, Period: "2026-01"})
	expect(t, err, ErrConflict, "duplicate monthly due")
	_, err = svc.AssignDue(ctx, f.accountant, DueInput{StudentID: f.first, StructureID: monthly.ID})
	expect(t, err, ErrInvalid, "monthly due without period")

	dues, err := svc.ListDues(ctx, Query{StudentID: f.second, CategoryID: tuition.ID, Period: "2026-01"})
	if err != nil || len(dues.Items) != 1 || dues.Items[0].DiscountAmount != 750 || dues.Items[0].Balance != 750 || !dues.Items[0].Overdue || dues.Items[0].Status != "unpaid" {
		t.Fatalf("concession due: %+v %v", dues, err)
	}
	concessionDue := dues.Items[0].ID
	if adjustments, err := svc.Adjustments(ctx, concessionDue); err != nil || len(adjustments) != 1 || adjustments[0].Amount != 750 {
		t.Fatalf("concession adjustment: %+v %v", adjustments, err)
	}

	// The first student owes January tuition 1500, the exam fee 800 and
	// December 2199 tuition 1500.
	firstKey := newKey(t, f.db)
	receipt, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 1000, Method: "cash", IdempotencyKey: firstKey})
	if err != nil || receipt.Payment.ReceiptNo == "" || receipt.Replayed || receipt.BalanceAfter != 2800 {
		t.Fatalf("partial payment: %+v %v", receipt, err)
	}
	allocated := 0.0
	for _, a := range receipt.Allocations {
		allocated += a.Amount
		if a.DueDate != "2026-01-10" {
			t.Fatalf("payment skipped the oldest dues: %+v", receipt.Allocations)
		}
	}
	if allocated != 1000 {
		t.Fatalf("allocated %v", allocated)
	}
	replay, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 1000, Method: "cash", IdempotencyKey: firstKey})
	if err != nil || !replay.Replayed || replay.Payment.ReceiptNo != receipt.Payment.ReceiptNo || replay.BalanceAfter != 2800 {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	_, err = svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 999, Method: "cash", IdempotencyKey: firstKey})
	expect(t, err, ErrDuplicate, "reused idempotency key")
	if _, err = svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 300, Method: "bkash", Reference: "TRX" + f.stamp, IdempotencyKey: newKey(t, f.db)}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 100, Method: "bkash", Reference: "trx" + f.stamp, IdempotencyKey: newKey(t, f.db)})
	expect(t, err, ErrDuplicate, "duplicate mobile reference")
	_, err = svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 2500.01, Method: "cash", IdempotencyKey: newKey(t, f.db)})
	expect(t, err, ErrOverpayment, "overpayment")
	var ledgerRows int64
	f.db.Table("ledger_entries").Where("payment_id=?", receipt.Payment.ID).Count(&ledgerRows)
	if ledgerRows != 1 {
		t.Fatalf("ledger rows for replayed receipt: %d", ledgerRows)
	}
	err = f.db.Exec("UPDATE student_fees SET paid_amount=amount+1 WHERE student_id=?", f.first).Error
	expect(t, dbErr(err), ErrOverpayment, "database balance guard")

	// Five tellers race to collect the same remaining 2500; only one succeeds.
	var wg sync.WaitGroup
	results := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.RecordPayment(ctx, f.accountant, PaymentInput{StudentID: f.first, Amount: 2500, Method: "cash", IdempotencyKey: newKey(t, f.db)})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if !errors.Is(err, ErrOverpayment) {
			t.Fatalf("concurrent payment: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("concurrent payments succeeded %d times", succeeded)
	}
	statement, err := svc.Statement(ctx, "student", f.studentUser, "")
	if err != nil || statement.Student.ID != f.first || statement.Totals.Charged != 3800 || statement.Totals.Paid != 3800 || statement.Totals.Balance != 0 || len(statement.Payments) != 3 {
		t.Fatalf("statement: %+v %v", statement.Totals, err)
	}
	var paidSum, ledgerSum float64
	f.db.Raw("SELECT COALESCE(sum(amount),0) FROM fee_payments WHERE student_id=?", f.first).Row().Scan(&paidSum)
	f.db.Raw("SELECT COALESCE(sum(le.amount),0) FROM ledger_entries le JOIN fee_payments p ON p.id=le.payment_id WHERE p.student_id=?", f.first).Row().Scan(&ledgerSum)
	if paidSum != 3800 || ledgerSum != 3800 {
		t.Fatalf("ledger reconciliation %v %v", paidSum, ledgerSum)
	}

	// Discounts and waivers.
	adjusted, err := svc.Adjust(ctx, f.accountant, "accountant", concessionDue, AdjustmentInput{Kind: "discount", Amount: 200, Reason: "Merit"})
	if err != nil || adjusted.Balance != 550 {
		t.Fatalf("discount: %+v %v", adjusted, err)
	}
	_, err = svc.Adjust(ctx, f.accountant, "accountant", concessionDue, AdjustmentInput{Kind: "discount", Amount: 600, Reason: "Too much"})
	expect(t, err, ErrOverpayment, "discount above balance")
	_, err = svc.Adjust(ctx, f.accountant, "accountant", concessionDue, AdjustmentInput{Kind: "waiver", Reason: "Flood"})
	expect(t, err, ErrForbidden, "accountant waiver")
	if adjusted, err = svc.Adjust(ctx, f.admin, "admin", concessionDue, AdjustmentInput{Kind: "waiver", Reason: "Flood"}); err != nil || adjusted.Balance != 0 || adjusted.Status != "waived" {
		t.Fatalf("waiver: %+v %v", adjusted, err)
	}
	_, err = svc.Adjust(ctx, f.admin, "admin", concessionDue, AdjustmentInput{Kind: "discount", Amount: 1, Reason: "Again"})
	expect(t, err, ErrConflict, "adjust settled due")
	firstDues, _ := svc.ListDues(ctx, Query{StudentID: f.first, Status: "paid"})
	if firstDues.Total != 3 {
		t.Fatalf("paid dues %d", firstDues.Total)
	}
	expect(t, svc.DeleteDue(ctx, f.accountant, firstDues.Items[0].ID), ErrConflict, "delete paid due")
	secondExam, _ := svc.ListDues(ctx, Query{StudentID: f.second, CategoryID: exam.ID})
	if err := svc.DeleteDue(ctx, f.accountant, secondExam.Items[0].ID); err != nil {
		t.Fatal(err)
	}

	// Mock bKash and Nagad: the second student still owes 750 for December 2199.
	_, err = svc.StartOnlinePayment(ctx, "guardian", f.stranger, IntentInput{StudentID: f.second, Provider: "bkash", Amount: 100})
	expect(t, err, ErrNotFound, "unlinked guardian")
	_, err = svc.StartOnlinePayment(ctx, "guardian", f.guardian, IntentInput{StudentID: f.second, Provider: "bkash", Amount: 750.01})
	expect(t, err, ErrOverpayment, "online overpayment")
	declined, err := svc.StartOnlinePayment(ctx, "guardian", f.guardian, IntentInput{StudentID: f.second, Provider: "bkash", Amount: 500})
	if err != nil || declined.Status != "pending" || declined.ProviderRef[:2] != "BK" {
		t.Fatalf("intent: %+v %v", declined, err)
	}
	_, err = svc.ConfirmOnlinePayment(ctx, f.guardian, declined.ID, ConfirmInput{OTP: "000000"})
	expect(t, err, ErrDeclined, "wrong OTP")
	_, err = svc.ConfirmOnlinePayment(ctx, f.guardian, declined.ID, ConfirmInput{OTP: MockOTP})
	expect(t, err, ErrConflict, "confirm failed intent")
	intent, err := svc.StartOnlinePayment(ctx, "guardian", f.guardian, IntentInput{StudentID: f.second, Provider: "nagad", Amount: 500})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ConfirmOnlinePayment(ctx, f.stranger, intent.ID, ConfirmInput{OTP: MockOTP})
	expect(t, err, ErrNotFound, "another payer confirming")
	online, err := svc.ConfirmOnlinePayment(ctx, f.guardian, intent.ID, ConfirmInput{OTP: MockOTP})
	if err != nil || online.Payment.Method != "nagad" || online.Payment.Source != "online" || online.Payment.Reference != intent.ProviderRef || online.BalanceAfter != 250 {
		t.Fatalf("online receipt: %+v %v", online, err)
	}
	again, err := svc.ConfirmOnlinePayment(ctx, f.guardian, intent.ID, ConfirmInput{OTP: MockOTP})
	if err != nil || !again.Replayed || again.Payment.ID != online.Payment.ID {
		t.Fatalf("confirm replay: %+v %v", again, err)
	}
	stale, _ := svc.StartOnlinePayment(ctx, "guardian", f.guardian, IntentInput{StudentID: f.second, Provider: "bkash", Amount: 100})
	f.db.Exec("UPDATE online_payment_intents SET expires_at=now()-interval '1 minute' WHERE id=?", stale.ID)
	_, err = svc.ConfirmOnlinePayment(ctx, f.guardian, stale.ID, ConfirmInput{OTP: MockOTP})
	expect(t, err, ErrExpired, "expired intent")

	// Receipt and statement scoping.
	if _, err := svc.Receipt(ctx, "student", f.studentUser, "", receipt.Payment.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Receipt(ctx, "student", f.studentUser, "", online.Payment.ID)
	expect(t, err, ErrNotFound, "student reading another receipt")
	if _, err := svc.Receipt(ctx, "guardian", f.guardian, f.second, online.Payment.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Receipt(ctx, "guardian", f.guardian, f.second, receipt.Payment.ID)
	expect(t, err, ErrNotFound, "guardian reading unrelated receipt through own child")
	_, err = svc.Statement(ctx, "guardian", f.guardian, f.first)
	expect(t, err, ErrNotFound, "guardian reading unlinked student")

	// Reports for this class only, so other suites cannot disturb the totals.
	today := todayDhaka()
	report, err := svc.Collections(ctx, Query{ClassID: f.class, From: today, To: today})
	if err != nil || report.Total != 4300 || report.Count != 4 || len(report.ByMethod) != 3 || len(report.ByCategory) != 2 {
		t.Fatalf("collections: %+v %v", report, err)
	}
	dueReport, err := svc.DueReport(ctx, Query{ClassID: f.class})
	if err != nil || dueReport.Total != 1 || dueReport.TotalBalance != 250 || dueReport.TotalOverdue != 0 || dueReport.Items[0].StudentID != f.second {
		t.Fatalf("due report: %+v %v", dueReport, err)
	}
	if overdue, err := svc.DueReport(ctx, Query{ClassID: f.class, OverdueOnly: true}); err != nil || overdue.Total != 0 {
		t.Fatalf("overdue report: %+v %v", overdue, err)
	}

	// Manual ledger entries on a date no other suite uses.
	expense, err := svc.SaveEntry(ctx, f.accountant, EntryInput{EntryType: "expense", Category: "utilities", Amount: 300, EntryDate: "2001-02-03", Description: "Electricity"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SaveEntry(ctx, f.accountant, EntryInput{EntryType: "income", Category: "donation", Amount: 1000, EntryDate: "2001-02-03", Description: "Alumni gift"}); err != nil {
		t.Fatal(err)
	}
	summary, err := svc.Summary(ctx, "2001-02-03", "2001-02-03")
	if err != nil || summary.Income != 1000 || summary.Expense != 300 || summary.Net != 700 {
		t.Fatalf("summary: %+v %v", summary, err)
	}
	var feeEntry string
	f.db.Raw("SELECT id FROM ledger_entries WHERE payment_id=?", receipt.Payment.ID).Row().Scan(&feeEntry)
	expect(t, svc.DeleteEntry(ctx, f.accountant, feeEntry), ErrConflict, "delete fee income")
	if err := svc.DeleteEntry(ctx, f.accountant, expense.ID); err != nil {
		t.Fatal(err)
	}
	if summary, _ = svc.Summary(ctx, "2001-02-03", "2001-02-03"); summary.Expense != 0 {
		t.Fatalf("expense after delete %v", summary.Expense)
	}
	var audits int64
	f.db.Table("audit_events").Where("actor_user_id IN ? AND action LIKE 'fee.%'", []string{f.accountant, f.admin, f.guardian}).Count(&audits)
	if audits < 10 {
		t.Fatalf("audit events %d", audits)
	}
}
