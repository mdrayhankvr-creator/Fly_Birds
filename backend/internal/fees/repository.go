package fees

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func dbErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			if strings.HasPrefix(pg.ConstraintName, "fee_payments_") {
				return ErrDuplicate
			}
			return ErrConflict
		case "23514":
			if pg.ConstraintName == "student_fees_no_overpayment" {
				return ErrOverpayment
			}
			return ErrInvalid
		case "23503", "22P02", "22007", "22008":
			return ErrInvalid
		}
	}
	return err
}

// lockSetup serializes fee configuration and bulk due generation.
func lockSetup(tx *gorm.DB) error {
	var unused any
	return tx.Raw("SELECT pg_advisory_xact_lock(801, 0)").Row().Scan(&unused)
}

// lockStudent serializes every balance change for one student, so concurrent
// payments, intents and adjustments cannot both spend the same balance.
func lockStudent(tx *gorm.DB, studentID string) error {
	var unused any
	return tx.Raw("SELECT pg_advisory_xact_lock(802, hashtext(?))", strings.ToLower(studentID)).Row().Scan(&unused)
}

func likePattern(v string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(v) + "%"
}

func paginate[T any](base *gorm.DB, q Query, selectExpr string, selectArgs []any, order string) (Page[T], error) {
	out := Page[T]{Items: []T{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Session(&gorm.Session{}).Count(&out.Total).Error; err != nil {
		return out, err
	}
	err := base.Select(selectExpr, selectArgs...).Order(order).Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Find(&out.Items).Error
	return out, err
}

func (r *GormRepository) Options(ctx context.Context) (Options, error) {
	db := r.db.WithContext(ctx)
	out := Options{Sessions: []Ref{}, Classes: []Ref{}, Sections: []SectionRef{}, Categories: []Category{}}
	if err := db.Table("academic_sessions").Select("id,name,is_active").Order("start_date DESC").Find(&out.Sessions).Error; err != nil {
		return out, err
	}
	if err := db.Table("academic_classes").Select("id,name,is_active").Order("display_order,name").Find(&out.Classes).Error; err != nil {
		return out, err
	}
	if err := db.Table("academic_sections").Select("id,name,session_id,class_id").Where("is_active").Order("display_order,name").Find(&out.Sections).Error; err != nil {
		return out, err
	}
	err := db.Table("fee_categories").Select("id,code,name_en,name_bn,frequency,is_active").Where("is_active").Order("code").Find(&out.Categories).Error
	return out, err
}

const studentSelect = "s.id,s.admission_no,s.name_en,s.name_bn,s.roll_no,s.status,s.session_id,s.class_id,s.section_id,se.name AS session_name,ac.name AS class_name,sec.name AS section_name"

func studentBase(db *gorm.DB) *gorm.DB {
	return db.Table("students s").Joins("JOIN academic_sessions se ON se.id=s.session_id").Joins("JOIN academic_classes ac ON ac.id=s.class_id").Joins("JOIN academic_sections sec ON sec.id=s.section_id")
}

func (r *GormRepository) SearchStudents(ctx context.Context, q Query) (Page[StudentRef], error) {
	base := studentBase(r.db.WithContext(ctx))
	if q.SessionID != "" {
		base = base.Where("s.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("s.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		base = base.Where("s.section_id=?", q.SectionID)
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(s.admission_no) LIKE ? OR lower(s.name_en) LIKE ? OR s.name_bn LIKE ?)", p, p, likePattern(q.Search))
	}
	return paginate[StudentRef](base, q, studentSelect, nil, "s.admission_no,s.id")
}

func (r *GormRepository) ListCategories(ctx context.Context, q Query) (Page[Category], error) {
	base := r.db.WithContext(ctx).Table("fee_categories")
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(code) LIKE ? OR lower(name_en) LIKE ? OR name_bn LIKE ?)", p, p, likePattern(q.Search))
	}
	return paginate[Category](base, q, "id,code,name_en,name_bn,frequency,is_active", nil, "code")
}

func categoryByID(tx *gorm.DB, id string) (Category, error) {
	var c Category
	err := tx.Table("fee_categories").Select("id,code,name_en,name_bn,frequency,is_active").Where("id=?", id).Take(&c).Error
	return c, dbErr(err)
}

func (r *GormRepository) SaveCategory(ctx context.Context, actor, id string, in CategoryInput) (Category, error) {
	var out Category
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSetup(tx); err != nil {
			return err
		}
		active := true
		if in.IsActive != nil {
			active = *in.IsActive
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO fee_categories(code,name_en,name_bn,frequency,is_active,created_by,updated_by) VALUES (?,?,?,?,?,?,?) RETURNING id", in.Code, in.NameEN, in.NameBN, in.Frequency, active, actor, actor).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			old, err := categoryByID(tx, id)
			if err != nil {
				return err
			}
			if old.Code != in.Code || old.Frequency != in.Frequency {
				var used int64
				if err := tx.Table("fee_structures").Where("category_id=?", id).Count(&used).Error; err != nil {
					return err
				}
				if used > 0 {
					return ErrConflict
				}
			}
			if err := tx.Exec("UPDATE fee_categories SET code=?,name_en=?,name_bn=?,frequency=?,is_active=?,updated_by=?,updated_at=now() WHERE id=?", in.Code, in.NameEN, in.NameBN, in.Frequency, active, actor, id).Error; err != nil {
				return dbErr(err)
			}
		}
		var err error
		if out, err = categoryByID(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.category_saved", "fee_category", id, map[string]any{"code": in.Code})
	})
	return out, dbErr(err)
}

const structureSelect = "fs.id,fs.session_id,fs.class_id,fs.category_id,fs.amount,fs.due_day,fs.is_active,se.name AS session_name,ac.name AS class_name,c.code AS category_code,c.name_en AS category_name_en,c.name_bn AS category_name_bn,c.frequency"

func structureBase(db *gorm.DB) *gorm.DB {
	return db.Table("fee_structures fs").Joins("JOIN academic_sessions se ON se.id=fs.session_id").Joins("JOIN academic_classes ac ON ac.id=fs.class_id").Joins("JOIN fee_categories c ON c.id=fs.category_id")
}

func structureByID(tx *gorm.DB, id string) (Structure, error) {
	var s Structure
	err := structureBase(tx).Select(structureSelect).Where("fs.id=?", id).Take(&s).Error
	return s, dbErr(err)
}

