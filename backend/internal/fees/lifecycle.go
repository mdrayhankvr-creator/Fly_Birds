package fees

import (
	"context"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Late fines, payment reversals and the dues cancelled when a student
// leaves (lifecycle audit QA-08 and QA-09).

// applyLateFines charges each due its late fine once when it still owes
// money after its due date (today in Dhaka). studentID limits it to one
// student ("" for every student). It must run inside a transaction.
func applyLateFines(tx *gorm.DB, today, studentID string) (FineResult, error) {
	var out FineResult
	filter, args := "", []any{today, today}
	if studentID != "" {
		filter = " AND student_id = ?::uuid"
		args = append(args, studentID)
	}
	err := tx.Raw(`WITH charged AS (
  UPDATE student_fees SET fine_amount = late_fine, fine_charged_on = ?::date, updated_at = now()
   WHERE late_fine > 0 AND fine_amount = 0 AND due_date < ?::date AND balance > 0 AND cancellation_id IS NULL`+filter+`
  RETURNING id, late_fine
), noted AS (
  INSERT INTO fee_adjustments (student_fee_id, kind, amount, reason) SELECT id, 'late_fine', late_fine, 'Late fine' FROM charged
)
SELECT count(*), COALESCE(sum(late_fine), 0) FROM charged`, args...).Row().Scan(&out.Charged, &out.Amount)
	if err != nil || out.Charged == 0 {
		return out, dbErr(err)
	}
	target, kind := "fees", "school"
	if studentID != "" {
		target, kind = studentID, "student"
	}
	return out, audit.Record(tx, nil, "fee.late_fines_charged", kind, target, map[string]any{"dues": out.Charged, "amount": Decimal(mustCents(out.Amount)), "as_of": today})
}

// ApplyLateFines charges the late fines due by today; studentID "" means
// every student.
func (r *GormRepository) ApplyLateFines(ctx context.Context, today, studentID string) (FineResult, error) {
	var out FineResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if studentID != "" {
			if err := lockStudent(tx, studentID); err != nil {
				return err
			}
		}
		var err error
		out, err = applyLateFines(tx, today, studentID)
		return err
	})
	return out, dbErr(err)
}

// ReversePayment reverses a counter payment. The dues it paid are owed
// again, the receipt is marked reversed, and a negative fee income line
// balances the ledger. Nothing is deleted.
func (r *GormRepository) ReversePayment(ctx context.Context, actor, id, kind, reason, today string) (Payment, error) {
	var out Payment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p struct {
			StudentID string
			Source    string
			Simulated bool
			Reversed  bool
			Amount    float64
			ReceiptNo string
		}
		if err := tx.Raw("SELECT student_id,source,simulated,reversed_at IS NOT NULL AS reversed,amount,receipt_no FROM fee_payments WHERE id=? FOR UPDATE", id).Row().
			Scan(&p.StudentID, &p.Source, &p.Simulated, &p.Reversed, &p.Amount, &p.ReceiptNo); err != nil {
			return dbErr(err)
		}
		if p.Simulated || p.Source != "counter" {
			return ErrForbidden
		}
		if p.Reversed {
			return ErrConflict
		}
		if err := lockStudent(tx, p.StudentID); err != nil {
			return err
		}
		var allocations []struct {
			StudentFeeID string
			Amount       float64
		}
		if err := tx.Raw("SELECT student_fee_id,amount FROM fee_payment_allocations WHERE payment_id=? ORDER BY student_fee_id", id).Scan(&allocations).Error; err != nil {
			return err
		}
		for _, a := range allocations {
			if err := tx.Exec("UPDATE student_fees SET paid_amount=paid_amount-?::numeric,updated_at=now() WHERE id=?", Decimal(mustCents(a.Amount)), a.StudentFeeID).Error; err != nil {
				return dbErr(err)
			}
		}
		if err := tx.Exec("UPDATE fee_payments SET reversed_at=now(),reversed_by=?,reversal_kind=?,reversal_reason=? WHERE id=?", actor, kind, reason, id).Error; err != nil {
			return dbErr(err)
		}
		label := map[string]string{"error": "Reversal of fee receipt ", "refund": "Refund of fee receipt "}[kind]
		cents := mustCents(p.Amount)
		if err := tx.Exec("INSERT INTO ledger_entries(entry_type,category,amount,entry_date,description,reference,reverses_payment_id,created_by) VALUES ('income','fees',?::numeric,?::date,?,?,?,?)",
			Decimal(-cents), today, label+p.ReceiptNo, p.ReceiptNo, id, actor).Error; err != nil {
			return dbErr(err)
		}
		if err := audit.Record(tx, &actor, "fee.payment_reversed", "fee_payment", id, map[string]any{"student_id": p.StudentID, "receipt_no": p.ReceiptNo, "amount": Decimal(cents), "kind": kind, "reason": reason, "dues_reopened": len(allocations)}); err != nil {
			return err
		}
		return paymentBase(tx).Select(paymentSelect).Where("p.id=?", id).Take(&out).Error
	})
	return out, dbErr(err)
}

