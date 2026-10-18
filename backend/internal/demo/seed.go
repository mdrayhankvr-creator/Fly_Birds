// Package demo seeds a fictional, repeatable demonstration school for visual
// review and training. Every row it creates is recorded in demo_records under
// a stable reference, so running it again adds nothing twice. It never updates
// or deletes rows it did not create; the only shared settings it touches are
// module switches (turned on) and, while the school still has the default
// "School" name, the school name and colours.
package demo

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"gorm.io/gorm"
)

// Domain of every demo account; it is reserved for documentation and testing.
const Domain = "demo.flybirds.test"

var ErrPassword = errors.New("demo password must be 12 to 72 bytes")

type Options struct {
	Password string    // password for newly created demo accounts
	Now      time.Time // reference date for attendance, fees and diaries
}

// Report counts what one run created; a repeat run reports mostly zeros.
type Report struct {
	Created  map[string]int `json:"created"`
	Accounts []string       `json:"accounts"`
	Session  string         `json:"session"`
}

type seeder struct {
	db     *gorm.DB
	auth   *auth.Service
	ctx    context.Context
	report Report
	admin  string
	today  time.Time
}

var tables = map[string]string{
	"user": "users", "session": "academic_sessions", "class": "academic_classes", "subject": "academic_subjects", "section": "academic_sections",
	"employee": "employees", "student": "students", "guardian": "guardians", "exam": "exams", "fee_category": "fee_categories",
	"fee_structure": "fee_structures", "student_fee": "student_fees", "payment": "fee_payments", "notice": "notices", "event": "school_events",
	"routine": "class_routines", "diary": "class_diaries", "ledger": "ledger_entries", "assignment": "teacher_assignments",
}

// ensure returns the recorded row for (kind, ref), creating it when the record
// is missing or the row was deleted since.
func (s *seeder) ensure(kind, ref string, create func(tx *gorm.DB) (string, error)) (string, error) {
	var id string
	err := s.db.WithContext(s.ctx).Raw("SELECT record_id::text FROM demo_records WHERE kind = ? AND ref = ?", kind, ref).Row().Scan(&id)
	if err == nil {
		var exists bool
		if err := s.db.WithContext(s.ctx).Raw("SELECT EXISTS (SELECT 1 FROM "+tables[kind]+" WHERE id = ?)", id).Row().Scan(&exists); err != nil {
			return "", err
		}
		if exists {
			return id, nil
		}
	}
	err = s.db.WithContext(s.ctx).Transaction(func(tx *gorm.DB) error {
		created, err := create(tx)
		if err != nil {
			return fmt.Errorf("%s %s: %w", kind, ref, err)
		}
		id = created
		return tx.Exec(`INSERT INTO demo_records (kind, ref, record_id) VALUES (?, ?, ?)
ON CONFLICT (kind, ref) DO UPDATE SET record_id = EXCLUDED.record_id, created_at = now()`, kind, ref, id).Error
	})
	if err == nil {
		s.report.Created[kind]++
	}
	return id, err
}

func scan(tx *gorm.DB, q string, args ...any) (string, error) {
	var id string
	err := tx.Raw(q, args...).Row().Scan(&id)
	return id, err
}