func (r *GormRepository) ListStructures(ctx context.Context, q Query) (Page[Structure], error) {
	base := structureBase(r.db.WithContext(ctx))
	if q.SessionID != "" {
		base = base.Where("fs.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("fs.class_id=?", q.ClassID)
	}
	if q.CategoryID != "" {
		base = base.Where("fs.category_id=?", q.CategoryID)
	}
	return paginate[Structure](base, q, structureSelect, nil, "se.start_date DESC,ac.display_order,ac.name,c.code")
}

// SaveStructure keeps the session, class and category fixed after creation;
// amount changes affect only dues created afterwards.
func (r *GormRepository) SaveStructure(ctx context.Context, actor, id string, in StructureInput, cents int64) (Structure, error) {
	var out Structure
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSetup(tx); err != nil {
			return err
		}
		active := true
		if in.IsActive != nil {
			active = *in.IsActive
		}
		if id == "" {
			var ok int64
			if err := tx.Table("fee_categories").Where("id=? AND is_active", in.CategoryID).Count(&ok).Error; err != nil {
				return err
			}
			if ok == 0 {
				return ErrInvalid
			}
			if err := tx.Raw("INSERT INTO fee_structures(session_id,class_id,category_id,amount,due_day,is_active,created_by,updated_by) VALUES (?,?,?,?::numeric,?,?,?,?) RETURNING id", in.SessionID, in.ClassID, in.CategoryID, Decimal(cents), in.DueDay, active, actor, actor).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			old, err := structureByID(tx, id)
			if err != nil {
				return err
			}
			if old.SessionID != in.SessionID || old.ClassID != in.ClassID || old.CategoryID != in.CategoryID {
				return ErrConflict
			}
			if err := tx.Exec("UPDATE fee_structures SET amount=?::numeric,due_day=?,is_active=?,updated_by=?,updated_at=now() WHERE id=?", Decimal(cents), in.DueDay, active, actor, id).Error; err != nil {
				return dbErr(err)
			}
		}
		var err error
		if out, err = structureByID(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.structure_saved", "fee_structure", id, map[string]any{"amount": Decimal(cents), "due_day": in.DueDay})
	})
	return out, dbErr(err)
}

const concessionSelect = "fc.id,fc.student_id,fc.session_id,se.name AS session_name,fc.category_id,fc.percent,fc.reason,s.name_en AS student_name,s.admission_no,c.name_en AS category_name_en,c.name_bn AS category_name_bn"

func concessionBase(db *gorm.DB) *gorm.DB {
	return db.Table("fee_concessions fc").Joins("JOIN students s ON s.id=fc.student_id").Joins("JOIN fee_categories c ON c.id=fc.category_id").Joins("JOIN academic_sessions se ON se.id=fc.session_id")
}

func (r *GormRepository) ListConcessions(ctx context.Context, q Query) (Page[Concession], error) {
	base := concessionBase(r.db.WithContext(ctx))
	if q.StudentID != "" {
		base = base.Where("fc.student_id=?", q.StudentID)
	}
	if q.SessionID != "" {
		base = base.Where("fc.session_id=?", q.SessionID)
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(s.admission_no) LIKE ? OR lower(s.name_en) LIKE ?)", p, p)
	}
	return paginate[Concession](base, q, concessionSelect, nil, "se.start_date DESC,s.admission_no,c.code")
}

// SaveConcession creates or replaces a student's standing percentage for one
// category in one session (the student's current one unless given). It
// applies to that session's dues created afterwards, never to existing dues.
func (r *GormRepository) SaveConcession(ctx context.Context, actor string, in ConcessionInput) (Concession, error) {
	var out Concession
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.SessionID == "" {
			if err := tx.Raw("SELECT session_id FROM students WHERE id=?", in.StudentID).Row().Scan(&in.SessionID); err != nil {
				return ErrNotFound
			}
		}
		var enrolled bool
		if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM student_enrolments WHERE student_id=? AND session_id=?)", in.StudentID, in.SessionID).Row().Scan(&enrolled); err != nil {
			return err
		}
		if !enrolled {
			return ErrInvalid
		}
		var id string
		if err := tx.Raw("INSERT INTO fee_concessions(student_id,session_id,category_id,percent,reason,created_by) VALUES (?,?,?,?,?,?) ON CONFLICT (student_id,session_id,category_id) DO UPDATE SET percent=EXCLUDED.percent,reason=EXCLUDED.reason RETURNING id", in.StudentID, in.SessionID, in.CategoryID, in.Percent, in.Reason, actor).Row().Scan(&id); err != nil {
			return dbErr(err)
		}
		if err := concessionBase(tx).Select(concessionSelect).Where("fc.id=?", id).Take(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.concession_saved", "student", in.StudentID, map[string]any{"category_id": in.CategoryID, "session_id": in.SessionID, "percent": in.Percent})
	})
	return out, dbErr(err)
}

// ReviewConcessions lists the concessions of one session held by students
// enrolled in another (usually the next) session, with the percentage
// already set there if any, so the office can decide them again.
func (r *GormRepository) ReviewConcessions(ctx context.Context, from, to string) ([]ConcessionReview, error) {
	out := []ConcessionReview{}
	err := concessionBase(r.db.WithContext(ctx)).
		Joins("JOIN student_enrolments en ON en.student_id=fc.student_id AND en.session_id=?", to).
		Joins("JOIN academic_classes tc ON tc.id=en.class_id").Joins("JOIN academic_sections tsec ON tsec.id=en.section_id").
		Joins("LEFT JOIN fee_concessions nfc ON nfc.student_id=fc.student_id AND nfc.category_id=fc.category_id AND nfc.session_id=?", to).
		Select(concessionSelect+",en.session_id AS to_session_id,tc.name AS to_class_name,tsec.name AS to_section_name,nfc.percent AS to_percent").
		Where("fc.session_id=?", from).Order("tc.display_order,tsec.name,s.admission_no,c.code").Scan(&out).Error
	return out, err
}

func (r *GormRepository) DeleteConcession(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var studentID string
		if err := tx.Raw("DELETE FROM fee_concessions WHERE id=? RETURNING student_id", id).Row().Scan(&studentID); err != nil {
			return ErrNotFound
		}
		return audit.Record(tx, &actor, "fee.concession_deleted", "student", studentID, map[string]any{"concession_id": id})
	}))
}

