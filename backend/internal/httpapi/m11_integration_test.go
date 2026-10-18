package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/analytics"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/dashboard"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

// TestPostgresM11Analytics builds an isolated school fixture through SQL and
// checks every analytics figure against values computed from that fixture,
// then the role and record isolation, module gates, validation, paging,
// empty data and CSV exports.
func TestPostgresM11Analytics(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
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
	if db.Table("schema_migrations").Where("version = 1101").Count(&applied); applied != 1 {
		t.Fatal("M11 migration 1101 not applied")
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x6b}, 32))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixNano()
	suffix := fmt.Sprintf("%d", now)
	tag := strings.ToUpper(fmt.Sprintf("%x", now))[:8]
	password := "M11TestPassword!" + suffix

	exec := func(q string, args ...any) {
		t.Helper()
		if err := db.Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	newID := func(q string, args ...any) string {
		t.Helper()
		var id string
		if err := db.Raw(q, args...).Row().Scan(&id); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return id
	}

	// Remember state the test changes, and remove everything it creates.
	modules := map[string]bool{}
	var moduleRows []struct {
		ModuleKey string
		Enabled   bool
	}
	db.Raw("SELECT module_key, enabled FROM school_modules WHERE school_id = 1").Scan(&moduleRows)
	for _, m := range moduleRows {
		modules[m.ModuleKey] = m.Enabled
	}
	var savedSettings analytics.Settings
	if savedSettings, err = analytics.NewRepository(db).Settings(ctx); err != nil {
		t.Fatal(err)
	}
	userIDs, sessionIDs, classIDs, subjectIDs, employeeIDs, guardianIDs, studentIDs := []string{}, []string{}, []string{}, []string{}, []string{}, []string{}, []string{}
	var categoryID string
	t.Cleanup(func() {
		steps := []struct {
			q    string
			args []any
		}{
			{"DELETE FROM ledger_entries WHERE payment_id IN (SELECT id FROM fee_payments WHERE student_id IN ?) OR created_by IN ?", []any{studentIDs, userIDs}},
			{"DELETE FROM fee_payment_allocations WHERE payment_id IN (SELECT id FROM fee_payments WHERE student_id IN ?)", []any{studentIDs}},
			{"DELETE FROM fee_payments WHERE student_id IN ?", []any{studentIDs}},
			{"DELETE FROM student_fees WHERE student_id IN ?", []any{studentIDs}},
			{"DELETE FROM fee_structures WHERE session_id IN ?", []any{sessionIDs}},
			{"DELETE FROM fee_categories WHERE id = ?", []any{categoryID}},
			{"DELETE FROM exams WHERE session_id IN ?", []any{sessionIDs}},
			{"DELETE FROM student_attendance WHERE session_id IN ?", []any{sessionIDs}},
			{"DELETE FROM teacher_attendance WHERE session_id IN ?", []any{sessionIDs}},
			{"DELETE FROM teacher_assignments WHERE session_id IN ?", []any{sessionIDs}},
			{"DELETE FROM student_guardians WHERE student_id IN ?", []any{studentIDs}},
			{"DELETE FROM guardians WHERE id IN ?", []any{guardianIDs}},
			{"DELETE FROM students WHERE id IN ?", []any{studentIDs}},
			{"DELETE FROM employees WHERE id IN ?", []any{employeeIDs}},
			{"DELETE FROM academic_class_subjects WHERE class_id IN ?", []any{classIDs}},
			{"DELETE FROM academic_sections WHERE session_id IN ?", []any{sessionIDs}},
			{"DELETE FROM academic_subjects WHERE id IN ?", []any{subjectIDs}},
			{"DELETE FROM academic_classes WHERE id IN ?", []any{classIDs}},
			{"DELETE FROM academic_sessions WHERE id IN ?", []any{sessionIDs}},
			{"DELETE FROM auth_sessions WHERE user_id IN ?", []any{userIDs}},
			{"DELETE FROM users WHERE id IN ?", []any{userIDs}},
		}
		for _, s := range steps {
			if err := db.Exec(s.q, s.args...).Error; err != nil {
				t.Errorf("cleanup %s: %v", s.q, err)
			}
		}
		for key, on := range modules {
			_ = db.Exec("UPDATE school_modules SET enabled = ? WHERE school_id = 1 AND module_key = ?", on, key).Error
		}
		_ = db.Exec(`UPDATE analytics_settings SET attendance_threshold = ?, attendance_window_days = ?, min_marked_days = ?, gpa_threshold = ?, consecutive_absences = ?, inactivity_days = ? WHERE id = 1`,
			savedSettings.AttendanceThreshold, savedSettings.AttendanceWindow, savedSettings.MinMarkedDays, savedSettings.GPAThreshold, savedSettings.ConsecutiveAbsences, savedSettings.InactivityDays).Error
	})
	// The test relies on the migrated analytics grants; restore every grant afterwards.
	var savedGrants []struct{ RoleID, PermissionID string }
	db.Raw("SELECT role_id, permission_id FROM role_permissions").Scan(&savedGrants)
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM role_permissions WHERE permission_id LIKE 'analytics:%'").Error
		for _, g := range savedGrants {
			_ = db.Exec("INSERT INTO role_permissions (role_id, permission_id) VALUES (?, ?) ON CONFLICT DO NOTHING", g.RoleID, g.PermissionID).Error
		}
	})
	exec(`INSERT INTO role_permissions (role_id, permission_id) VALUES ('admin', 'analytics:read'), ('admin', 'analytics:finance'), ('admin', 'analytics:manage'),
  ('head_teacher', 'analytics:read'), ('teacher', 'analytics:read_assigned'), ('accountant', 'analytics:finance'), ('student', 'analytics:read_self'), ('guardian', 'analytics:read_children')
ON CONFLICT DO NOTHING`)
	exec("UPDATE analytics_settings SET attendance_threshold = 75, attendance_window_days = 30, min_marked_days = 5, gpa_threshold = 2, consecutive_absences = 5, inactivity_days = 21 WHERE id = 1")
	for _, key := range []string{"academics", "students", "staff", "attendance", "examinations", "fees", "analytics"} {
		exec("UPDATE school_modules SET enabled = true WHERE school_id = 1 AND module_key = ?", key)
	}

	type actor struct {
		user    auth.User
		headers map[string]string
	}
	login := func(role string) actor {
		u, err := authService.BootstrapUser(ctx, fmt.Sprintf("m11-%s-%d-%s@example.test", role, len(userIDs), suffix), password, role)
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		c, err := authService.Login(ctx, u.Email, password)
		if err != nil {
			t.Fatal(err)
		}
		return actor{u, map[string]string{"Authorization": "Bearer " + c.AccessToken}}
	}
	adm, head, teacher, otherTeacher, accountant := login("admin"), login("head_teacher"), login("teacher"), login("teacher"), login("accountant")
	studentUser, guardianUser, strangerGuardian := login("student"), login("guardian"), login("guardian")

	// Fixture: today is the last of ten attendance days.
	nowDhaka := time.Now().In(analytics.Dhaka)
	today := time.Date(nowDhaka.Year(), nowDhaka.Month(), nowDhaka.Day(), 0, 0, 0, 0, time.UTC)
	d := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	year := 3000 + int(now%3000)
	session := newID("INSERT INTO academic_sessions (name, start_date, end_date) VALUES (?, ?, ?) RETURNING id", fmt.Sprintf("%04d-%04d", year, year+1), d(-100), d(200))
	emptySession := newID("INSERT INTO academic_sessions (name, start_date, end_date) VALUES (?, ?, ?) RETURNING id", fmt.Sprintf("%04d", year), d(-500), d(-401))
	sessionIDs = append(sessionIDs, session, emptySession)
	classA := newID("INSERT INTO academic_classes (name, display_order) VALUES (?, 1) RETURNING id", "M11 Class A "+suffix)
	classB := newID("INSERT INTO academic_classes (name, display_order) VALUES (?, 2) RETURNING id", "M11 Class B "+suffix)
	classIDs = append(classIDs, classA, classB)
	sub1 := newID("INSERT INTO academic_subjects (name_en, name_bn, code) VALUES ('Mathematics', 'গণিত', ?) RETURNING id", "M11M"+tag)
	sub2 := newID("INSERT INTO academic_subjects (name_en, name_bn, code) VALUES ('English', 'ইংরেজি', ?) RETURNING id", "M11E"+tag)
	subjectIDs = append(subjectIDs, sub1, sub2)
	for _, c := range []string{classA, classB} {
		for _, s := range []string{sub1, sub2} {
			exec("INSERT INTO academic_class_subjects (class_id, subject_id, is_required, is_fourth_subject) VALUES (?, ?, true, false)", c, s)
		}
	}
	section := func(class, code string, order int) string {
		return newID("INSERT INTO academic_sections (session_id, class_id, name, code, display_order) VALUES (?, ?, ?, ?, ?) RETURNING id", session, class, "Section "+code, code, order)
	}
	a1, a2, b1 := section(classA, "A1", 1), section(classA, "A2", 2), section(classB, "B1", 1)
	employee := func(u actor, code string) string {
		id := newID("INSERT INTO employees (employee_code, user_id, kind, name_en, name_bn, designation, joined_on) VALUES (?, ?, 'teacher', ?, 'শিক্ষক', 'Teacher', ?) RETURNING id", code+tag, u.user.ID, "Teacher "+code, d(-300))
		employeeIDs = append(employeeIDs, id)
		return id
	}
	t1, t2 := employee(teacher, "T1"), employee(otherTeacher, "T2")
	exec("INSERT INTO teacher_assignments (employee_id, session_id, class_id, subject_id) VALUES (?, ?, ?, ?)", t1, session, classA, sub1)
	exec("INSERT INTO teacher_assignments (employee_id, session_id, class_id, subject_id) VALUES (?, ?, ?, ?)", t2, session, classB, sub2)

	type pupil struct {
		id, class, section, admission string
	}
	pupils := map[string]pupil{}
	addStudent := func(key, class, sec string, roll int, status, nameEN string, user *actor) {
		var userID any
		if user != nil {
			userID = user.user.ID
		}
		admission := "M11-" + tag + "-" + key
		id := newID("INSERT INTO students (admission_no, name_en, name_bn, status, session_id, class_id, section_id, roll_no, user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id",
			admission, nameEN, "শিক্ষার্থী "+key, status, session, class, sec, roll, userID)
		studentIDs = append(studentIDs, id)
		pupils[key] = pupil{id, class, sec, admission}
	}
	addStudent("s1", classA, a1, 1, "active", "=HYPERLINK(\"x\") Rahim", &studentUser)
	addStudent("s2", classA, a1, 2, "active", "Karim", nil)
	addStudent("s3", classA, a1, 3, "active", "Salma", nil)
	addStudent("s4", classA, a1, 4, "active", "Nadia", nil)
	addStudent("s8", classA, a1, 8, "inactive", "Former", nil)
	addStudent("s5", classA, a2, 1, "active", "Imran", nil)
	addStudent("s6", classB, b1, 1, "active", "Faria", nil)
	addStudent("s7", classB, b1, 2, "active", "Tanvir", nil)
	exec("UPDATE students SET created_at = now() - interval '60 days' WHERE id = ?", pupils["s6"].id)
	guardian := newID("INSERT INTO guardians (name_en, user_id) VALUES ('Guardian One', ?) RETURNING id", guardianUser.user.ID)
	stranger := newID("INSERT INTO guardians (name_en, user_id) VALUES ('Guardian Two', ?) RETURNING id", strangerGuardian.user.ID)
	guardianIDs = append(guardianIDs, guardian, stranger)
	exec("INSERT INTO student_guardians (student_id, guardian_id, relationship, is_primary) VALUES (?, ?, 'Mother', true)", pupils["s1"].id, guardian)

	// Ten days of attendance (P present, L late, A absent); s6 is never marked.
	patterns := map[string]string{"s1": "PPLPPPPPPP", "s2": "PPPPPAAAAA", "s3": "PAPAPAPAPP", "s4": "PPPPPPPPPP", "s8": "AAAAAAAAAA", "s5": "PPPPPPPPPP", "s7": "PAPAPPPPPP"}
	status := map[byte]string{'P': "present", 'L': "late", 'A': "absent"}
	type tally struct{ marked, present, late, absent int64 }
	add := func(m map[string]*tally, key string, s byte) {
		if m[key] == nil {
			m[key] = &tally{}
		}
		m[key].marked++
		switch s {
		case 'P':
			m[key].present++
		case 'L':
			m[key].late++
		default:
			m[key].absent++
		}
	}
	byDay, byClass, bySection, last5 := map[string]*tally{}, map[string]*tally{}, map[string]*tally{}, map[string]*tally{}
	for key, pattern := range patterns {
		p := pupils[key]
		for i := 0; i < 10; i++ {
			date := d(i - 9)
			exec("INSERT INTO student_attendance (student_id, session_id, class_id, section_id, attendance_date, status, source, recorded_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, 'manual', ?, ?)",
				p.id, session, p.class, p.section, date, status[pattern[i]], adm.user.ID, adm.user.ID)
			add(byDay, date, pattern[i])
			add(byClass, p.class, pattern[i])
			add(bySection, p.section, pattern[i])
			if i >= 5 {
				add(last5, "all", pattern[i])
			}
		}
	}
	for i, s := range "PPPPAPLPPP" {
		ts := map[rune]string{'P': "present", 'A': "absent", 'L': "leave"}[s]
		exec("INSERT INTO teacher_attendance (session_id, employee_id, attendance_date, status, recorded_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)", session, t1, d(i-9), ts, adm.user.ID, adm.user.ID)
	}

	// Published exams with results; a draft exam with a result is ignored.
	exam := func(name, class, sec, state string) string {
		return newID(`INSERT INTO exams (name, exam_type, session_id, class_id, section_id, status, published_at, created_by, updated_by)
VALUES (?, 'half_yearly', ?, ?, ?, ?, CASE WHEN ? = 'draft' THEN NULL ELSE now() END, ?, ?) RETURNING id`, name, session, class, sec, state, state, adm.user.ID, adm.user.ID)
	}
	examSubject := func(examID, class, subject string) string {
		return newID("INSERT INTO exam_subjects (exam_id, class_id, subject_id, full_marks, pass_marks) VALUES (?, ?, ?, 100, 33) RETURNING id", examID, class, subject)
	}
	mark := func(examID, es, key string, marks float64, passed bool) {
		p := pupils[key]
		exec("INSERT INTO exam_marks (exam_id, exam_subject_id, student_id, session_id, class_id, section_id, marks, grade, grade_point, passed, entered_by, updated_by) VALUES (?, ?, ?, ?, ?, ?, ?, 'X', 0, ?, ?, ?)",
			examID, es, p.id, session, p.class, p.section, marks, passed, adm.user.ID, adm.user.ID)
	}
	result := func(examID, key string, total, gpa float64, passed bool, pos int) {
		p := pupils[key]
		exec("INSERT INTO exam_results (exam_id, student_id, session_id, class_id, section_id, total_marks, full_marks, gpa, passed, class_position, section_position) VALUES (?, ?, ?, ?, ?, ?, 200, ?, ?, ?, ?)",
			examID, p.id, session, p.class, p.section, total, gpa, passed, pos, pos)
	}
	examA1 := exam("Half yearly", classA, a1, "published")
	es11, es12 := examSubject(examA1, classA, sub1), examSubject(examA1, classA, sub2)
	for _, m := range []struct {
		key        string
		m1, m2     float64
		p1, p2     bool
		gpa        float64
		passed     bool
		resultRank int
	}{{"s1", 90, 85, true, true, 5, true, 1}, {"s2", 70, 60, true, true, 3.5, true, 3}, {"s3", 75, 72, true, true, 4, true, 2}, {"s4", 20, 50, false, true, 0, false, 4}} {
		mark(examA1, es11, m.key, m.m1, m.p1)
		mark(examA1, es12, m.key, m.m2, m.p2)
		result(examA1, m.key, m.m1+m.m2, m.gpa, m.passed, m.resultRank)
	}
	examA2 := exam("Half yearly", classA, a2, "locked")
	mark(examA2, examSubject(examA2, classA, sub1), "s5", 35, true)
	result(examA2, "s5", 35, 1.5, true, 1)
	examB := exam("Half yearly", classB, b1, "published")
	esB1, esB2 := examSubject(examB, classB, sub1), examSubject(examB, classB, sub2)
	mark(examB, esB1, "s6", 80, true)
	mark(examB, esB2, "s6", 82, true)
	mark(examB, esB1, "s7", 60, true)
	mark(examB, esB2, "s7", 55, true)
	result(examB, "s6", 162, 4.25, true, 1)
	result(examB, "s7", 115, 3, true, 2)
	draft := exam("Draft test", classB, b1, "draft")
	result(draft, "s7", 10, 1, false, 1)

	// Fees: dues 45 and 5 days overdue, one due in 10 days; three payments.
	categoryID = newID("INSERT INTO fee_categories (code, name_en, frequency, created_by, updated_by) VALUES (?, 'Tuition', 'monthly', ?, ?) RETURNING id", "M11"+tag, adm.user.ID, adm.user.ID)
	structure := map[string]string{}
	for _, c := range []string{classA, classB} {
		structure[c] = newID("INSERT INTO fee_structures (session_id, class_id, category_id, amount, created_by, updated_by) VALUES (?, ?, ?, 1000, ?, ?) RETURNING id", session, c, categoryID, adm.user.ID, adm.user.ID)
	}
	due := func(key, period, dueDate string, amount, discount, paid float64) string {
		p := pupils[key]
		return newID("INSERT INTO student_fees (student_id, structure_id, session_id, class_id, period, due_date, amount, discount_amount, paid_amount, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id",
			p.id, structure[p.class], session, p.class, period, dueDate, amount, discount, paid, adm.user.ID)
	}
	s1a := due("s1", "2001-01-01", d(-45), 1000, 0, 1000)
	due("s2", "2001-01-01", d(-45), 1000, 0, 0)
	s3a := due("s3", "2001-01-01", d(-45), 1000, 100, 400)
	due("s6", "2001-01-01", d(-45), 1000, 0, 0)
	s1b := due("s1", "2001-02-01", d(-5), 500, 0, 500)
	due("s6", "2001-02-01", d(-5), 500, 0, 0)
	due("s1", "2001-03-01", d(10), 500, 0, 0)
	pay := func(key, method, ref, paidOn, fee string, amount float64) {
		p := pupils[key]
		pid := newID("INSERT INTO fee_payments (student_id, amount, method, source, reference, paid_on, idempotency_key, received_by) VALUES (?, ?, ?, 'counter', ?, ?, gen_random_uuid(), ?) RETURNING id",
			p.id, amount, method, ref, paidOn, adm.user.ID)
		exec("INSERT INTO fee_payment_allocations (payment_id, student_fee_id, amount) VALUES (?, ?, ?)", pid, fee, amount)
		exec("INSERT INTO ledger_entries (entry_type, category, amount, entry_date, description, payment_id, created_by) VALUES ('income', 'fees', ?, ?, 'Fee receipt', ?, ?)", amount, paidOn, pid, adm.user.ID)
	}
	pay("s1", "cash", "", d(-40), s1a, 1000)
	pay("s1", "bkash", "BK"+tag, d(-3), s1b, 500)
	pay("s3", "bank", "BN"+tag, d(-30), s3a, 400)
	exec("INSERT INTO ledger_entries (entry_type, category, amount, entry_date, description, created_by) VALUES ('expense', 'utilities', 300, ?, 'Electricity', ?)", d(-10), adm.user.ID)
	exec("INSERT INTO ledger_entries (entry_type, category, amount, entry_date, description, created_by) VALUES ('income', 'donation', 200, ?, 'Old donation', ?)", d(-70), adm.user.ID)

	svc := analytics.NewService(analytics.NewRepository(db))
	auditService := audit.NewService(audit.NewRepository(db))
	router := NewRouterWithAnalytics(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, auditService,
		dashboard.NewService(dashboard.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, svc)
	raw := func(method, path, body string, a actor, want int) []byte {
		t.Helper()
		r := perform(router, method, path, body, nil, a.headers)
		if r.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		return r.Body.Bytes()
	}
	get := func(path string, a actor, want int) map[string]any {
		t.Helper()
		var out map[string]any
		if err := json.Unmarshal(raw("GET", path, "", a, want), &out); err != nil {
			t.Fatal(err)
		}
		if want != 200 {
			return out
		}
		return out["data"].(map[string]any)
	}
	q := func(path string, kv ...string) string {
		parts := []string{}
		for i := 0; i+1 < len(kv); i += 2 {
			parts = append(parts, kv[i]+"="+kv[i+1])
		}
		return path + "?" + strings.Join(parts, "&")
	}
	obj := func(v any) map[string]any { return v.(map[string]any) }
	list := func(v any) []any { return v.([]any) }
	n := func(v any) float64 {
		if v == nil {
			return math.NaN()
		}
		return v.(float64)
	}
	eq := func(label string, got any, want float64) {
		t.Helper()
		if g := n(got); math.IsNaN(g) || math.Abs(g-want) > 1e-9 {
			t.Errorf("%s = %v, want %v", label, got, want)
		}
	}
	rate := func(c *tally) float64 { return math.Round(float64(c.present+c.late)/float64(c.marked)*1000) / 10 }
	checkCounts := func(label string, got map[string]any, want *tally) {
		t.Helper()
		eq(label+" marked", got["marked"], float64(want.marked))
		eq(label+" present", got["present"], float64(want.present))
		eq(label+" late", got["late"], float64(want.late))
		eq(label+" absent", got["absent"], float64(want.absent))
		eq(label+" rate", got["rate"], rate(want))
	}

	// Attendance accuracy against the fixture.
	att := get(q("/analytics/attendance", "session_id", session, "from", d(-9), "to", d(0)), head, 200)
	checkCounts("summary", obj(att["summary"]), &tally{70, 48, 1, 21})
	eq("days", att["days"], 10)
	eq("students", att["students"], 7)
	if att["filter"].(map[string]any)["group"] != "day" || att["group_by"] != "class" {
		t.Fatalf("grouping %v", att["filter"])
	}
	trend := list(att["trend"])
	if len(trend) != 10 {
		t.Fatalf("trend has %d points", len(trend))
	}
	for _, p := range trend {
		point := obj(p)
		checkCounts("day "+point["period"].(string), point, byDay[point["period"].(string)])
	}
	groups := list(att["groups"])
	if len(groups) != 2 || obj(groups[0])["id"] != classA || obj(groups[1])["id"] != classB {
		t.Fatalf("class groups %v", groups)
	}
	checkCounts("class A", obj(groups[0]), byClass[classA])
	checkCounts("class B", obj(groups[1]), byClass[classB])
	eq("class A rate literal", obj(groups[0])["rate"], 68.3)
	teachers := obj(att["teachers"])
	eq("teacher present", teachers["present"], 8)
	eq("teacher absent", teachers["absent"], 1)
	eq("teacher leave", teachers["leave"], 1)
	eq("teacher rate", teachers["rate"], 88.9)
	byA := get(q("/analytics/attendance", "session_id", session, "class_id", classA, "from", d(-9), "to", d(0)), head, 200)
	if byA["group_by"] != "section" || len(list(byA["groups"])) != 2 {
		t.Fatalf("section grouping %v", byA["groups"])
	}
	checkCounts("A1", obj(list(byA["groups"])[0]), bySection[a1])
	checkCounts("A2", obj(list(byA["groups"])[1]), bySection[a2])
	onlyA1 := get(q("/analytics/attendance", "session_id", session, "section_id", a1, "from", d(-9), "to", d(0)), head, 200)
	checkCounts("section filter", obj(onlyA1["summary"]), bySection[a1])
	if obj(onlyA1["filter"])["class_id"] != classA {
		t.Fatal("section filter did not resolve its class")
	}
	recent := get(q("/analytics/attendance", "session_id", session, "from", d(-4), "to", d(0)), head, 200)
	checkCounts("date filter", obj(recent["summary"]), last5["all"])
	weekly := get(q("/analytics/attendance", "session_id", session, "from", d(-9), "to", d(0), "group", "week"), head, 200)
	var weekSum float64
	for _, p := range list(weekly["trend"]) {
		weekSum += n(obj(p)["marked"])
	}
	eq("weekly buckets add up", weekSum, 70)

	// Examinations: pass rate, distribution, subjects and groups.
	ex := get(q("/analytics/exams", "session_id", session), head, 200)
	sum := obj(ex["summary"])
	eq("results", sum["results"], 7)
	eq("passed", sum["passed"], 6)
	eq("pass rate", sum["pass_rate"], 85.7)
	eq("avg gpa", sum["avg_gpa"], 3.04)
	eq("gpa5", sum["gpa5"], 1)
	if len(list(ex["exams"])) != 3 {
		t.Fatalf("draft exam leaked or exam missing: %v", ex["exams"])
	}
	wantDist := map[string]float64{"A+": 1, "A": 2, "A-": 1, "B": 1, "C": 0, "D": 1, "F": 1}
	var bands []string
	db.Raw("SELECT b.grade FROM grading_bands b JOIN school_settings ss ON ss.grading_scheme_id = b.scheme_id AND ss.id = 1").Scan(&bands)
	if len(bands) == 7 {
		for _, g := range list(ex["distribution"]) {
			eq("grade "+obj(g)["grade"].(string), obj(g)["count"], wantDist[obj(g)["grade"].(string)])
		}
	} else {
		t.Logf("school uses a custom grading scheme (%d bands); distribution totals only", len(bands))
	}
	var distTotal float64
	for _, g := range list(ex["distribution"]) {
		distTotal += n(obj(g)["count"])
	}
	eq("distribution total", distTotal, 7)
	subjects := list(ex["subjects"])
	if len(subjects) != 2 {
		t.Fatalf("subjects %v", subjects)
	}
	for _, s := range subjects {
		row := obj(s)
		switch row["subject_id"] {
		case sub1:
			eq("math entries", row["entries"], 7)
			eq("math pass rate", row["pass_rate"], 85.7)
			eq("math avg", row["avg_percent"], 61.4)
		case sub2:
			eq("english pass rate", row["pass_rate"], 100)
			eq("english avg", row["avg_percent"], 67.3)
		}
	}
	oneExam := obj(get(q("/analytics/exams", "session_id", session, "exam_id", examA1), head, 200)["summary"])
	eq("exam filter pass rate", oneExam["pass_rate"], 75)
	eq("exam filter avg", oneExam["avg_gpa"], 3.13)

	// Fees against the fixture over the last 60 days.
	fees := get(q("/analytics/fees", "session_id", session, "from", d(-60), "to", d(0)), accountant, 200)
	fs := obj(fees["summary"])
	for label, want := range map[string]float64{"billed": 5000, "discounts": 100, "net_billed": 4900, "paid_against_billed": 1900, "collection_rate": 38.8,
		"collected": 1900, "outstanding": 3500, "overdue": 3000, "overdue_students": 3} {
		eq("fees "+label, fs[label], want)
	}
	var ledgerIncome, ledgerExpense float64
	db.Raw("SELECT COALESCE(sum(amount) FILTER (WHERE entry_type = 'income'), 0)::float8, COALESCE(sum(amount) FILTER (WHERE entry_type = 'expense'), 0)::float8 FROM ledger_entries WHERE entry_date BETWEEN ? AND ?", d(-60), d(0)).Row().Scan(&ledgerIncome, &ledgerExpense)
	eq("ledger income", fs["income"], ledgerIncome)
	eq("ledger expense", fs["expense"], ledgerExpense)
	if ledgerIncome < 1900 || ledgerExpense < 300 {
		t.Fatalf("fixture ledger missing: %v %v", ledgerIncome, ledgerExpense)
	}
	aging := map[string]map[string]any{}
	for _, b := range list(fees["aging"]) {
		aging[obj(b)["bucket"].(string)] = obj(b)
	}
	eq("aging 0-30", aging["0_30"]["amount"], 500)
	eq("aging 31-60", aging["31_60"]["amount"], 2500)
	eq("aging 31-60 students", aging["31_60"]["students"], 3)
	eq("aging 90+", aging["90_plus"]["amount"], 0)
	methods := map[string]float64{}
	for _, m := range list(fees["methods"]) {
		methods[obj(m)["method"].(string)] = n(obj(m)["amount"])
	}
	if methods["cash"] != 1000 || methods["bkash"] != 500 || methods["bank"] != 400 {
		t.Fatalf("methods %v", methods)
	}
	top := list(fees["top_overdue"])
	if len(top) != 3 || obj(top[0])["student_id"] != pupils["s6"].id || n(obj(top[0])["overdue"]) != 1500 || obj(top[1])["student_id"] != pupils["s2"].id {
		t.Fatalf("top overdue %v", top)
	}
	var monthsCollected float64
	for _, m := range list(fees["months"]) {
		monthsCollected += n(obj(m)["collected"])
	}
	eq("monthly collected adds up", monthsCollected, 1900)
	if from := obj(get(q("/analytics/fees", "session_id", session), adm, 200)["filter"])["from"]; from != d(-100) {
		t.Errorf("default fee range starts %v, want the session start %s", from, d(-100))
	}
	feesB := obj(get(q("/analytics/fees", "session_id", session, "class_id", classB, "from", d(-60), "to", d(0)), adm, 200)["summary"])
	eq("class B net billed", feesB["net_billed"], 1500)
	eq("class B collected", feesB["collected"], 0)
	eq("class B outstanding", feesB["outstanding"], 1500)

	// Alerts: the rules flag exactly the designed students.
	risks := get(q("/analytics/risks", "session_id", session), head, 200)
	rs := obj(risks["summary"])
	eq("students considered", rs["students"], 7) // s8 is inactive
	eq("dropout", rs["dropout"], 2)
	eq("at risk", rs["at_risk"], 3)
	flagged := map[string][]any{}
	levels := map[string]string{}
	for _, item := range list(risks["items"]) {
		r := obj(item)
		flagged[r["student_id"].(string)] = list(r["reasons"])
		levels[r["student_id"].(string)] = r["level"].(string)
	}
	expect := map[string]string{"s2": "dropout", "s6": "dropout", "s3": "at_risk", "s4": "at_risk", "s5": "at_risk"}
	for key, level := range expect {
		if levels[pupils[key].id] != level {
			t.Errorf("%s level %q want %q (%v)", key, levels[pupils[key].id], level, flagged[pupils[key].id])
		}
	}
	if _, ok := flagged[pupils["s1"].id]; ok {
		t.Error("healthy student flagged")
	}
	if fmt.Sprint(flagged[pupils["s2"].id]) != "[consecutive_absences low_attendance]" || fmt.Sprint(flagged[pupils["s6"].id]) != "[no_recent_presence]" ||
		fmt.Sprint(flagged[pupils["s3"].id]) != "[low_attendance]" || fmt.Sprint(flagged[pupils["s4"].id]) != "[failed_exam]" || fmt.Sprint(flagged[pupils["s5"].id]) != "[low_gpa]" {
		t.Errorf("reasons %v", flagged)
	}
	if lvl := obj(list(risks["items"])[0])["level"]; lvl != "dropout" {
		t.Errorf("dropout alerts must come first, got %v", lvl)
	}
	pages := map[string]bool{}
	for page := 1; page <= 3; page++ {
		p := get(q("/analytics/risks", "session_id", session, "page", fmt.Sprint(page), "page_size", "2"), head, 200)
		eq("paged total", p["total"], 5)
		for _, item := range list(p["items"]) {
			pages[obj(item)["student_id"].(string)] = true
		}
		if page == 3 && len(list(p["items"])) != 1 {
			t.Fatalf("last page %v", p["items"])
		}
	}
	if len(pages) != 5 {
		t.Fatalf("paging lost or repeated alerts: %d", len(pages))
	}
	dropouts := get(q("/analytics/risks", "session_id", session, "level", "dropout"), head, 200)
	eq("level filter", dropouts["total"], 2)
	gpaOnly := get(q("/analytics/risks", "session_id", session, "reason", "low_gpa"), head, 200)
	eq("reason filter", gpaOnly["total"], 1)

	// Teacher scope: assigned class A, subject Mathematics only.
	tAtt := get(q("/analytics/attendance", "session_id", session, "from", d(-9), "to", d(0)), teacher, 200)
	checkCounts("teacher summary", obj(tAtt["summary"]), byClass[classA])
	if tAtt["teachers"] != nil || len(list(tAtt["groups"])) != 1 {
		t.Fatalf("teacher saw other classes or staff attendance: %v", tAtt)
	}
	get(q("/analytics/attendance", "session_id", session, "class_id", classB), teacher, 403)
	get(q("/analytics/attendance", "session_id", session, "section_id", b1), teacher, 403)
	tEx := get(q("/analytics/exams", "session_id", session), teacher, 200)
	eq("teacher results", obj(tEx["summary"])["results"], 5)
	eq("teacher pass rate", obj(tEx["summary"])["pass_rate"], 80)
	if s := list(tEx["subjects"]); len(s) != 1 || obj(s[0])["subject_id"] != sub1 {
		t.Fatalf("teacher subjects %v", s)
	} else {
		eq("teacher math entries", obj(s[0])["entries"], 5)
		eq("teacher math avg", obj(s[0])["avg_percent"], 58)
	}
	tRisk := get(q("/analytics/risks", "session_id", session), teacher, 200)
	eq("teacher alerts", tRisk["total"], 4)
	for _, item := range list(tRisk["items"]) {
		if obj(item)["student_id"] == pupils["s6"].id {
			t.Fatal("teacher saw a class B alert")
		}
	}
	opts := get(q("/analytics/options", "session_id", session), teacher, 200)
	if len(list(opts["classes"])) != 1 || obj(list(opts["classes"])[0])["id"] != classA || len(list(opts["sections"])) != 2 {
		t.Fatalf("teacher options %v %v", opts["classes"], opts["sections"])
	}
	get("/analytics/students/"+pupils["s1"].id, teacher, 200)
	get("/analytics/students/"+pupils["s6"].id, teacher, 404)
	if v := get("/analytics/students/"+pupils["s1"].id, teacher, 200); v["fees"] != nil {
		t.Fatal("teacher saw fee balance")
	}
	other := get(q("/analytics/attendance", "session_id", session), otherTeacher, 200)
	eq("other teacher only class B", obj(other["summary"])["marked"], float64(byClass[classB].marked))
	for _, path := range []string{"/analytics/overview", "/analytics/sessions", q("/analytics/fees", "session_id", session), q("/analytics/reports/teacher_attendance", "session_id", session), q("/analytics/reports/fee_balances", "session_id", session), "/analytics/me", "/analytics/children"} {
		get(path, teacher, 403)
	}

	// Finance access: only finance viewers see money.
	for _, path := range []string{q("/analytics/attendance", "session_id", session), q("/analytics/exams", "session_id", session), q("/analytics/risks", "session_id", session), "/analytics/overview", "/analytics/students/" + pupils["s1"].id, q("/analytics/reports/student_attendance", "session_id", session)} {
		get(path, accountant, 403)
	}
	get(q("/analytics/options", "session_id", session), accountant, 200)
	feeReport := get(q("/analytics/reports/fee_balances", "session_id", session), accountant, 200)
	eq("fee report rows", feeReport["total"], 4)
	firstFee := list(list(feeReport["rows"])[0])
	if firstFee[0] != pupils["s6"].admission || firstFee[8] != "1500" {
		t.Fatalf("fee report order %v", firstFee)
	}
	for _, path := range []string{q("/analytics/fees", "session_id", session), q("/analytics/reports/fee_balances", "session_id", session)} {
		get(path, head, 403)
	}
	headOverview := get(q("/analytics/overview", "session_id", session), head, 200)
	if headOverview["fees"] != nil {
		t.Fatal("head teacher overview exposed fees")
	}
	eq("overview active students", headOverview["active_students"], 7)
	eq("overview students by status inactive", obj(headOverview["students_by_status"])["inactive"], 1)
	eq("overview today marked", obj(headOverview["today_students"])["marked"], 7)
	eq("overview dropout", obj(headOverview["risks"])["dropout"], 2)
	adminOverview := get(q("/analytics/overview", "session_id", session), adm, 200)
	eq("admin overview fees outstanding", obj(adminOverview["fees"])["outstanding"], 3500)
	for _, s := range list(get("/analytics/sessions", head, 200)["items"]) {
		if obj(s)["net_billed"] != nil {
			t.Fatal("head teacher session comparison exposed fees")
		}
	}
	foundSession := false
	for _, s := range list(get("/analytics/sessions", adm, 200)["items"]) {
		if obj(s)["id"] == session {
			foundSession = true
			eq("session attendance rate", obj(s)["attendance_rate"], 70)
			eq("session pass rate", obj(s)["pass_rate"], 85.7)
			eq("session net billed", obj(s)["net_billed"], 5400)
			eq("session active students", obj(s)["active_students"], 7)
		}
	}
	if !foundSession {
		t.Log("fixture session is older than the eight most recent sessions in this database; comparison values not checked")
	}

	// Student and guardian isolation.
	me := get("/analytics/me", studentUser, 200)
	if obj(me["student"])["id"] != pupils["s1"].id {
		t.Fatalf("student summary for %v", obj(me["student"])["id"])
	}
	checkCounts("student session attendance", obj(obj(me["attendance"])["session"]), &tally{10, 9, 1, 0})
	if r := list(me["results"]); len(r) != 1 || obj(r[0])["grade"] != "A+" || n(obj(r[0])["percent"]) != 87.5 {
		t.Fatalf("student results %v", r)
	}
	if len(list(me["latest_subjects"])) != 2 || me["alert"] != nil {
		t.Fatalf("student subjects or alert %v %v", me["latest_subjects"], me["alert"])
	}
	eq("student outstanding", obj(me["fees"])["outstanding"], 500)
	for _, path := range []string{"/analytics/students/" + pupils["s2"].id, "/analytics/children", q("/analytics/attendance", "session_id", session), "/analytics/children/" + pupils["s1"].id} {
		get(path, studentUser, 403)
	}
	kids := list(get("/analytics/children", guardianUser, 200)["items"])
	if len(kids) != 1 || obj(obj(kids[0])["student"])["id"] != pupils["s1"].id {
		t.Fatalf("guardian children %v", kids)
	}
	get("/analytics/children/"+pupils["s1"].id, guardianUser, 200)
	get("/analytics/children/"+pupils["s2"].id, guardianUser, 404)
	get("/analytics/children/not-a-uuid", guardianUser, 400)
	get("/analytics/me", guardianUser, 403)
	if len(list(get("/analytics/children", strangerGuardian, 200)["items"])) != 0 {
		t.Fatal("unlinked guardian saw a child")
	}
	get("/analytics/children/"+pupils["s1"].id, strangerGuardian, 404)
	alert := obj(get("/analytics/students/"+pupils["s2"].id, head, 200)["alert"])
	if alert["level"] != "dropout" {
		t.Fatalf("student alert %v", alert)
	}

	// Validation and empty data.
	for _, path := range []string{
		q("/analytics/attendance", "session_id", "nope"),
		q("/analytics/attendance", "session_id", session, "from", d(-500), "to", d(0)),
		q("/analytics/attendance", "session_id", session, "to", d(1)),
		q("/analytics/attendance", "session_id", session, "from", d(0), "to", d(-3)),
		q("/analytics/attendance", "session_id", session, "from", d(-120), "group", "day"),
		q("/analytics/attendance", "session_id", session, "class_id", classB, "section_id", a1),
		q("/analytics/exams", "session_id", session, "exam_type", "quiz"),
		q("/analytics/risks", "session_id", session, "page_size", "101"),
		q("/analytics/risks", "session_id", session, "level", "maybe"),
		q("/analytics/reports/student_attendance", "session_id", session, "page", "0"),
	} {
		get(path, head, 400)
	}
	get(q("/analytics/attendance", "session_id", "00000000-0000-4000-8000-000000000000"), head, 404)
	get(q("/analytics/reports/nothing", "session_id", session), head, 404)
	emptyAtt := get(q("/analytics/attendance", "session_id", emptySession), head, 200)
	if obj(emptyAtt["summary"])["rate"] != nil || len(list(emptyAtt["trend"])) != 0 || n(obj(emptyAtt["summary"])["marked"]) != 0 {
		t.Fatalf("empty attendance %v", emptyAtt["summary"])
	}
	emptyEx := get(q("/analytics/exams", "session_id", emptySession), head, 200)
	if obj(emptyEx["summary"])["pass_rate"] != nil || len(list(emptyEx["exams"])) != 0 {
		t.Fatalf("empty exams %v", emptyEx["summary"])
	}
	emptyFees := obj(get(q("/analytics/fees", "session_id", emptySession), adm, 200)["summary"])
	if emptyFees["collection_rate"] != nil || n(emptyFees["outstanding"]) != 0 {
		t.Fatalf("empty fees %v", emptyFees)
	}
	if v := get(q("/analytics/risks", "session_id", emptySession), head, 200); n(v["total"]) != 0 {
		t.Fatal("empty session has alerts")
	}

	// Reports: paging, scope and CSV.
	rep := get(q("/analytics/reports/student_attendance", "session_id", session, "from", d(-9), "to", d(0), "page_size", "3"), head, 200)
	eq("student report total", rep["total"], 8)
	if len(list(rep["rows"])) != 3 || len(list(rep["columns"])) != 12 {
		t.Fatalf("report page %v", rep)
	}
	first := list(list(rep["rows"])[0])
	if first[0] != pupils["s1"].admission || first[7] != "10" || first[11] != "100" {
		t.Fatalf("first report row %v", first)
	}
	tRep := get(q("/analytics/reports/exam_results", "session_id", session), teacher, 200)
	eq("teacher exam report rows", tRep["total"], 5)
	trep := get(q("/analytics/reports/teacher_attendance", "session_id", session, "from", d(-9), "to", d(0), "page_size", "100"), head, 200)
	foundTeacher := false
	for _, row := range list(trep["rows"]) {
		if list(row)[0] == "T1"+tag {
			foundTeacher = true
			if list(row)[5] != "8" || list(row)[8] != "88.9" {
				t.Fatalf("teacher report row %v", row)
			}
		}
	}
	if !foundTeacher {
		t.Fatal("teacher attendance report missing T1")
	}
	csvResp := perform(router, "GET", q("/analytics/reports/student_attendance/csv", "session_id", session, "from", d(-9), "to", d(0)), "", nil, head.headers)
	if csvResp.Code != 200 || !strings.HasPrefix(csvResp.Body.String(), "\xEF\xBB\xBF") || csvResp.Header().Get("X-Total-Rows") != "8" {
		t.Fatalf("csv %d %q", csvResp.Code, csvResp.Header())
	}
	body := csvResp.Body.String()
	if !strings.Contains(body, "'=HYPERLINK") || !strings.Contains(body, "শিক্ষার্থী s1") || strings.Count(body, "\n") != 9 {
		t.Fatalf("csv content %q", body)
	}
	var exported int64
	db.Raw("SELECT count(*) FROM audit_events WHERE action = 'analytics.exported' AND actor_user_id = ? AND target_id = 'student_attendance'", head.user.ID).Row().Scan(&exported)
	if exported != 1 {
		t.Fatalf("export audit events %d", exported)
	}
	riskCSV := perform(router, "GET", q("/analytics/reports/risks/csv", "session_id", session), "", nil, teacher.headers)
	if riskCSV.Code != 200 || strings.Count(riskCSV.Body.String(), "\n") != 5 {
		t.Fatalf("teacher risk csv %d %q", riskCSV.Code, riskCSV.Body.String())
	}
	get(q("/analytics/reports/fee_balances/csv", "session_id", session), teacher, 403)

	// Thresholds: admin only, validated and audited; changes take effect.
	settingsBody := `{"attendance_threshold":50,"attendance_window_days":30,"min_marked_days":5,"gpa_threshold":2,"consecutive_absences":6,"inactivity_days":21}`
	raw("PUT", "/analytics/settings", settingsBody, head, 403)
	raw("PUT", "/analytics/settings", `{"attendance_threshold":50,"attendance_window_days":5,"min_marked_days":5,"gpa_threshold":2,"consecutive_absences":6,"inactivity_days":21}`, adm, 400)
	raw("PUT", "/analytics/settings", `{"attendance_threshold":50,"unknown":1}`, adm, 400)
	raw("PUT", "/analytics/settings", settingsBody, adm, 200)
	after := obj(get(q("/analytics/risks", "session_id", session), head, 200)["summary"])
	eq("dropout after thresholds", after["dropout"], 1) // s2's 5-day run is now below 6
	eq("at risk after thresholds", after["at_risk"], 2) // s2 (50%) and s3 (60%) are no longer below 50%
	var settingsAudit int64
	db.Raw("SELECT count(*) FROM audit_events WHERE action = 'analytics.settings_updated' AND actor_user_id = ?", adm.user.ID).Row().Scan(&settingsAudit)
	if settingsAudit != 1 {
		t.Fatalf("settings audit %d", settingsAudit)
	}

	// Navigation offers each role its analytics page.
	nav := func(a actor) string {
		data := get("/dashboard", a, 200)
		keys := []string{}
		for _, item := range list(data["navigation"]) {
			keys = append(keys, obj(item)["key"].(string))
		}
		return strings.Join(keys, ",")
	}
	for a, want := range map[*actor][]string{&head: {"analytics", "reports"}, &teacher: {"analytics_teacher", "reports"}, &accountant: {"analytics", "reports"}, &studentUser: {"analytics_student"}, &guardianUser: {"analytics_guardian"}} {
		got := nav(*a)
		for _, key := range want {
			if !strings.Contains(","+got+",", ","+key+",") {
				t.Errorf("%s navigation %s lacks %s", a.user.Role, got, key)
			}
		}
	}

	// Module gates: a source module off closes its analytics; analytics off closes all.
	exec("UPDATE school_modules SET enabled = false WHERE school_id = 1 AND module_key = 'attendance'")
	get(q("/analytics/attendance", "session_id", session), head, 403)
	get(q("/analytics/reports/student_attendance", "session_id", session), head, 403)
	offOverview := get(q("/analytics/overview", "session_id", session), head, 200)
	if offOverview["today_students"] != nil || offOverview["last_30_days"] != nil {
		t.Fatal("overview showed attendance while the module is off")
	}
	if v := obj(get(q("/analytics/risks", "session_id", session), head, 200)["summary"]); n(v["dropout"]) != 0 {
		t.Fatal("attendance alerts while the module is off")
	}
	exec("UPDATE school_modules SET enabled = false WHERE school_id = 1 AND module_key = 'analytics'")
	for _, path := range []string{"/analytics/overview", "/analytics/me", q("/analytics/options", "session_id", session)} {
		code := perform(router, "GET", path, "", nil, map[string]string{"Authorization": head.headers["Authorization"]}).Code
		if path == "/analytics/me" {
			code = perform(router, "GET", path, "", nil, studentUser.headers).Code
		}
		if code != 403 {
			t.Errorf("%s with analytics off: %d", path, code)
		}
	}
	if strings.Contains(nav(head), "analytics") {
		t.Error("navigation still offers analytics while the module is off")
	}
	raw("GET", "/analytics/overview", "", actor{headers: map[string]string{}}, 401)
}