const cancellationSelect = `fc.id,fc.student_id,s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,s.status AS student_status,ac.name AS class_name,se.name AS session_name,
fc.reason,to_char(fc.left_on,'YYYY-MM-DD') AS left_on,fc.dues,fc.amount,to_char(fc.created_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS created_at,
COALESCE(NULLIF(cu.display_name,''),cu.email,'') AS created_by_name,to_char(fc.reverted_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS reverted_at,fc.revert_reason,
COALESCE(NULLIF(ru.display_name,''),ru.email,'') AS reverted_by_name`

func cancellationBase(db *gorm.DB) *gorm.DB {
	return db.Table("fee_due_cancellations fc").Joins("JOIN students s ON s.id=fc.student_id").Joins("JOIN academic_classes ac ON ac.id=s.class_id").
		Joins("JOIN academic_sessions se ON se.id=fc.session_id").Joins("LEFT JOIN users cu ON cu.id=fc.created_by").Joins("LEFT JOIN users ru ON ru.id=fc.reverted_by")
}

func cancellationPeriods(db *gorm.DB, items []Cancellation) error {
	for i := range items {
		items[i].Periods = []string{}
		if err := db.Raw(`SELECT to_char(sf.period,'YYYY-MM') FROM fee_adjustments a JOIN student_fees sf ON sf.id=a.student_fee_id
 WHERE a.cancellation_id=? AND a.kind='leaving_cancellation' AND sf.period IS NOT NULL GROUP BY sf.period ORDER BY sf.period`, items[i].ID).Scan(&items[i].Periods).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListCancellations lists the dues cancelled when students left, newest
// first. Status "open" or "reverted" filters them.
func (r *GormRepository) ListCancellations(ctx context.Context, q Query) (Page[Cancellation], error) {
	db := r.db.WithContext(ctx)
	base := cancellationBase(db)
	if q.StudentID != "" {
		base = base.Where("fc.student_id=?", q.StudentID)
	}
	if q.SessionID != "" {
		base = base.Where("fc.session_id=?", q.SessionID)
	}
	switch q.Status {
	case "open":
		base = base.Where("fc.reverted_at IS NULL")
	case "reverted":
		base = base.Where("fc.reverted_at IS NOT NULL")
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(s.admission_no) LIKE ? OR lower(s.name_en) LIKE ? OR s.name_bn LIKE ?)", p, p, likePattern(q.Search))
	}
	out, err := paginate[Cancellation](base, q, cancellationSelect, nil, "fc.created_at DESC,fc.id")
	if err != nil {
		return out, err
	}
	return out, cancellationPeriods(db, out.Items)
}

// RevertCancellation makes the dues of one leaving owed again (for example
// when the leaving was recorded by mistake). The cancellation and both
// adjustments stay on record.
func (r *GormRepository) RevertCancellation(ctx context.Context, actor, id, reason string) (Cancellation, error) {
	var out Cancellation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var c struct {
			StudentID string
			Reverted  bool
		}
		if err := tx.Raw("SELECT student_id,reverted_at IS NOT NULL FROM fee_due_cancellations WHERE id=? FOR UPDATE", id).Row().Scan(&c.StudentID, &c.Reverted); err != nil {
			return dbErr(err)
		}
		if c.Reverted {
			return ErrConflict
		}
		if err := lockStudent(tx, c.StudentID); err != nil {
			return err
		}
		var restored struct {
			Dues   int64
			Amount float64
		}
		if err := tx.Raw(`WITH cancelled AS (
  SELECT a.student_fee_id, a.amount FROM fee_adjustments a
   WHERE a.cancellation_id = ? AND a.kind = 'leaving_cancellation'
), owed AS (
  UPDATE student_fees sf SET discount_amount = sf.discount_amount - c.amount, cancellation_id = NULL, updated_at = now()
    FROM cancelled c WHERE sf.id = c.student_fee_id AND sf.cancellation_id = ?
  RETURNING sf.id, c.amount
), noted AS (
  INSERT INTO fee_adjustments (student_fee_id, kind, amount, reason, created_by, cancellation_id)
  SELECT id, 'cancellation_reverted', amount, ?, ?, ? FROM owed
)
SELECT count(*), COALESCE(sum(amount), 0) FROM owed`, id, id, reason, actor, id).Row().Scan(&restored.Dues, &restored.Amount); err != nil {
			return dbErr(err)
		}
		if err := tx.Exec("UPDATE fee_due_cancellations SET reverted_at=now(),reverted_by=?,revert_reason=? WHERE id=?", actor, reason, id).Error; err != nil {
			return dbErr(err)
		}
		if err := audit.Record(tx, &actor, "fee.leaving_cancellation_reverted", "student", c.StudentID, map[string]any{"cancellation_id": id, "dues": restored.Dues, "amount": Decimal(mustCents(restored.Amount)), "reason": reason}); err != nil {
			return err
		}
		if err := cancellationBase(tx).Select(cancellationSelect).Where("fc.id=?", id).Take(&out).Error; err != nil {
			return err
		}
		items := []Cancellation{out}
		if err := cancellationPeriods(tx, items); err != nil {
			return err
		}
		out = items[0]
		return nil
	})
	return out, dbErr(err)
}