const dueSelect = `sf.id,sf.student_id,sf.structure_id,sf.session_id,sf.class_id,to_char(sf.period,'YYYY-MM') AS period,to_char(sf.due_date,'YYYY-MM-DD') AS due_date,sf.amount,sf.discount_amount,sf.paid_amount,sf.balance,
CASE WHEN sf.balance=0 AND sf.paid_amount=0 THEN 'waived' WHEN sf.balance=0 THEN 'paid' WHEN sf.paid_amount>0 THEN 'partial' ELSE 'unpaid' END AS status,
(sf.balance>0 AND sf.due_date < ?::date) AS overdue,c.id AS category_id,c.code AS category_code,c.name_en AS category_name_en,c.name_bn AS category_name_bn,
s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,se.name AS session_name,ac.name AS class_name`

func dueBase(db *gorm.DB) *gorm.DB {
	return db.Table("student_fees sf").Joins("JOIN fee_structures fs ON fs.id=sf.structure_id").Joins("JOIN fee_categories c ON c.id=fs.category_id").Joins("JOIN students s ON s.id=sf.student_id").Joins("JOIN academic_sessions se ON se.id=sf.session_id").Joins("JOIN academic_classes ac ON ac.id=sf.class_id")
}

func dueByID(tx *gorm.DB, id, today string) (Due, error) {
	var d Due
	err := dueBase(tx).Select(dueSelect, today).Where("sf.id=?", id).Take(&d).Error
	return d, dbErr(err)
}

func todayDhaka() string { return time.Now().In(Dhaka).Format(time.DateOnly) }

func (r *GormRepository) ListDues(ctx context.Context, q Query) (Page[Due], error) {
	base := dueBase(r.db.WithContext(ctx))
	if q.SessionID != "" {
		base = base.Where("sf.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		base = base.Where("sf.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		base = base.Where("s.section_id=?", q.SectionID)
	}
	if q.StudentID != "" {
		base = base.Where("sf.student_id=?", q.StudentID)
	}
	if q.CategoryID != "" {
		base = base.Where("fs.category_id=?", q.CategoryID)
	}
	if q.Period != "" {
		base = base.Where("sf.period=to_date(?,'YYYY-MM')", q.Period)
	}
	switch q.Status {
	case "unpaid":
		base = base.Where("sf.balance>0 AND sf.paid_amount=0")
	case "partial":
		base = base.Where("sf.balance>0 AND sf.paid_amount>0")
	case "paid":
		base = base.Where("sf.balance=0 AND sf.paid_amount>0")
	case "waived":
		base = base.Where("sf.balance=0 AND sf.paid_amount=0")
	case "outstanding":
		base = base.Where("sf.balance>0")
	case "overdue":
		base = base.Where("sf.balance>0 AND sf.due_date < ?::date", q.Today)
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(s.admission_no) LIKE ? OR lower(s.name_en) LIKE ? OR s.name_bn LIKE ?)", p, p, likePattern(q.Search))
	}
	return paginate[Due](base, q, dueSelect, []any{q.Today}, "sf.due_date DESC,s.admission_no,c.code,sf.id")
}

type structureContext struct {
	ID        string
	SessionID string
	ClassID   string
	Category  string
	Frequency string
	Amount    float64
	DueDay    int
	IsActive  bool
	CatActive bool
}

// periodWithinSession reports whether the month overlaps the session dates.
func periodWithinSession(tx *gorm.DB, sessionID, month string) (bool, error) {
	var n int64
	err := tx.Table("academic_sessions").Where("id=? AND date_trunc('month',start_date)::date <= ?::date AND end_date >= ?::date", sessionID, month, month).Count(&n).Error
	return n > 0, err
}

func (r *GormRepository) AssignDue(ctx context.Context, actor string, in DueInput) (Due, error) {
	var out Due
	today := todayDhaka()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStudent(tx, in.StudentID); err != nil {
			return err
		}
		var st structureContext
		if err := tx.Table("fee_structures fs").Joins("JOIN fee_categories c ON c.id=fs.category_id").Select("fs.id,fs.session_id,fs.class_id,c.id AS category,c.frequency,fs.amount,fs.due_day,fs.is_active,c.is_active AS cat_active").Where("fs.id=?", in.StructureID).Take(&st).Error; err != nil {
			return dbErr(err)
		}
		var student struct{ SessionID, ClassID string }
		if err := tx.Table("students").Select("session_id,class_id").Where("id=?", in.StudentID).Take(&student).Error; err != nil {
			return dbErr(err)
		}
		if student.SessionID != st.SessionID || student.ClassID != st.ClassID {
			return ErrInvalid
		}
		if !st.IsActive || !st.CatActive {
			return ErrConflict
		}
		var period any
		dueDate := today
		if in.Period != "" {
			month := in.Period + "-01"
			inside, err := periodWithinSession(tx, st.SessionID, month)
			if err != nil {
				return err
			}
			if !inside {
				return ErrInvalid
			}
			first, _ := time.Parse(time.DateOnly, month)
			dueDate = first.AddDate(0, 0, st.DueDay-1).Format(time.DateOnly)
			if st.Frequency == "monthly" {
				period = month
			}
		} else if st.Frequency == "monthly" {
			return ErrInvalid
		}
		amount, _ := ToCents(st.Amount)
		var percent float64
		if err := tx.Raw("SELECT COALESCE((SELECT percent FROM fee_concessions WHERE student_id=? AND category_id=? AND session_id=?),0)", in.StudentID, st.Category, st.SessionID).Row().Scan(&percent); err != nil {
			return err
		}
		discount := PercentOf(amount, percent)
		var id string
		if err := tx.Raw("INSERT INTO student_fees(student_id,structure_id,session_id,class_id,period,due_date,amount,discount_amount,created_by) VALUES (?,?,?,?,?::date,?::date,?::numeric,?::numeric,?) RETURNING id", in.StudentID, st.ID, st.SessionID, st.ClassID, period, dueDate, Decimal(amount), Decimal(discount), actor).Row().Scan(&id); err != nil {
			return dbErr(err)
		}
		if discount > 0 {
			if err := tx.Exec("INSERT INTO fee_adjustments(student_fee_id,kind,amount,reason,created_by) VALUES (?,'discount',?::numeric,'Standing concession',?)", id, Decimal(discount), actor).Error; err != nil {
				return dbErr(err)
			}
		}
		var err error
		if out, err = dueByID(tx, id, today); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.due_assigned", "student", in.StudentID, map[string]any{"student_fee_id": id, "amount": Decimal(amount), "discount": Decimal(discount)})
	})
	return out, dbErr(err)
}