// stableUUID derives a repeatable UUID (for payment idempotency keys).
func stableUUID(ref string) string {
	h := md5.Sum([]byte("flybirds-demo:" + ref))
	h[6] = (h[6] & 0x0f) | 0x30
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

// free returns name unless a row the seed did not create already uses it, in
// which case the name gains a "(Demo)" suffix.
func (s *seeder) free(table, column, name string) (string, error) {
	var taken bool
	err := s.db.WithContext(s.ctx).Raw("SELECT EXISTS (SELECT 1 FROM "+table+" t WHERE lower(t."+column+") = lower(?) AND NOT EXISTS (SELECT 1 FROM demo_records d WHERE d.record_id = t.id))", name).Row().Scan(&taken)
	if err != nil || !taken {
		return name, err
	}
	return name + " (Demo)", nil
}

type person struct{ en, bn string }

var firstNames = []person{{"Ayesha", "আয়েশা"}, {"Rahim", "রহিম"}, {"Nusrat", "নুসরাত"}, {"Tanvir", "তানভীর"}, {"Mitu", "মিতু"}, {"Sakib", "সাকিব"},
	{"Lamia", "লামিয়া"}, {"Arif", "আরিফ"}, {"Puja", "পূজা"}, {"Fahim", "ফাহিম"}, {"Sadia", "সাদিয়া"}, {"Imran", "ইমরান"}}
var lastNames = []person{{"Rahman", "রহমান"}, {"Hossain", "হোসেন"}, {"Akter", "আক্তার"}, {"Islam", "ইসলাম"}, {"Saha", "সাহা"}, {"Chowdhury", "চৌধুরী"}}

// Seed creates (or completes) the demo school.
func Seed(ctx context.Context, db *gorm.DB, authService *auth.Service, opt Options) (Report, error) {
	if len(opt.Password) < 12 || len(opt.Password) > 72 {
		return Report{}, ErrPassword
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	n := opt.Now.In(time.FixedZone("Asia/Dhaka", 6*3600))
	s := &seeder{db: db, auth: authService, ctx: ctx, report: Report{Created: map[string]int{}}, today: time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)}
	err := s.run(opt.Password)
	return s.report, err
}

func (s *seeder) account(ref, role, name string, password string) (string, error) {
	email := ref + "@" + Domain
	id, err := s.ensure("user", ref, func(tx *gorm.DB) (string, error) {
		var existing string
		if err := tx.Raw("SELECT id::text FROM users WHERE email = ?", email).Scan(&existing).Error; err != nil {
			return "", err
		}
		if existing != "" {
			return "", fmt.Errorf("%s already exists and was not created by the demo seed", email)
		}
		u, err := s.auth.BootstrapUser(s.ctx, email, password, role)
		if err != nil {
			return "", err
		}
		return u.ID, tx.Exec("UPDATE users SET display_name = ? WHERE id = ?", name, u.ID).Error
	})
	if err == nil {
		s.report.Accounts = append(s.report.Accounts, email+" ("+role+")")
	}
	return id, err
}

func (s *seeder) run(password string) error {
	db := s.db.WithContext(s.ctx)
	var err error
	if s.admin, err = s.account("demo.admin", "admin", "Demo Administrator", password); err != nil {
		return err
	}
	head, err := s.account("demo.head", "head_teacher", "Shahana Begum", password)
	if err != nil {
		return err
	}
	teacherUsers := []string{}
	for _, t := range []struct{ ref, name string }{{"demo.teacher", "Rina Akter"}, {"demo.teacher2", "Kamal Hossain"}, {"demo.teacher3", "Nasrin Sultana"}} {
		id, err := s.account(t.ref, "teacher", t.name, password)
		if err != nil {
			return err
		}
		teacherUsers = append(teacherUsers, id)
	}
	if _, err := s.account("demo.accounts", "accountant", "Jamal Uddin", password); err != nil {
		return err
	}
	studentUser, err := s.account("demo.student", "student", "Ayesha Rahman", password)
	if err != nil {
		return err
	}
	guardianUser, err := s.account("demo.guardian", "guardian", "Halima Rahman", password)
	if err != nil {
		return err
	}
	_ = head

	// School: name only while it is still the default, and modules on.
	if err := db.Exec(`UPDATE school_settings SET name_en = 'Green Valley Demo School', name_bn = 'গ্রিন ভ্যালি ডেমো স্কুল', primary_color = '#1D4ED8', secondary_color = '#0F172A', updated_at = now()
WHERE id = 1 AND name_en = 'School'`).Error; err != nil {
		return err
	}
	for _, m := range []string{"academics", "students", "staff", "attendance", "routine", "examinations", "fees", "notices", "diary", "analytics"} {
		if err := db.Exec("UPDATE school_modules SET enabled = true, updated_at = now() WHERE school_id = 1 AND module_key = ? AND NOT enabled", m).Error; err != nil {
			return err
		}
	}

	// Academic session: the active one if present, otherwise this year's.
	var session string
	if err := db.Raw("SELECT id::text FROM academic_sessions WHERE is_active").Scan(&session).Error; err != nil {
		return err
	}
	if session == "" {
		year := s.today.Year()
		if session, err = s.ensure("session", fmt.Sprint(year), func(tx *gorm.DB) (string, error) {
			name := fmt.Sprint(year)
			var existing string
			if err := tx.Raw("SELECT id::text FROM academic_sessions WHERE name = ?", name).Scan(&existing).Error; err != nil {
				return "", err
			}
			if existing != "" {
				return existing, tx.Exec("UPDATE academic_sessions SET is_active = true WHERE id = ? AND NOT EXISTS (SELECT 1 FROM academic_sessions WHERE is_active)", existing).Error
			}
			return scan(tx, "INSERT INTO academic_sessions (name, start_date, end_date, is_active) VALUES (?, ?, ?, true) RETURNING id", name, fmt.Sprintf("%d-01-01", year), fmt.Sprintf("%d-12-31", year))
		}); err != nil {
			return err
		}
	}
	if err := db.Raw("SELECT name FROM academic_sessions WHERE id = ?", session).Scan(&s.report.Session).Error; err != nil {
		return err
	}
	sessionStart := s.today.AddDate(0, 0, -60)
	var start time.Time
	if err := db.Raw("SELECT start_date FROM academic_sessions WHERE id = ?", session).Row().Scan(&start); err == nil && start.After(sessionStart) {
		sessionStart = start
	}

	// Subjects and classes.
	subjects := []struct{ code, en, bn string }{{"BAN", "Bangla", "বাংলা"}, {"ENG", "English", "ইংরেজি"}, {"MATH", "Mathematics", "গণিত"}}
	subjectIDs := []string{}
	for _, sub := range subjects {
		id, err := s.ensure("subject", sub.code, func(tx *gorm.DB) (string, error) {
			code, err := s.free("academic_subjects", "code", sub.code)
			if err != nil {
				return "", err
			}
			if code != sub.code { // the plain code belongs to the school's own subject
				code = "DEMO-" + sub.code
			}
			return scan(tx, "INSERT INTO academic_subjects (name_en, name_bn, code) VALUES (?, ?, ?) RETURNING id", sub.en, sub.bn, code)
		})
		if err != nil {
			return err
		}
		subjectIDs = append(subjectIDs, id)
	}
	employees := []string{}
	for i, t := range []struct{ code, en, bn, title string }{{"DEMO-T01", "Rina Akter", "রিনা আক্তার", "Assistant Teacher"}, {"DEMO-T02", "Kamal Hossain", "কামাল হোসেন", "Senior Teacher"}, {"DEMO-T03", "Nasrin Sultana", "নাসরিন সুলতানা", "Assistant Teacher"}} {
		uid := teacherUsers[i]
		id, err := s.ensure("employee", t.code, func(tx *gorm.DB) (string, error) {
			return scan(tx, "INSERT INTO employees (employee_code, user_id, kind, name_en, name_bn, designation, phone, joined_on) VALUES (?, ?, 'teacher', ?, ?, ?, '01700000000', '2021-01-10') RETURNING id", t.code, uid, t.en, t.bn, t.title)
		})
		if err != nil {
			return err
		}
		employees = append(employees, id)
	}
	if _, err := s.ensure("employee", "DEMO-S01", func(tx *gorm.DB) (string, error) {
		return scan(tx, "INSERT INTO employees (employee_code, kind, name_en, name_bn, designation, joined_on) VALUES ('DEMO-S01', 'non_teaching', 'Abul Kalam', 'আবুল কালাম', 'Office Assistant', '2019-03-01') RETURNING id")
	}); err != nil {
		return err
	}
	// Subject j (Bangla, English, Mathematics) is taught by employees[teach[j]]:
	// the demo teacher account (T01) teaches Mathematics, matching the diary.
	teach := []int{1, 2, 0}
	category, err := s.ensure("fee_category", "TUITION", func(tx *gorm.DB) (string, error) {
		code := "DEMO_TUITION"
		return scan(tx, "INSERT INTO fee_categories (code, name_en, name_bn, frequency, created_by, updated_by) VALUES (?, 'Monthly tuition', 'মাসিক বেতন', 'monthly', ?, ?) RETURNING id", code, s.admin, s.admin)
	})
	if err != nil {
		return err
	}

	type pupil struct{ id, class, section string }
	pupils := []pupil{}
	var demoGuardian string
	seq := 0
	for ci, cls := range []struct{ en string }{{"Class Six"}, {"Class Seven"}, {"Class Eight"}} {
		classID, err := s.ensure("class", cls.en, func(tx *gorm.DB) (string, error) {
			name, err := s.free("academic_classes", "name", cls.en)
			if err != nil {
				return "", err
			}
			return scan(tx, "INSERT INTO academic_classes (name, display_order) VALUES (?, ?) RETURNING id", name, 60+ci)
		})
		if err != nil {
			return err
		}
		for j, sub := range subjectIDs {
			if err := db.Exec("INSERT INTO academic_class_subjects (class_id, subject_id, is_required, is_fourth_subject) VALUES (?, ?, true, false) ON CONFLICT (class_id, subject_id) DO NOTHING", classID, sub).Error; err != nil {
				return err
			}
			if _, err := s.ensure("assignment", fmt.Sprintf("%s/%d", cls.en, j), func(tx *gorm.DB) (string, error) {
				var existing string
				if err := tx.Raw("SELECT id::text FROM teacher_assignments WHERE employee_id = ? AND session_id = ? AND class_id = ? AND subject_id = ?", employees[teach[j]], session, classID, sub).Scan(&existing).Error; err != nil || existing != "" {
					return existing, err
				}
				return scan(tx, "INSERT INTO teacher_assignments (employee_id, session_id, class_id, subject_id) VALUES (?, ?, ?, ?) RETURNING id", employees[teach[j]], session, classID, sub)
			}); err != nil {
				return err
			}
		}
		structure, err := s.ensure("fee_structure", cls.en, func(tx *gorm.DB) (string, error) {
			return scan(tx, "INSERT INTO fee_structures (session_id, class_id, category_id, amount, due_day, created_by, updated_by) VALUES (?, ?, ?, ?, 10, ?, ?) RETURNING id", session, classID, category, 800+ci*150, s.admin, s.admin)
		})
		if err != nil {
			return err
		}
		for si, code := range []string{"A", "B"} {
			secRef := cls.en + "/" + code
			sectionID, err := s.ensure("section", secRef, func(tx *gorm.DB) (string, error) {
				return scan(tx, "INSERT INTO academic_sections (session_id, class_id, name, code, display_order) VALUES (?, ?, ?, ?, ?) RETURNING id", session, classID, "Section "+code, "DEMO-"+code, si)
			})
			if err != nil {
				return err
			}
			// Weekly routine: three periods a day, Saturday to Thursday.
			secIndex := ci*2 + si
			for day := 0; day <= 6; day++ {
				if day == 5 { // Friday
					continue
				}
				for j := range subjectIDs {
					period := (secIndex + 2*j) % 6
					startMin := 9*60 + period*45
					if _, err := s.ensure("routine", fmt.Sprintf("%s/%d/%d", secRef, day, j), func(tx *gorm.DB) (string, error) {
						return scan(tx, "INSERT INTO class_routines (session_id, class_id, section_id, subject_id, teacher_id, day_of_week, start_minute, end_minute, room, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id",
							session, classID, sectionID, subjectIDs[j], employees[teach[j]], day, startMin, startMin+40, fmt.Sprintf("Room %d0%d", ci+1, si+1), s.admin, s.admin)
					}); err != nil {
						return err
					}
				}
			}
			examID, err := s.ensure("exam", secRef, func(tx *gorm.DB) (string, error) {
				return scan(tx, "INSERT INTO exams (name, exam_type, session_id, class_id, section_id, created_by, updated_by) VALUES ('Half Yearly Examination', 'half_yearly', ?, ?, ?, ?, ?) RETURNING id", session, classID, sectionID, s.admin, s.admin)
			})
			if err != nil {
				return err
			}
			for k := 0; k < 6; k++ {
				seq++
				first, last := firstNames[(seq-1)%len(firstNames)], lastNames[(seq+ci)%len(lastNames)]
				admission := fmt.Sprintf("DEMO-%03d", seq)
				var userID any
				if seq == 1 {
					userID = studentUser
				}
				studentID, err := s.ensure("student", admission, func(tx *gorm.DB) (string, error) {
					return scan(tx, `INSERT INTO students (admission_no, name_en, name_bn, date_of_birth, status, session_id, class_id, section_id, roll_no, user_id, created_at)
VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, now() - interval '120 days') RETURNING id`, admission, first.en+" "+last.en, first.bn+" "+last.bn, fmt.Sprintf("%d-0%d-1%d", 2014-ci, 1+k%9, k), session, classID, sectionID, k+1, userID)
				})
				if err != nil {
					return err
				}
				pupils = append(pupils, pupil{studentID, classID, sectionID})
				guardianID, err := s.ensure("guardian", admission, func(tx *gorm.DB) (string, error) {
					var gUser any
					name, nameBn := "Parent of "+first.en, first.bn+"-এর অভিভাবক"
					if seq == 1 {
						gUser, name, nameBn = guardianUser, "Halima Rahman", "হালিমা রহমান"
					}
					return scan(tx, "INSERT INTO guardians (name_en, name_bn, phone, user_id) VALUES (?, ?, ?, ?) RETURNING id", name, nameBn, fmt.Sprintf("0171%07d", 1000+seq), gUser)
				})
				if err != nil {
					return err
				}
				if seq == 1 {
					demoGuardian = guardianID
				}
				if seq == 2 && demoGuardian != "" { // the demo guardian has two children
					if err := db.Exec("INSERT INTO student_guardians (student_id, guardian_id, relationship, is_primary) VALUES (?, ?, 'Mother', false) ON CONFLICT DO NOTHING", studentID, demoGuardian).Error; err != nil {
						return err
					}
				}
				if seq != 4 {
					if err := db.Exec("INSERT INTO student_guardians (student_id, guardian_id, relationship, is_primary) VALUES (?, ?, 'Mother', true) ON CONFLICT DO NOTHING", studentID, guardianID).Error; err != nil {
						return err
					}
				}
				// Marks for the half-yearly exam.
				for j, sub := range subjectIDs {
					m := float64(38 + (seq*7+j*13)%60)
					if seq%10 == 7 && j == 2 {
						m = 24
					}
					if err := db.Exec(`INSERT INTO exam_subjects (exam_id, class_id, subject_id, full_marks, pass_marks) VALUES (?, ?, ?, 100, 33) ON CONFLICT (exam_id, subject_id) DO NOTHING`, examID, classID, sub).Error; err != nil {
						return err
					}
					if err := db.Exec(`INSERT INTO exam_marks (exam_id, exam_subject_id, student_id, session_id, class_id, section_id, marks, entered_by, updated_by)
SELECT ?, es.id, ?, ?, ?, ?, ?, ?, ? FROM exam_subjects es WHERE es.exam_id = ? AND es.subject_id = ?
ON CONFLICT (exam_subject_id, student_id) DO NOTHING`, examID, studentID, session, classID, sectionID, m, s.admin, s.admin, examID, sub).Error; err != nil {
						return err
					}
				}
				// Tuition for the last three months; most of it paid.
				amount := 800.0 + float64(ci)*150
				for mth := 0; mth < 3; mth++ {
					due := time.Date(s.today.Year(), s.today.Month(), 10, 0, 0, 0, 0, time.UTC).AddDate(0, mth-2, 0)
					ref := fmt.Sprintf("%s/%s", admission, due.Format("2006-01"))
					feeID, err := s.ensure("student_fee", ref, func(tx *gorm.DB) (string, error) {
						return scan(tx, "INSERT INTO student_fees (student_id, structure_id, session_id, class_id, period, due_date, amount, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id",
							studentID, structure, session, classID, due.Format("2006-01")+"-01", due.Format(time.DateOnly), amount, s.admin)
					})
					if err != nil {
						return err
					}
					if seq%4 == 0 || (mth == 2 && seq%3 == 0) || due.After(s.today) {
						continue
					}
					paidOn := due.AddDate(0, 0, -(seq % 6))
					if paidOn.After(s.today) {
						paidOn = s.today
					}
					method := []string{"cash", "bkash", "nagad", "bank"}[(seq+mth)%4]
					if _, err := s.ensure("payment", ref, func(tx *gorm.DB) (string, error) {
						reference := ""
						if method != "cash" {
							reference = strings.ToUpper(strings.ReplaceAll("DM"+ref, "/", ""))
							reference = strings.ReplaceAll(reference, "-", "")
						}
						pid, err := scan(tx, "INSERT INTO fee_payments (student_id, amount, method, source, reference, paid_on, idempotency_key, received_by) VALUES (?, ?, ?, 'counter', ?, ?, ?, ?) RETURNING id",
							studentID, amount, method, reference, paidOn.Format(time.DateOnly), stableUUID(ref), s.admin)
						if err != nil {
							return "", err
						}
						if err := tx.Exec("INSERT INTO fee_payment_allocations (payment_id, student_fee_id, amount) VALUES (?, ?, ?)", pid, feeID, amount).Error; err != nil {
							return "", err
						}
						if err := tx.Exec("UPDATE student_fees SET paid_amount = paid_amount + ? WHERE id = ?", amount, feeID).Error; err != nil {
							return "", err
						}
						return pid, tx.Exec("INSERT INTO ledger_entries (entry_type, category, amount, entry_date, description, payment_id, created_by) VALUES ('income', 'fees', ?, ?, 'Tuition receipt', ?, ?)", amount, paidOn.Format(time.DateOnly), pid, s.admin).Error
					}); err != nil {
						return err
					}
				}
			}
			// Publish the exam once (results and grades are computed by the exams module).
			var status string
			if err := db.Raw("SELECT status FROM exams WHERE id = ?", examID).Scan(&status).Error; err != nil {
				return err
			}
			if status == "draft" {
				if _, err := exams.NewRepository(s.db).SetState(s.ctx, s.admin, examID, "published"); err != nil {
					return fmt.Errorf("publish exam %s: %w", secRef, err)
				}
			}
		}
	}

	// Attendance for school days (Saturday to Thursday) up to yesterday, so
	// today's register is still open. Existing days are left as they are.
	from := s.today.AddDate(0, 0, -35)
	if from.Before(sessionStart) {
		from = sessionStart
	}
	for i, p := range pupils {
		seq := i + 1
		for day := from; day.Before(s.today); day = day.AddDate(0, 0, 1) {
			if day.Weekday() == time.Friday {
				continue
			}
			o := int(s.today.Sub(day).Hours() / 24)
			status := "present"
			switch {
			case seq == 9 && o <= 7:
				status = "absent" // a recent run of absences
			case seq == 14 && o > 3 && o%3 == 0:
				status = "absent"
			case seq == 23 && o <= 15:
				continue // has stopped coming
			case (seq+o)%11 == 0:
				status = "late"
			case (seq*3+o)%17 == 0:
				status = "absent"
			}
			if err := db.Exec(`INSERT INTO student_attendance (student_id, session_id, class_id, section_id, attendance_date, status, source, recorded_by, updated_by)
VALUES (?, ?, ?, ?, ?, ?, 'manual', ?, ?) ON CONFLICT (student_id, session_id, attendance_date) DO NOTHING`, p.id, session, p.class, p.section, day.Format(time.DateOnly), status, s.admin, s.admin).Error; err != nil {
				return err
			}
		}
	}
	for day := from; day.Before(s.today); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Friday {
			continue
		}
		for i, e := range employees {
			o := int(s.today.Sub(day).Hours() / 24)
			st := "present"
			if (o+i*5)%13 == 0 {
				st = "absent"
			} else if (o+i)%19 == 0 {
				st = "leave"
			}
			if err := db.Exec(`INSERT INTO teacher_attendance (session_id, employee_id, attendance_date, status, recorded_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (session_id, employee_id, attendance_date) DO NOTHING`, session, e, day.Format(time.DateOnly), st, s.admin, s.admin).Error; err != nil {
				return err
			}
		}
	}

	// Notices and events.
	notices := []struct {
		ref, en, bn, body, bodyBn, priority string
		roles                               string
	}{
		{"half-yearly-results", "Half-yearly results are published", "অর্ধবার্ষিক পরীক্ষার ফল প্রকাশিত হয়েছে", "Results are available on the results page. Guardians can discuss them with class teachers on Thursday.", "ফলাফল পাতায় ফল দেখা যাবে। অভিভাবকগণ বৃহস্পতিবার শ্রেণি শিক্ষকের সঙ্গে আলোচনা করতে পারবেন।", "important", "{head_teacher,teacher,student,guardian}"},
		{"fees-reminder", "Tuition fees for this month", "এই মাসের বেতন পরিশোধ", "Please pay this month's tuition by the 10th. bKash, Nagad, bank and counter payments are accepted.", "অনুগ্রহ করে ১০ তারিখের মধ্যে এই মাসের বেতন পরিশোধ করুন। বিকাশ, নগদ, ব্যাংক ও কাউন্টারে পরিশোধ করা যাবে।", "normal", "{student,guardian,accountant}"},
		{"staff-meeting", "Staff meeting on Sunday", "রবিবার শিক্ষক সভা", "All teachers meet in the library at 2:30 pm to plan the annual examination.", "বার্ষিক পরীক্ষার পরিকল্পনার জন্য সকল শিক্ষক দুপুর ২:৩০টায় গ্রন্থাগারে উপস্থিত থাকবেন।", "urgent", "{head_teacher,teacher}"},
	}
	for i, nt := range notices {
		if _, err := s.ensure("notice", nt.ref, func(tx *gorm.DB) (string, error) {
			return scan(tx, `INSERT INTO notices (title_en, title_bn, body_en, body_bn, priority, audience_roles, status, publish_at, created_by, updated_by)
VALUES (?, ?, ?, ?, ?, ?::text[], 'published', now() - make_interval(days => ?), ?, ?) RETURNING id`, nt.en, nt.bn, nt.body, nt.bodyBn, nt.priority, nt.roles, i+1, s.admin, s.admin)
		}); err != nil {
			return err
		}
	}
	events := []struct {
		ref, en, bn, kind string
		offset, length    int
	}{{"annual-sports", "Annual sports day", "বার্ষিক ক্রীড়া দিবস", "event", 6, 0}, {"victory-day", "Victory Day holiday", "বিজয় দিবসের ছুটি", "holiday", 14, 0}, {"annual-exam", "Annual examination begins", "বার্ষিক পরীক্ষা শুরু", "exam", 24, 6}, {"guardian-meeting", "Guardian meeting", "অভিভাবক সভা", "meeting", 3, 0}}
	for _, ev := range events {
		startDay := s.today.AddDate(0, 0, ev.offset)
		if _, err := s.ensure("event", ev.ref+"/"+startDay.Format("2006"), func(tx *gorm.DB) (string, error) {
			return scan(tx, `INSERT INTO school_events (title_en, title_bn, kind, starts_on, ends_on, location, audience_roles, status, created_by, updated_by)
VALUES (?, ?, ?, ?, ?, 'School campus', '{head_teacher,teacher,student,guardian,accountant}', 'published', ?, ?) RETURNING id`, ev.en, ev.bn, ev.kind, startDay.Format(time.DateOnly), startDay.AddDate(0, 0, ev.length).Format(time.DateOnly), s.admin, s.admin)
		}); err != nil {
			return err
		}
	}

	// Class diary: the mathematics teacher's last five school days.
	var sixA struct{ Class, Section string }
	if err := db.Raw("SELECT p.class_id AS class, p.section_id AS section FROM students p WHERE p.id = ?", pupils[0].id).Scan(&sixA).Error; err != nil {
		return err
	}
	count := 0
	for day := s.today; count < 5 && day.After(s.today.AddDate(0, 0, -14)); day = day.AddDate(0, 0, -1) {
		if day.Weekday() == time.Friday {
			continue
		}
		count++
		topic := []string{"Fractions: adding unlike denominators", "Ratio and proportion", "Simple equations", "Area of rectangles", "Revision: whole numbers"}[count-1]
		if _, err := s.ensure("diary", day.Format(time.DateOnly), func(tx *gorm.DB) (string, error) {
			return scan(tx, `INSERT INTO class_diaries (session_id, class_id, section_id, subject_id, teacher_user_id, diary_date, lesson_topics, class_notes, homework, status, published_at)
VALUES (?, ?, ?, ?, ?, ?, ?, 'Worked examples from the textbook; pairs checked each other''s answers.', 'Exercise 3.2, questions 1 to 6.', 'published', now()) RETURNING id`,
				session, sixA.Class, sixA.Section, subjectIDs[2], teacherUsers[0], day.Format(time.DateOnly), topic)
		}); err != nil {
			return err
		}
	}
	// One school expense so the ledger is not only fee income.
	month := s.today.AddDate(0, 0, -3)
	for i, e := range []struct {
		cat, desc string
		amount    float64
	}{{"salary", "Teacher salaries", 96000}, {"utilities", "Electricity and water", 6500}} {
		if _, err := s.ensure("ledger", fmt.Sprintf("%s/%s", e.cat, month.Format("2006-01")), func(tx *gorm.DB) (string, error) {
			return scan(tx, "INSERT INTO ledger_entries (entry_type, category, amount, entry_date, description, created_by) VALUES ('expense', ?, ?, ?, ?, ?) RETURNING id", e.cat, e.amount, month.AddDate(0, 0, -i).Format(time.DateOnly), e.desc, s.admin)
		}); err != nil {
			return err
		}
	}
	return nil
}
