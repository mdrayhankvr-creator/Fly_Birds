package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/diary"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notices"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/notify"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/rbac"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/school"
)

const m9PDF = "%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n"

func m9PNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	return b.Bytes()
}

func TestPostgresM9NoticesNotificationsAndDiary(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
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
	if db.Table("schema_migrations").Where("version IN (901, 902, 903)").Count(&applied); applied != 3 {
		t.Fatalf("M9 migrations applied %d", applied)
	}
	authService, err := auth.NewService(auth.NewRepository(db), bytes.Repeat([]byte{0x79}, 32))
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "M9TestPassword!" + suffix
	userIDs := []string{}
	var sessionID, classID, sectionA, sectionB, subjectID, otherSubject, studentA, studentB, guardianARow, guardianBRow string
	employeeIDs := []string{}
	modules := map[string]bool{}
	for _, key := range []string{"notices", "diary", "attendance", "fees", "examinations"} {
		var on bool
		db.Table("school_modules").Select("enabled").Where("school_id=1 AND module_key=?", key).Scan(&on)
		modules[key] = on
	}
	var oldSettings struct {
		DailyRequired bool
		ReminderTime  string
	}
	db.Raw("SELECT daily_required, to_char(reminder_time,'HH24:MI') AS reminder_time FROM diary_settings WHERE id=1").Scan(&oldSettings)
	t.Cleanup(func() {
		for key, on := range modules {
			_ = db.Exec("UPDATE school_modules SET enabled=? WHERE school_id=1 AND module_key=?", on, key).Error
		}
		_ = db.Exec("UPDATE diary_settings SET daily_required=?, reminder_time=?::time WHERE id=1", oldSettings.DailyRequired, oldSettings.ReminderTime).Error
		if len(userIDs) > 0 {
			for _, q := range []string{
				"DELETE FROM homework_submissions WHERE diary_id IN (SELECT id FROM class_diaries WHERE teacher_user_id IN ?)",
				"DELETE FROM class_diaries WHERE teacher_user_id IN ?",
				"DELETE FROM notices WHERE created_by IN ?",
				"DELETE FROM school_events WHERE created_by IN ?",
				"DELETE FROM file_objects WHERE uploaded_by IN ?",
			} {
				if err := db.Exec(q, userIDs).Error; err != nil {
					t.Errorf("cleanup %s: %v", q, err)
				}
			}
		}
		if sessionID != "" {
			_ = db.Exec("DELETE FROM student_attendance WHERE session_id=?", sessionID).Error
			_ = db.Exec("DELETE FROM student_guardians WHERE student_id IN (SELECT id FROM students WHERE session_id=?)", sessionID).Error
			_ = db.Exec("DELETE FROM students WHERE session_id=?", sessionID).Error
			_ = db.Exec("DELETE FROM teacher_assignments WHERE session_id=?", sessionID).Error
		}
		_ = db.Exec("DELETE FROM guardians WHERE id IN ?", []string{guardianARow + "", guardianBRow + ""}).Error
		if len(employeeIDs) > 0 {
			_ = db.Exec("DELETE FROM employees WHERE id IN ?", employeeIDs).Error
		}
		if classID != "" {
			_ = db.Exec("DELETE FROM academic_sections WHERE class_id=?", classID).Error
			_ = db.Exec("DELETE FROM academic_class_subjects WHERE class_id=?", classID).Error
			_ = db.Exec("DELETE FROM academic_classes WHERE id=?", classID).Error
		}
		_ = db.Exec("DELETE FROM academic_subjects WHERE id IN ?", []string{subjectID + "", otherSubject + ""}).Error
		if sessionID != "" {
			_ = db.Exec("DELETE FROM academic_sessions WHERE id=?", sessionID).Error
		}
		if len(userIDs) > 0 {
			_ = db.Exec("DELETE FROM auth_sessions WHERE user_id IN ?", userIDs).Error
			_ = db.Exec("DELETE FROM users WHERE id IN ?", userIDs).Error
		}
	})
	type actor struct {
		user    auth.User
		headers map[string]string
	}
	login := func(role string) actor {
		u, err := authService.BootstrapUser(ctx, fmt.Sprintf("m9-%s-%d-%s@example.test", role, len(userIDs), suffix), password, role)
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
	admin, head, teacher, otherTeacher := login("admin"), login("head_teacher"), login("teacher"), login("teacher")
	stuA, stuB, guardA, guardB, accountant := login("student"), login("student"), login("guardian"), login("guardian"), login("accountant")
	insert := func(q string, args ...any) string {
		t.Helper()
		var id string
		if err := db.Raw(q, args...).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	sessionID = insert("INSERT INTO academic_sessions(name,start_date,end_date) VALUES (?,'2026-01-01','2199-12-31') RETURNING id", fmt.Sprintf("%04d", 7000+time.Now().Nanosecond()%2000))
	classID = insert("INSERT INTO academic_classes(name) VALUES (?) RETURNING id", "M9 Class "+suffix)
	sectionA = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", sessionID, classID)
	sectionB = insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", sessionID, classID)
	subjectID = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'গণিত',?) RETURNING id", "Maths "+suffix, "M9M"+suffix[len(suffix)-8:])
	otherSubject = insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,'ইংরেজি',?) RETURNING id", "English "+suffix, "M9E"+suffix[len(suffix)-8:])
	for _, s := range []string{subjectID, otherSubject} {
		if err := db.Exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", classID, s).Error; err != nil {
			t.Fatal(err)
		}
	}
	employee := func(u auth.User, code, subject string) {
		id := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher',?,'Teacher','2020-01-01') RETURNING id", code+suffix[len(suffix)-8:], u.ID, "Teacher "+code)
		employeeIDs = append(employeeIDs, id)
		if err := db.Exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", id, sessionID, classID, subject).Error; err != nil {
			t.Fatal(err)
		}
	}
	employee(teacher.user, "T1", subjectID)
	employee(otherTeacher.user, "T2", otherSubject)
	studentA = insert("INSERT INTO students(admission_no,name_en,name_bn,session_id,class_id,section_id,roll_no,user_id) VALUES (?,?,?,?,?,?,1,?) RETURNING id", "M9A"+suffix, "Student A", "শিক্ষার্থী ক", sessionID, classID, sectionA, stuA.user.ID)
	studentB = insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,roll_no,user_id) VALUES (?,?,?,?,?,1,?) RETURNING id", "M9B"+suffix, "Student B", sessionID, classID, sectionB, stuB.user.ID)
	guardianARow = insert("INSERT INTO guardians(name_en,phone,user_id) VALUES ('Guardian A','01711-000000',?) RETURNING id", guardA.user.ID)
	guardianBRow = insert("INSERT INTO guardians(name_en,user_id) VALUES ('Guardian B',?) RETURNING id", guardB.user.ID)
	for pair, g := range map[string]string{studentA: guardianARow, studentB: guardianBRow} {
		if err := db.Exec("INSERT INTO student_guardians(student_id,guardian_id,relationship,is_primary) VALUES (?,?,'mother',true)", pair, g).Error; err != nil {
			t.Fatal(err)
		}
	}

	store, err := files.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	notifier := notify.NewService(notify.NewRepository(db), notify.MockSMS{})
	comm := &Communication{Notices: notices.NewService(notices.NewRepository(db)), Notify: notifier, Diary: diary.NewService(diary.NewRepository(db)), Files: files.NewService(db, store)}
	router := NewRouterWithCommunication(authService, false, rbac.NewService(rbac.NewRepository(db)), school.NewService(school.NewRepository(db)), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, comm)
	call := func(method, path, body string, a actor, want int) map[string]any {
		t.Helper()
		r := perform(router, method, path, body, nil, a.headers)
		if r.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		var out map[string]any
		_ = json.Unmarshal(r.Body.Bytes(), &out)
		return out
	}
	data := func(m map[string]any) map[string]any { return m["data"].(map[string]any) }
	items := func(m map[string]any) []any { return data(m)["items"].([]any) }
	upload := func(path, name string, body []byte, a actor) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, _ := w.CreateFormFile("file", name)
		_, _ = part.Write(body)
		_ = w.Close()
		req := httptest.NewRequest(http.MethodPost, path, &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", a.headers["Authorization"])
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	fileID := func(rec *httptest.ResponseRecorder) string {
		t.Helper()
		if rec.Code != 201 {
			t.Fatalf("upload got %d: %s", rec.Code, rec.Body.String())
		}
		var out struct{ Data files.Meta }
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Data.ID
	}
	module := func(key string, on bool) {
		if err := db.Exec("UPDATE school_modules SET enabled=? WHERE school_id=1 AND module_key=?", on, key).Error; err != nil {
			t.Fatal(err)
		}
	}
	dispatch := func() notify.Result {
		t.Helper()
		var out struct{ Data notify.Result }
		r := perform(router, "POST", "/notifications/dispatch", "", nil, admin.headers)
		if r.Code != 200 {
			_, cause := notifier.Dispatch(ctx)
			t.Fatalf("dispatch %d %s: %v", r.Code, r.Body.String(), cause)
		}
		_ = json.Unmarshal(r.Body.Bytes(), &out)
		return out.Data
	}
	inboxKeys := func(a actor) []string {
		var keys []string
		db.Table("notifications").Where("user_id=? AND withdrawn_at IS NULL", a.user.ID).Order("source_key").Pluck("source_key", &keys)
		return keys
	}

	// ---- Notices ----
	module("notices", false)
	call("GET", "/notices", "", stuA, 403)
	module("notices", true)
	module("attendance", true)
	module("fees", true)
	module("examinations", true)
	call("GET", "/notices", "", actor{}, 401)
	call("POST", "/notices", `{"title_en":"X","body_en":"x","audience_roles":["student"]}`, teacher, 403)
	if r := upload("/notices/files", "plan.pdf", []byte(m9PDF), teacher); r.Code != 403 {
		t.Fatalf("teacher notice upload %d", r.Code)
	}
	if r := upload("/notices/files", "evil.pdf", []byte("<html><script>alert(1)</script>"), head); r.Code != 415 {
		t.Fatalf("html upload %d %s", r.Code, r.Body.String())
	}
	if r := upload("/notices/files", "../../etc/passwd.pdf", []byte("%PDF-1.4 /JavaScript (x)\n%%EOF"), head); r.Code != 415 {
		t.Fatalf("active pdf upload %d", r.Code)
	}
	noticeFile := fileID(upload("/notices/files", `..\..\routine "final".pdf`, []byte(m9PDF), head))
	if fileID(upload("/notices/files", "other.pdf", []byte(m9PDF), admin)) == "" {
		t.Fatal("admin upload")
	}
	body := fmt.Sprintf(`{"title_en":"Exam week","title_bn":"পরীক্ষার সপ্তাহ","body_en":"Bring admit cards.","priority":"urgent","audience_roles":["student","guardian"],"targets":[{"class_id":%q,"section_id":%q}],"file_ids":[%q]}`, classID, sectionA, noticeFile)
	call("POST", "/notices", strings.Replace(body, noticeFile, fileID(upload("/notices/files", "x.pdf", []byte(m9PDF), admin)), 1), head, 400)
	call("POST", "/notices", fmt.Sprintf(`{"title_en":"Bad","body_en":"x","audience_roles":["student"],"targets":[{"class_id":%q,"section_id":%q}]}`, classID, "00000000-0000-4000-8000-000000000000"), head, 400)
	call("POST", "/notices", `{"title_en":"Bad","body_en":"x","audience_roles":["admin"]}`, head, 400)
	notice := data(call("POST", "/notices", body, head, 201))
	noticeID := notice["id"].(string)
	if notice["status"] != "draft" || len(notice["attachments"].([]any)) != 1 || notice["attachments"].([]any)[0].(map[string]any)["name"] != "routine final.pdf" {
		t.Fatalf("notice %+v", notice)
	}
	if n := len(items(call("GET", "/notices", "", stuA, 200))); n != 0 {
		t.Fatalf("draft visible to student: %d", n)
	}
	call("POST", "/notices/"+noticeID+"/publish", `{}`, head, 200)
	if n := len(items(call("GET", "/notices", "", stuA, 200))); n != 1 {
		t.Fatalf("student A feed %d", n)
	}
	for name, a := range map[string]actor{"student B": stuB, "guardian B": guardB, "teacher": teacher, "accountant": accountant} {
		if n := len(items(call("GET", "/notices", "", a, 200))); n != 0 {
			t.Fatalf("%s sees targeted notice", name)
		}
	}
	if n := len(items(call("GET", "/notices", "", guardA, 200))); n != 1 {
		t.Fatalf("guardian A feed %d", n)
	}
	rec := perform(router, "GET", "/notices/"+noticeID+"/files/"+noticeFile+"?download=1", "", nil, stuA.headers)
	if rec.Code != 200 || rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "sandbox") ||
		!strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") || rec.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(rec.Body.String(), "%PDF") {
		t.Fatalf("notice file %d %v", rec.Code, rec.Header())
	}
	call("GET", "/notices/"+noticeID, "", stuB, 404)
	call("GET", "/notices/"+noticeID+"/files/"+noticeFile, "", stuB, 404)
	scheduled := data(call("POST", "/notices", `{"title_en":"Later","body_en":"Soon","audience_roles":["student"]}`, admin, 201))
	call("POST", "/notices/"+scheduled["id"].(string)+"/publish", fmt.Sprintf(`{"publish_at":%q}`, time.Now().Add(48*time.Hour).UTC().Format(time.RFC3339)), admin, 200)
	if n := len(items(call("GET", "/notices", "", stuB, 200))); n != 0 {
		t.Fatal("scheduled notice is live")
	}
	if n := len(items(call("GET", "/notices/manage?status=scheduled&search=later", "", head, 200))); n != 1 {
		t.Fatalf("scheduled filter %d", n)
	}
	expired := data(call("POST", "/notices", fmt.Sprintf(`{"title_en":"Old","body_en":"Gone","audience_roles":["student"],"expires_at":%q}`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)), admin, 201))
	call("POST", "/notices/"+expired["id"].(string)+"/publish", fmt.Sprintf(`{"publish_at":%q}`, time.Now().Add(-2*time.Hour).UTC().Format(time.RFC3339)), admin, 200)
	call("POST", "/notices/"+expired["id"].(string)+"/publish", `{}`, admin, 400)
	if n := len(items(call("GET", "/notices", "", stuB, 200))); n != 0 {
		t.Fatal("expired notice is live")
	}
	call("DELETE", "/notices/"+noticeID, "", head, 409)

	// ---- Events ----
	call("POST", "/events", `{}`, stuA, 403)
	evBody := fmt.Sprintf(`{"title_en":"Half-yearly exam","kind":"exam","starts_on":"2026-11-02","ends_on":"2026-11-02","start_time":"10:00","end_time":"09:00","audience_roles":["student"],"targets":[{"class_id":%q}]}`, classID)
	call("POST", "/events", evBody, admin, 400)
	call("POST", "/events", strings.Replace(evBody, `"ends_on":"2026-11-02"`, `"ends_on":"2026-11-01"`, 1), admin, 400)
	event := data(call("POST", "/events", strings.Replace(evBody, `"end_time":"09:00"`, `"end_time":"13:00"`, 1), admin, 201))
	eventID := event["id"].(string)
	listEvents := func(a actor, extra string) []any {
		return call("GET", "/events?from=2026-11-01&to=2026-11-30"+extra, "", a, 200)["data"].([]any)
	}
	if len(listEvents(stuA, "")) != 0 || len(listEvents(admin, "&manage=1")) == 0 {
		t.Fatal("draft event visibility")
	}
	call("POST", "/events/"+eventID+"/publish", "", admin, 200)
	if len(listEvents(stuA, "")) != 1 || len(listEvents(stuB, "")) != 1 || len(listEvents(teacher, "")) != 0 {
		t.Fatal("published event visibility")
	}
	call("POST", "/events/"+eventID+"/cancel", "", head, 200)
	if ev := listEvents(stuA, ""); len(ev) != 1 || ev[0].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("cancelled event %+v", ev)
	}
	call("GET", "/events?from=2026-01-01&to=2028-01-01", "", stuA, 400)
	call("DELETE", "/events/"+eventID, "", admin, 409)

	// ---- Notifications and mock SMS ----
	first := dispatch()
	if first.Created["notice"] < 2 {
		t.Fatalf("notice notifications %+v", first)
	}
	if keys := inboxKeys(stuB); len(keys) != 0 {
		t.Fatalf("student B notified %v", keys)
	}
	var smsStatus, smsRef string
	db.Raw("SELECT status, provider_ref FROM sms_messages WHERE user_id=? AND source_key=?", guardA.user.ID, "notice:"+noticeID).Row().Scan(&smsStatus, &smsRef)
	if smsStatus != "sent_mock" || !strings.HasPrefix(smsRef, "MOCK-") {
		t.Fatalf("urgent notice SMS %q %q", smsStatus, smsRef)
	}
	attendance := insert("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) VALUES (?,?,?,?,(now() AT TIME ZONE 'Asia/Dhaka')::date,'absent','manual',?,?) RETURNING id", studentA, sessionID, classID, sectionA, teacher.user.ID, teacher.user.ID)
	absence := insert("INSERT INTO attendance_absence_events(attendance_id,student_id,session_id,attendance_date) SELECT id,student_id,session_id,attendance_date FROM student_attendance WHERE id=? RETURNING id", attendance)
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = notifier.Dispatch(ctx) }()
	}
	wg.Wait()
	dispatch()
	var count int64
	db.Table("notifications").Where("source_key=?", "absence:"+absence).Count(&count)
	if count != 1 {
		t.Fatalf("absence notifications after repeated dispatch %d", count)
	}
	db.Table("sms_messages").Where("source_key=? AND status='sent_mock'", "absence:"+absence).Count(&count)
	if count != 1 {
		t.Fatalf("absence SMS %d", count)
	}
	inbox := data(call("GET", "/notifications", "", guardA, 200))
	if inbox["unread"].(float64) < 2 {
		t.Fatalf("guardian inbox %+v", inbox)
	}
	firstID := inbox["items"].([]any)[0].(map[string]any)["id"].(string)
	call("POST", "/notifications/"+firstID+"/read", "", stuA, 404)
	call("POST", "/notifications/"+firstID+"/read", "", guardA, 200)
	if after := data(call("GET", "/notifications?unread=1", "", guardA, 200)); after["unread"].(float64) != inbox["unread"].(float64)-1 {
		t.Fatalf("unread after read %+v", after["unread"])
	}
	db.Exec("UPDATE attendance_absence_events SET state='cancelled' WHERE id=?", absence)
	if res := dispatch(); res.Withdrawn != 1 {
		t.Fatalf("withdrawn %+v", res)
	}
	if strings.Contains(strings.Join(inboxKeys(guardA), ","), "absence:") {
		t.Fatal("corrected absence still in inbox")
	}
	call("PUT", "/notifications/preferences", `{"items":[{"category":"absence","in_app":false,"sms":false},{"category":"absence","in_app":true,"sms":true}]}`, guardA, 400)
	prefs := call("PUT", "/notifications/preferences", `{"items":[{"category":"absence","in_app":false,"sms":false}]}`, guardA, 200)["data"].([]any)
	if len(prefs) != 5 || prefs[1].(map[string]any)["in_app"] != false {
		t.Fatalf("preferences %+v", prefs)
	}
	second := insert("INSERT INTO student_attendance(student_id,session_id,class_id,section_id,attendance_date,status,source,recorded_by,updated_by) VALUES (?,?,?,?,(now() AT TIME ZONE 'Asia/Dhaka')::date - 1,'absent','manual',?,?) RETURNING id", studentA, sessionID, classID, sectionA, teacher.user.ID, teacher.user.ID)
	secondEvent := insert("INSERT INTO attendance_absence_events(attendance_id,student_id,session_id,attendance_date) SELECT id,student_id,session_id,attendance_date FROM student_attendance WHERE id=? RETURNING id", second)
	dispatch()
	db.Table("notifications").Where("source_key=?", "absence:"+secondEvent).Count(&count)
	var smsCount int64
	db.Table("sms_messages").Where("source_key=?", "absence:"+secondEvent).Count(&smsCount)
	if count != 0 || smsCount != 0 {
		t.Fatalf("preferences ignored: %d notifications, %d SMS", count, smsCount)
	}
	call("GET", "/notifications/sms", "", head, 403)
	log := data(call("GET", "/notifications/sms?status=sent_mock", "", admin, 200))
	logBody, _ := json.Marshal(log)
	if strings.Contains(string(logBody), "01711000000") || !strings.Contains(string(logBody), "000") || log["counts"].(map[string]any)["sent_mock"].(float64) < 2 {
		t.Fatalf("sms log %s", logBody)
	}

	// ---- Class diary ----
	module("diary", false)
	call("GET", "/diary/assignments", "", teacher, 403)
	module("diary", true)
	today := time.Now().In(diary.Dhaka).Format(time.DateOnly)
	assignments := call("GET", "/diary/assignments", "", teacher, 200)["data"].([]any)
	if len(assignments) != 2 {
		t.Fatalf("assignments %+v", assignments)
	}
	diaryBody := func(section, subject, date string, enabled bool, ids ...string) string {
		quoted := []string{}
		for _, id := range ids {
			quoted = append(quoted, fmt.Sprintf("%q", id))
		}
		return fmt.Sprintf(`{"section_id":%q,"subject_id":%q,"diary_date":%q,"lesson_topics":"Fractions","homework":"Exercise 3.2","submission_enabled":%t,"file_ids":[%s]}`, section, subject, date, enabled, strings.Join(quoted, ","))
	}
	call("POST", "/diary", diaryBody(sectionA, subjectID, today, true), otherTeacher, 403)
	call("POST", "/diary", diaryBody(sectionA, subjectID, today, true), stuA, 403)
	call("POST", "/diary", diaryBody(sectionA, subjectID, time.Now().AddDate(0, 0, 9).Format(time.DateOnly), false), teacher, 400)
	if r := upload("/diary/files", "notes.txt", []byte("plain text"), teacher); r.Code != 415 {
		t.Fatalf("text upload %d", r.Code)
	}
	pdf, img := fileID(upload("/diary/files", "notes.pdf", []byte(m9PDF), teacher)), fileID(upload("/diary/files", "board.png", m9PNG(), teacher))
	d := data(call("POST", "/diary", diaryBody(sectionA, subjectID, today, true, pdf, img), teacher, 201))
	diaryID := d["id"].(string)
	call("POST", "/diary", diaryBody(sectionA, subjectID, today, false), teacher, 409)
	if len(items(call("GET", "/diary/me", "", stuA, 200))) != 0 {
		t.Fatal("draft diary visible to student")
	}
	call("PATCH", "/diary/"+diaryID, diaryBody(sectionA, subjectID, today, true, pdf, img), otherTeacher, 404)
	call("POST", "/diary/"+diaryID+"/publish", "", teacher, 200)
	call("POST", "/diary/"+diaryID+"/publish", "", teacher, 409)
	if len(items(call("GET", "/diary/me", "", stuA, 200))) != 1 || len(items(call("GET", "/diary/me", "", stuB, 200))) != 0 {
		t.Fatal("published diary visibility by section")
	}
	if len(items(call("GET", "/diary/children/"+studentA+"?subject_id="+subjectID+"&from="+today+"&to="+today, "", guardA, 200))) != 1 {
		t.Fatal("guardian child diary")
	}
	call("GET", "/diary/children/"+studentB, "", guardA, 404)
	call("GET", "/diary/me", "", guardA, 403)
	if rec := perform(router, "GET", "/diary/"+diaryID+"/files/"+img, "", nil, stuA.headers); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("student diary image %d", rec.Code)
	}
	call("GET", "/diary/"+diaryID+"/files/"+img, "", stuB, 404)
	call("GET", "/diary/"+diaryID, "", guardB, 404)
	call("GET", "/diary/"+diaryID+"?student_id="+studentA, "", guardA, 200)

	// Optional homework.
	if r := upload("/diary/homework/files", "fake.png", append([]byte("\x89PNG\r\n\x1a\n"), []byte("junk")...), stuA); r.Code != 415 {
		t.Fatalf("fake png %d", r.Code)
	}
	if r := upload("/diary/homework/files", "answer.png", m9PNG(), teacher); r.Code != 403 {
		t.Fatalf("teacher homework upload %d", r.Code)
	}
	answer := fileID(upload("/diary/homework/files", "answer.png", m9PNG(), stuA))
	call("PUT", "/diary/"+diaryID+"/submission", `{"answer_text":""}`, stuA, 400)
	call("PUT", "/diary/"+diaryID+"/submission", fmt.Sprintf(`{"answer_text":"Done","file_ids":[%q]}`, answer), stuB, 404)
	sub := data(call("PUT", "/diary/"+diaryID+"/submission", fmt.Sprintf(`{"answer_text":"Done","file_ids":[%q]}`, answer), stuA, 200))
	call("PUT", "/diary/"+diaryID+"/submission", fmt.Sprintf(`{"answer_text":"Done again","file_ids":[%q]}`, answer), stuA, 200)
	roster := call("GET", "/diary/"+diaryID+"/submissions", "", teacher, 200)["data"].([]any)
	if len(roster) != 1 || roster[0].(map[string]any)["status"] != "submitted" {
		t.Fatalf("submission roster %+v", roster)
	}
	call("GET", "/diary/"+diaryID+"/submissions", "", otherTeacher, 404)
	subID := sub["id"].(string)
	for who, want := range map[string]int{"teacher": 200, "guardian A": 200, "student B": 404, "guardian B": 404} {
		a := map[string]actor{"teacher": teacher, "guardian A": guardA, "student B": stuB, "guardian B": guardB}[who]
		if rec := perform(router, "GET", "/diary/submissions/"+subID+"/files/"+answer, "", nil, a.headers); rec.Code != want {
			t.Fatalf("%s homework file %d", who, rec.Code)
		}
	}
	call("PATCH", "/diary/submissions/"+subID+"/review", `{"note":"Good work"}`, otherTeacher, 404)
	call("PATCH", "/diary/submissions/"+subID+"/review", `{"note":"Good work"}`, teacher, 200)
	call("PUT", "/diary/"+diaryID+"/submission", `{"answer_text":"Changed"}`, stuA, 409)
	noSubmission := data(call("POST", "/diary", diaryBody(sectionA, subjectID, time.Now().In(diary.Dhaka).AddDate(0, 0, -1).Format(time.DateOnly), false), teacher, 201))
	call("POST", "/diary/"+noSubmission["id"].(string)+"/publish", "", teacher, 200)
	if code := call("PUT", "/diary/"+noSubmission["id"].(string)+"/submission", `{"answer_text":"x"}`, stuA, 409)["error"].(map[string]any)["code"]; code != "submission_closed" {
		t.Fatalf("closed submission code %v", code)
	}
	draft := data(call("POST", "/diary", diaryBody(sectionB, subjectID, today, false), teacher, 201))
	call("DELETE", "/diary/"+draft["id"].(string), "", teacher, 200)
	call("DELETE", "/diary/"+diaryID, "", teacher, 409)

	// Diary notifications, reminders and compliance.
	call("PUT", "/diary/settings", `{"daily_required":true,"reminder_time":"00:00"}`, head, 403)
	call("PUT", "/diary/settings", `{"daily_required":true,"reminder_time":"25:00"}`, admin, 400)
	call("PUT", "/diary/settings", `{"daily_required":true,"reminder_time":"00:00"}`, admin, 200)
	result := dispatch()
	if result.Created["diary"] < 2 {
		t.Fatalf("diary notifications %+v", result)
	}
	keys := strings.Join(inboxKeys(teacher), ",")
	if time.Now().In(diary.Dhaka).Weekday() != time.Friday && !strings.Contains(keys, "diary_reminder:"+today) {
		t.Fatalf("teacher reminder missing: %s", keys)
	}
	rows := call("GET", "/diary/compliance?date="+today, "", head, 200)["data"].([]any)
	published, missing := 0, 0
	for _, r := range rows {
		row := r.(map[string]any)
		if !strings.HasPrefix(row["class_name"].(string), "M9 Class") {
			continue
		}
		if row["diary_status"] == "published" {
			published++
		} else {
			missing++
		}
	}
	if published != 1 || missing != 3 {
		t.Fatalf("compliance published=%d missing=%d %+v", published, missing, rows)
	}
	var audits int64
	db.Table("audit_events").Where("actor_user_id IN ? AND (action LIKE 'notice.%' OR action LIKE 'event.%' OR action LIKE 'diary.%' OR action LIKE 'homework.%')", userIDs).Count(&audits)
	if audits < 12 {
		t.Fatalf("audit events %d", audits)
	}
	_ = gin.H{}
}