// GenerateDues creates one due per active student and active structure for the
// month. Existing dues are skipped by the unique key, so reruns are safe.
func (r *GormRepository) GenerateDues(ctx context.Context, actor string, in GenerateInput) (GenerateResult, error) {
	var out GenerateResult
	month := in.Period + "-01"
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSetup(tx); err != nil {
			return err
		}
		inside, err := periodWithinSession(tx, in.SessionID, month)
		if err != nil {
			return err
		}
		if !inside {
			return ErrInvalid
		}
		row := tx.Raw(`WITH candidates AS (
  SELECT s.student_id, fs.id AS structure_id, fs.session_id, fs.class_id,
         CASE WHEN c.frequency='monthly' THEN ?::date END AS period,
         ?::date + (fs.due_day - 1) AS due_date, fs.amount,
         COALESCE(round(fs.amount * fc.percent / 100, 2), 0) AS discount_amount
  FROM fee_structures fs
  JOIN fee_categories c ON c.id=fs.category_id AND c.is_active
  JOIN student_rolls s ON s.session_id=fs.session_id AND s.class_id=fs.class_id
  LEFT JOIN fee_concessions fc ON fc.student_id=s.student_id AND fc.category_id=c.id AND fc.session_id=fs.session_id
  WHERE fs.session_id=? AND fs.is_active AND (?='' OR fs.class_id::text=?) AND (c.frequency='monthly' OR ?)
), inserted AS (
  INSERT INTO student_fees(student_id,structure_id,session_id,class_id,period,due_date,amount,discount_amount,created_by)
  SELECT student_id,structure_id,session_id,class_id,period,due_date,amount,discount_amount,? FROM candidates
  ON CONFLICT ON CONSTRAINT student_fees_period_key DO NOTHING
  RETURNING id, discount_amount
), concessions AS (
  INSERT INTO fee_adjustments(student_fee_id,kind,amount,reason,created_by)
  SELECT id,'discount',discount_amount,'Standing concession',? FROM inserted WHERE discount_amount > 0
)
SELECT (SELECT count(*) FROM candidates), (SELECT count(*) FROM inserted)`, month, month, in.SessionID, in.ClassID, in.ClassID, in.IncludeOneTime, actor, actor).Row()
		var candidates int64
		if err := row.Scan(&candidates, &out.Created); err != nil {
			return dbErr(err)
		}
		out.Skipped = candidates - out.Created
		return audit.Record(tx, &actor, "fee.dues_generated", "academic_session", in.SessionID, map[string]any{"period": in.Period, "class_id": in.ClassID, "created": out.Created, "skipped": out.Skipped})
	})
	return out, dbErr(err)
}

// DeleteDue removes a mistaken due only while no money has been applied.
func (r *GormRepository) DeleteDue(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var studentID string
		if err := tx.Raw("SELECT student_id FROM student_fees WHERE id=?", id).Row().Scan(&studentID); err != nil {
			return ErrNotFound
		}
		if err := lockStudent(tx, studentID); err != nil {
			return err
		}
		var paid float64
		if err := tx.Raw("SELECT paid_amount FROM student_fees WHERE id=? FOR UPDATE", id).Row().Scan(&paid); err != nil {
			return ErrNotFound
		}
		var allocations int64
		if err := tx.Table("fee_payment_allocations").Where("student_fee_id=?", id).Count(&allocations).Error; err != nil {
			return err
		}
		if paid > 0 || allocations > 0 {
			return ErrConflict
		}
		if err := tx.Exec("DELETE FROM student_fees WHERE id=?", id).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &actor, "fee.due_deleted", "student", studentID, map[string]any{"student_fee_id": id})
	}))
}

func (r *GormRepository) Adjust(ctx context.Context, actor, kind, dueID string, cents int64, reason string) (Due, error) {
	var out Due
	today := todayDhaka()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var studentID string
		if err := tx.Raw("SELECT student_id FROM student_fees WHERE id=?", dueID).Row().Scan(&studentID); err != nil {
			return ErrNotFound
		}
		if err := lockStudent(tx, studentID); err != nil {
			return err
		}
		var balance float64
		if err := tx.Raw("SELECT balance FROM student_fees WHERE id=? FOR UPDATE", dueID).Row().Scan(&balance); err != nil {
			return ErrNotFound
		}
		remaining, _ := ToCents(balance)
		if remaining == 0 {
			return ErrConflict
		}
		if kind == "waiver" {
			cents = remaining
		}
		if cents > remaining {
			return ErrOverpayment
		}
		if err := tx.Exec("UPDATE student_fees SET discount_amount=discount_amount+?::numeric,updated_at=now() WHERE id=?", Decimal(cents), dueID).Error; err != nil {
			return dbErr(err)
		}
		if err := tx.Exec("INSERT INTO fee_adjustments(student_fee_id,kind,amount,reason,created_by) VALUES (?,?,?::numeric,?,?)", dueID, kind, Decimal(cents), reason, actor).Error; err != nil {
			return dbErr(err)
		}
		var err error
		if out, err = dueByID(tx, dueID, today); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.due_"+kind, "student", studentID, map[string]any{"student_fee_id": dueID, "amount": Decimal(cents)})
	})
	return out, dbErr(err)
}

func (r *GormRepository) Adjustments(ctx context.Context, dueID string) ([]Adjustment, error) {
	out := []Adjustment{}
	err := r.db.WithContext(ctx).Table("fee_adjustments").Select("id,student_fee_id,kind,amount,reason,to_char(created_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS created_at").Where("student_fee_id=?", dueID).Order("created_at,id").Find(&out).Error
	return out, err
}

func (r *GormRepository) RecordPayment(ctx context.Context, actor string, rec PaymentRecord) (string, bool, error) {
	var id string
	var replay bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		id, replay, err = recordPayment(tx, actor, rec)
		return err
	})
	return id, replay, dbErr(err)
}

// recordPayment must run inside a transaction. It allocates the amount to the
// oldest selected dues, issues the receipt and posts fee income to the ledger.
func recordPayment(tx *gorm.DB, actor string, rec PaymentRecord) (string, bool, error) {
	if err := lockStudent(tx, rec.StudentID); err != nil {
		return "", false, err
	}
	var existing struct {
		ID        string
		StudentID string
		Amount    float64
	}
	if err := tx.Raw("SELECT id,student_id,amount FROM fee_payments WHERE idempotency_key=?", rec.IdempotencyKey).Scan(&existing).Error; err != nil {
		return "", false, err
	}
	if existing.ID != "" {
		if prior, _ := ToCents(existing.Amount); existing.StudentID == rec.StudentID && prior == rec.AmountCents {
			return existing.ID, true, nil
		}
		return "", false, ErrDuplicate
	}
	var known int64
	if err := tx.Table("students").Where("id=?", rec.StudentID).Count(&known).Error; err != nil {
		return "", false, err
	}
	if known == 0 {
		return "", false, ErrNotFound
	}
	query := tx.Table("student_fees").Select("id,balance").Where("student_id=? AND balance>0", rec.StudentID)
	if len(rec.FeeIDs) > 0 {
		var owned int64
		if err := tx.Table("student_fees").Where("student_id=? AND id IN ?", rec.StudentID, rec.FeeIDs).Count(&owned).Error; err != nil {
			return "", false, dbErr(err)
		}
		if owned != int64(len(rec.FeeIDs)) {
			return "", false, ErrInvalid
		}
		query = query.Where("id IN ?", rec.FeeIDs)
	}
	var rows []struct {
		ID      string
		Balance float64
	}
	if err := query.Order("due_date,created_at,id").Clauses(clause.Locking{Strength: "UPDATE"}).Find(&rows).Error; err != nil {
		return "", false, err
	}
	dues := make([]Outstanding, 0, len(rows))
	for _, row := range rows {
		balance, _ := ToCents(row.Balance)
		dues = append(dues, Outstanding{ID: row.ID, Balance: balance})
	}
	splits, err := Allocate(rec.AmountCents, dues)
	if err != nil {
		return "", false, err
	}
	var id, receiptNo string
	if err := tx.Raw("INSERT INTO fee_payments(student_id,amount,method,source,reference,paid_on,note,idempotency_key,received_by) VALUES (?,?::numeric,?,?,?,?::date,?,?,?) RETURNING id,receipt_no", rec.StudentID, Decimal(rec.AmountCents), rec.Method, rec.Source, rec.Reference, rec.PaidOn, rec.Note, rec.IdempotencyKey, actor).Row().Scan(&id, &receiptNo); err != nil {
		return "", false, dbErr(err)
	}
	for _, split := range splits {
		if err := tx.Exec("INSERT INTO fee_payment_allocations(payment_id,student_fee_id,amount) VALUES (?,?,?::numeric)", id, split.ID, Decimal(split.Amount)).Error; err != nil {
			return "", false, dbErr(err)
		}
		if err := tx.Exec("UPDATE student_fees SET paid_amount=paid_amount+?::numeric,updated_at=now() WHERE id=?", Decimal(split.Amount), split.ID).Error; err != nil {
			return "", false, dbErr(err)
		}
	}
	if err := tx.Exec("INSERT INTO ledger_entries(entry_type,category,amount,entry_date,description,reference,payment_id,created_by) VALUES ('income','fees',?::numeric,?::date,?,?,?,?)", Decimal(rec.AmountCents), rec.PaidOn, "Fee receipt "+receiptNo, rec.Reference, id, actor).Error; err != nil {
		return "", false, dbErr(err)
	}
	if err := audit.Record(tx, &actor, "fee.payment_recorded", "fee_payment", id, map[string]any{"student_id": rec.StudentID, "amount": Decimal(rec.AmountCents), "method": rec.Method, "source": rec.Source, "receipt_no": receiptNo}); err != nil {
		return "", false, err
	}
	return id, false, nil
}

const paymentSelect = `p.id,p.receipt_no,p.student_id,p.amount,p.method,p.source,p.simulated,to_char(p.voided_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS voided_at,p.void_reason,p.reference,to_char(p.paid_on,'YYYY-MM-DD') AS paid_on,p.note,
CASE WHEN p.source='online' THEN '' ELSE COALESCE(NULLIF(u.display_name,''),u.email) END AS received_by_name,
to_char(p.created_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS created_at,s.name_en AS student_name,s.name_bn AS student_name_bn,s.admission_no,ac.name AS class_name,sec.name AS section_name`

func paymentBase(db *gorm.DB) *gorm.DB {
	return db.Table("fee_payments p").Joins("JOIN students s ON s.id=p.student_id").Joins("JOIN academic_classes ac ON ac.id=s.class_id").Joins("JOIN academic_sections sec ON sec.id=s.section_id").Joins("JOIN users u ON u.id=p.received_by")
}

func (r *GormRepository) ListPayments(ctx context.Context, q Query) (Page[Payment], error) {
	base := paymentBase(r.db.WithContext(ctx))
	if q.StudentID != "" {
		base = base.Where("p.student_id=?", q.StudentID)
	}
	if q.Method != "" {
		base = base.Where("p.method=?", q.Method)
	}
	switch q.Simulated {
	case "true":
		base = base.Where("p.simulated")
	case "false":
		base = base.Where("NOT p.simulated")
	case "unreviewed":
		base = base.Where("p.simulated AND p.voided_at IS NULL")
	}
	if q.From != "" {
		base = base.Where("p.paid_on>=?::date", q.From)
	}
	if q.To != "" {
		base = base.Where("p.paid_on<=?::date", q.To)
	}
	if q.ClassID != "" {
		base = base.Where("s.class_id=?", q.ClassID)
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(p.receipt_no) LIKE ? OR lower(p.reference) LIKE ? OR lower(s.admission_no) LIKE ? OR lower(s.name_en) LIKE ? OR s.name_bn LIKE ?)", p, p, p, p, likePattern(q.Search))
	}
	return paginate[Payment](base, q, paymentSelect, nil, "p.paid_on DESC,p.created_at DESC,p.id")
}

// Receipt reads one receipt; owner, when set, must match the paying student.
func (r *GormRepository) Receipt(ctx context.Context, paymentID, owner string) (Receipt, error) {
	db := r.db.WithContext(ctx)
	out := Receipt{Allocations: []Allocation{}}
	if err := paymentBase(db).Select(paymentSelect).Where("p.id=?", paymentID).Take(&out.Payment).Error; err != nil {
		return Receipt{}, dbErr(err)
	}
	if owner != "" && !strings.EqualFold(out.Payment.StudentID, owner) {
		return Receipt{}, ErrNotFound
	}
	if err := db.Table("fee_payment_allocations a").Joins("JOIN student_fees sf ON sf.id=a.student_fee_id").Joins("JOIN fee_structures fs ON fs.id=sf.structure_id").Joins("JOIN fee_categories c ON c.id=fs.category_id").
		Select("a.student_fee_id,a.amount,c.name_en AS category_name_en,c.name_bn AS category_name_bn,to_char(sf.period,'YYYY-MM') AS period,to_char(sf.due_date,'YYYY-MM-DD') AS due_date").
		Where("a.payment_id=?", paymentID).Order("sf.due_date,c.code").Find(&out.Allocations).Error; err != nil {
		return Receipt{}, err
	}
	if err := db.Raw("SELECT COALESCE(sum(balance),0) FROM student_fees WHERE student_id=?", out.Payment.StudentID).Row().Scan(&out.BalanceAfter); err != nil {
		return Receipt{}, err
	}
	err := db.Raw("SELECT name_en,name_bn,logo_url,address,phone FROM school_settings WHERE id=1").Row().Scan(&out.SchoolNameEN, &out.SchoolNameBN, &out.LogoURL, &out.Address, &out.Phone)
	return out, err
}

// ResolveStudent maps a self or guardian request to a permitted student id.
// Unknown and unlinked students both read as not found.
func (r *GormRepository) ResolveStudent(ctx context.Context, scope, actor, studentID string) (string, error) {
	db := r.db.WithContext(ctx)
	var id string
	switch scope {
	case "student":
		if err := db.Raw("SELECT id FROM students WHERE user_id=?", actor).Row().Scan(&id); err != nil {
			return "", ErrNotFound
		}
		if studentID != "" && !strings.EqualFold(studentID, id) {
			return "", ErrNotFound
		}
	case "guardian":
		if studentID == "" {
			return "", ErrInvalid
		}
		if err := db.Raw("SELECT s.id FROM students s JOIN student_guardians sg ON sg.student_id=s.id JOIN guardians g ON g.id=sg.guardian_id WHERE s.id=? AND g.user_id=?", studentID, actor).Row().Scan(&id); err != nil {
			return "", ErrNotFound
		}
	default:
		return "", ErrForbidden
	}
	return id, nil
}

func (r *GormRepository) Statement(ctx context.Context, studentID, today string) (Statement, error) {
	db := r.db.WithContext(ctx)
	out := Statement{Dues: []Due{}, Payments: []Payment{}}
	if err := studentBase(db).Select(studentSelect).Where("s.id=?", studentID).Take(&out.Student).Error; err != nil {
		return Statement{}, dbErr(err)
	}
	if err := dueBase(db).Select(dueSelect, today).Where("sf.student_id=?", studentID).Order("sf.due_date,c.code,sf.id").Find(&out.Dues).Error; err != nil {
		return Statement{}, err
	}
	if err := paymentBase(db).Select(paymentSelect).Where("p.student_id=?", studentID).Order("p.paid_on DESC,p.created_at DESC").Limit(200).Find(&out.Payments).Error; err != nil {
		return Statement{}, err
	}
	var charged, discount, paid, balance, overdue int64
	for _, d := range out.Dues {
		a, _ := ToCents(d.Amount)
		ds, _ := ToCents(d.DiscountAmount)
		p, _ := ToCents(d.PaidAmount)
		b, _ := ToCents(d.Balance)
		charged, discount, paid, balance = charged+a, discount+ds, paid+p, balance+b
		if d.Overdue {
			overdue += b
		}
	}
	out.Totals = Totals{Charged: float64(charged) / 100, Discount: float64(discount) / 100, Paid: float64(paid) / 100, Balance: float64(balance) / 100, Overdue: float64(overdue) / 100}
	err := db.Raw("SELECT COALESCE(sum(amount),0) FROM fee_payments WHERE student_id=? AND simulated AND voided_at IS NULL", studentID).Row().Scan(&out.Totals.SimulatedUnreviewed)
	return out, err
}

const intentSelect = "id,provider,provider_ref,student_id,payer_user_id,amount,status,to_char(expires_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI:SS') AS expires_at,(status='pending' AND expires_at < now()) AS expired,payment_id"

func (r *GormRepository) CreateIntent(ctx context.Context, actor, studentID, provider, reference string, cents int64, lifetime time.Duration) (Intent, error) {
	var out Intent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockStudent(tx, studentID); err != nil {
			return err
		}
		var outstanding float64
		if err := tx.Raw("SELECT COALESCE(sum(balance),0) FROM student_fees WHERE student_id=?", studentID).Row().Scan(&outstanding); err != nil {
			return err
		}
		if available, _ := ToCents(outstanding); cents > available {
			return ErrOverpayment
		}
		var id string
		if err := tx.Raw("INSERT INTO online_payment_intents(provider,provider_ref,student_id,payer_user_id,amount,expires_at) VALUES (?,?,?,?,?::numeric,now()+make_interval(secs => ?)) RETURNING id", provider, reference, studentID, actor, Decimal(cents), lifetime.Seconds()).Row().Scan(&id); err != nil {
			return dbErr(err)
		}
		if err := tx.Table("online_payment_intents").Select(intentSelect).Where("id=?", id).Take(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.online_payment_started", "student", studentID, map[string]any{"intent_id": id, "provider": provider, "amount": Decimal(cents)})
	})
	return out, dbErr(err)
}

func (r *GormRepository) Intent(ctx context.Context, actor, id string) (Intent, error) {
	var out Intent
	err := r.db.WithContext(ctx).Table("online_payment_intents").Select(intentSelect).Where("id=? AND payer_user_id=?", id, actor).Take(&out).Error
	return out, dbErr(err)
}

func (r *GormRepository) FailIntent(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Exec("UPDATE online_payment_intents SET status='failed',updated_at=now() WHERE id=? AND status='pending'", id).Error
}

// SimulateIntent closes an approved simulated checkout. It records no
// payment, receipt or ledger income and changes no balance; a repeated
// confirmation returns the same simulated intent.
func (r *GormRepository) SimulateIntent(ctx context.Context, actor, id string) (Intent, bool, error) {
	var intent Intent
	replay := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("online_payment_intents").Select(intentSelect).Where("id=? AND payer_user_id=?", id, actor).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&intent).Error; err != nil {
			return dbErr(err)
		}
		switch {
		case intent.Status == "simulated":
			replay = true
			return nil
		case intent.Status != "pending":
			return ErrConflict
		case intent.Expired:
			return ErrExpired
		}
		if err := tx.Exec("UPDATE online_payment_intents SET status='simulated',updated_at=now() WHERE id=?", id).Error; err != nil {
			return err
		}
		intent.Status = "simulated"
		return audit.Record(tx, &actor, "fee.online_payment_simulated", "student", intent.StudentID, map[string]any{"intent_id": id, "provider": intent.Provider, "amount": intent.Amount, "money_moved": false})
	})
	return intent, replay, dbErr(err)
}

// PaymentSettings reads the administrator's simulation switch.
func (r *GormRepository) PaymentSettings(ctx context.Context) (bool, string, error) {
	var enabled bool
	var updated string
	err := r.db.WithContext(ctx).Raw("SELECT simulation_enabled, to_char(updated_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') FROM payment_settings WHERE id=1").Row().Scan(&enabled, &updated)
	return enabled, updated, err
}

// SavePaymentSettings records the administrator's simulation switch.
func (r *GormRepository) SavePaymentSettings(ctx context.Context, actor string, enabled bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE payment_settings SET simulation_enabled=?,updated_by=?,updated_at=now() WHERE id=1", enabled, actor).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "fee.payment_simulation_changed", "school", "payments", map[string]any{"simulation_enabled": enabled})
	})
}

// VoidSimulatedPayment reverses a simulated payment recorded before the
// payment-safety upgrade: the dues it reduced are owed again. The payment,
// its allocations and its ledger line stay in the history, marked void.
func (r *GormRepository) VoidSimulatedPayment(ctx context.Context, actor, id, reason string) (Payment, error) {
	var out Payment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p struct {
			StudentID string
			Simulated bool
			Voided    bool
			Amount    float64
			ReceiptNo string
		}
		if err := tx.Raw("SELECT student_id,simulated,voided_at IS NOT NULL AS voided,amount,receipt_no FROM fee_payments WHERE id=? FOR UPDATE", id).Row().Scan(&p.StudentID, &p.Simulated, &p.Voided, &p.Amount, &p.ReceiptNo); err != nil {
			return dbErr(err)
		}
		if !p.Simulated {
			return ErrForbidden
		}
		if p.Voided {
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
		if err := tx.Exec("UPDATE fee_payments SET voided_at=now(),voided_by=?,void_reason=? WHERE id=?", actor, reason, id).Error; err != nil {
			return dbErr(err)
		}
		if err := audit.Record(tx, &actor, "fee.simulated_payment_voided", "fee_payment", id, map[string]any{"student_id": p.StudentID, "receipt_no": p.ReceiptNo, "amount": p.Amount, "dues_restored": len(allocations), "reason": reason}); err != nil {
			return err
		}
		return paymentBase(tx).Select(paymentSelect).Where("p.id=?", id).Take(&out).Error
	})
	return out, dbErr(err)
}

func mustCents(v float64) int64 {
	c, _ := ToCents(v)
	return c
}

const entrySelect = "le.id,le.entry_type,le.category,le.amount,COALESCE(fp.simulated,false) AS simulated,to_char(le.entry_date,'YYYY-MM-DD') AS entry_date,le.description,le.reference,le.payment_id,COALESCE(NULLIF(u.display_name,''),u.email) AS created_by_name"

func entryBase(db *gorm.DB) *gorm.DB {
	return db.Table("ledger_entries le").Joins("JOIN users u ON u.id=le.created_by").Joins("LEFT JOIN fee_payments fp ON fp.id=le.payment_id")
}

func (r *GormRepository) ListEntries(ctx context.Context, q Query) (Page[Entry], error) {
	base := entryBase(r.db.WithContext(ctx))
	if q.EntryType != "" {
		base = base.Where("le.entry_type=?", q.EntryType)
	}
	if q.Category != "" {
		base = base.Where("le.category=?", q.Category)
	}
	if q.From != "" {
		base = base.Where("le.entry_date>=?::date", q.From)
	}
	if q.To != "" {
		base = base.Where("le.entry_date<=?::date", q.To)
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		base = base.Where("(lower(le.description) LIKE ? OR lower(le.reference) LIKE ?)", p, p)
	}
	return paginate[Entry](base, q, entrySelect, nil, "le.entry_date DESC,le.created_at DESC,le.id")
}

func (r *GormRepository) SaveEntry(ctx context.Context, actor string, in EntryInput, cents int64) (Entry, error) {
	var out Entry
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id string
		if err := tx.Raw("INSERT INTO ledger_entries(entry_type,category,amount,entry_date,description,reference,created_by) VALUES (?,?,?::numeric,?::date,?,?,?) RETURNING id", in.EntryType, in.Category, Decimal(cents), in.EntryDate, in.Description, in.Reference, actor).Row().Scan(&id); err != nil {
			return dbErr(err)
		}
		if err := entryBase(tx).Select(entrySelect).Where("le.id=?", id).Take(&out).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "ledger.entry_recorded", "ledger_entry", id, map[string]any{"type": in.EntryType, "category": in.Category, "amount": Decimal(cents)})
	})
	return out, dbErr(err)
}

// DeleteEntry removes a manual entry. Fee income stays tied to its receipt.
func (r *GormRepository) DeleteEntry(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var entry struct {
			PaymentID *string
			Amount    float64
			EntryType string
		}
		if err := tx.Table("ledger_entries").Select("payment_id,amount,entry_type").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&entry).Error; err != nil {
			return dbErr(err)
		}
		if entry.PaymentID != nil {
			return ErrConflict
		}
		if err := tx.Exec("DELETE FROM ledger_entries WHERE id=?", id).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &actor, "ledger.entry_deleted", "ledger_entry", id, map[string]any{"type": entry.EntryType, "amount": entry.Amount})
	}))
}

func (r *GormRepository) Summary(ctx context.Context, from, to, today string) (Summary, error) {
	db := r.db.WithContext(ctx)
	out := Summary{From: from, To: to, IncomeByCategory: []Bucket{}, ExpenseByCategory: []Bucket{}}
	var rows []struct {
		EntryType string
		Bucket
	}
	if err := db.Table("ledger_entries").Select("entry_type,category AS key,sum(amount) AS total,count(*) AS count").Where("entry_date BETWEEN ?::date AND ?::date", from, to).
		Where("NOT EXISTS (SELECT 1 FROM fee_payments sp WHERE sp.id=ledger_entries.payment_id AND sp.simulated)").Group("entry_type,category").Order("entry_type,total DESC").Find(&rows).Error; err != nil {
		return out, err
	}
	var income, expense int64
	for _, row := range rows {
		cents, _ := ToCents(row.Total)
		if row.EntryType == "income" {
			income += cents
			out.IncomeByCategory = append(out.IncomeByCategory, row.Bucket)
		} else {
			expense += cents
			out.ExpenseByCategory = append(out.ExpenseByCategory, row.Bucket)
		}
	}
	out.Income, out.Expense, out.Net = float64(income)/100, float64(expense)/100, float64(income-expense)/100
	err := db.Raw("SELECT COALESCE(sum(balance),0), COALESCE(sum(balance) FILTER (WHERE due_date < ?::date),0) FROM student_fees", today).Row().Scan(&out.Outstanding, &out.Arrears)
	return out, err
}

func (r *GormRepository) Collections(ctx context.Context, q Query) (CollectionReport, error) {
	db := r.db.WithContext(ctx)
	out := CollectionReport{From: q.From, To: q.To, ByMethod: []Bucket{}, ByClass: []Bucket{}, ByCategory: []Bucket{}, ByDay: []Bucket{}}
	includeSimulated := false
	base := func() *gorm.DB {
		b := db.Table("fee_payment_allocations a").Joins("JOIN fee_payments p ON p.id=a.payment_id").Joins("JOIN student_fees sf ON sf.id=a.student_fee_id").Joins("JOIN fee_structures fs ON fs.id=sf.structure_id").Joins("JOIN fee_categories c ON c.id=fs.category_id").Joins("JOIN academic_classes ac ON ac.id=sf.class_id").
			Where("p.paid_on BETWEEN ?::date AND ?::date", q.From, q.To)
		if !includeSimulated {
			b = b.Where("NOT p.simulated")
		} else {
			b = b.Where("p.simulated")
		}
		if q.SessionID != "" {
			b = b.Where("sf.session_id=?", q.SessionID)
		}
		if q.ClassID != "" {
			b = b.Where("sf.class_id=?", q.ClassID)
		}
		if q.Method != "" {
			b = b.Where("p.method=?", q.Method)
		}
		return b
	}
	if err := base().Select("COALESCE(sum(a.amount),0) AS total, count(DISTINCT p.id) AS count").Row().Scan(&out.Total, &out.Count); err != nil {
		return out, err
	}
	includeSimulated = true
	if err := base().Select("COALESCE(sum(a.amount),0) AS total, count(DISTINCT p.id) AS count").Row().Scan(&out.SimulatedTotal, &out.SimulatedCount); err != nil {
		return out, err
	}
	includeSimulated = false
	groups := []struct {
		target *[]Bucket
		sel    string
		group  string
		order  string
	}{
		{&out.ByMethod, "p.method AS key,sum(a.amount) AS total,count(DISTINCT p.id) AS count", "p.method", "total DESC"},
		{&out.ByClass, "ac.id::text AS key,ac.name AS name_en,ac.name AS name_bn,sum(a.amount) AS total,count(DISTINCT p.id) AS count", "ac.id,ac.name,ac.display_order", "ac.display_order,ac.name"},
		{&out.ByCategory, "c.code AS key,c.name_en,c.name_bn,sum(a.amount) AS total,count(DISTINCT p.id) AS count", "c.code,c.name_en,c.name_bn", "total DESC"},
		{&out.ByDay, "to_char(p.paid_on,'YYYY-MM-DD') AS key,sum(a.amount) AS total,count(DISTINCT p.id) AS count", "p.paid_on", "p.paid_on"},
	}
	for _, g := range groups {
		if err := base().Select(g.sel).Group(g.group).Order(g.order).Find(g.target).Error; err != nil {
			return out, err
		}
	}
	return out, nil
}

// DueReport lists per-student balances and arrears, largest arrears first.
func (r *GormRepository) DueReport(ctx context.Context, q Query) (DueReport, error) {
	out := DueReport{Items: []DueReportRow{}, Page: q.Page, PageSize: q.PageSize}
	where := []string{"TRUE"}
	args := []any{}
	add := func(clause string, values ...any) { where = append(where, clause); args = append(args, values...) }
	if q.SessionID != "" {
		add("sf.session_id=?", q.SessionID)
	}
	if q.ClassID != "" {
		add("sf.class_id=?", q.ClassID)
	}
	if q.SectionID != "" {
		add("s.section_id=?", q.SectionID)
	}
	if q.Search != "" {
		p := likePattern(strings.ToLower(q.Search))
		add("(lower(s.admission_no) LIKE ? OR lower(s.name_en) LIKE ? OR s.name_bn LIKE ?)", p, p, likePattern(q.Search))
	}
	having := "sum(sf.balance) > 0"
	if q.OverdueOnly {
		having = "sum(sf.balance) FILTER (WHERE sf.due_date < ?::date) > 0"
	}
	grouped := fmt.Sprintf(`SELECT s.id AS student_id,s.admission_no,s.name_en,s.name_bn,ac.name AS class_name,sec.name AS section_name,
 sum(sf.amount) AS charged,sum(sf.discount_amount) AS discount,sum(sf.paid_amount) AS paid,sum(sf.balance) AS balance,
 COALESCE(sum(sf.balance) FILTER (WHERE sf.due_date < ?::date),0) AS overdue,
 to_char(min(sf.due_date) FILTER (WHERE sf.balance > 0),'YYYY-MM-DD') AS oldest_due
FROM student_fees sf JOIN students s ON s.id=sf.student_id JOIN academic_classes ac ON ac.id=s.class_id JOIN academic_sections sec ON sec.id=s.section_id
WHERE %s GROUP BY s.id,s.admission_no,s.name_en,s.name_bn,ac.name,sec.name HAVING %s`, strings.Join(where, " AND "), having)
	groupedArgs := append([]any{q.Today}, args...)
	if q.OverdueOnly {
		groupedArgs = append(groupedArgs, q.Today)
	}
	db := r.db.WithContext(ctx)
	if err := db.Raw("SELECT count(*),COALESCE(sum(balance),0),COALESCE(sum(overdue),0) FROM ("+grouped+") g", groupedArgs...).Row().Scan(&out.Total, &out.TotalBalance, &out.TotalOverdue); err != nil {
		return out, err
	}
	pageArgs := append(append([]any{}, groupedArgs...), q.PageSize, (q.Page-1)*q.PageSize)
	err := db.Raw(grouped+" ORDER BY overdue DESC,balance DESC,s.admission_no LIMIT ? OFFSET ?", pageArgs...).Scan(&out.Items).Error
	return out, err
}
