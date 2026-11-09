package routine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine/planner"
)

type captureRenderer struct{ docs []documents.RoutineDoc }

func (c *captureRenderer) RenderRoutine(_ context.Context, d documents.RoutineDoc) ([]byte, error) {
	c.docs = append(c.docs, d)
	return []byte("%PDF-1.7 fake"), nil
}
func (c *captureRenderer) RoutineAvailable() bool { return true }

func issueCodes(err error) []string {
	var ie *IssuesError
	if !errors.As(err, &ie) {
		return nil
	}
	out := []string{}
	for _, i := range ie.Issues {
		out = append(out, i.Code)
	}
	return out
}

func hasCode(err error, code string) bool {
	for _, c := range issueCodes(err) {
		if c == code {
			return true
		}
	}
	return false
}

// TestPostgresTermRoutinePlanner (E8): rooms, sittings, papers, the
// validator on manual placement, the generator, approval, publication,
// amendments with their changes, the student's and teacher's views and the
// routine documents.
func TestPostgresTermRoutinePlanner(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	insert := func(query string, args ...any) string {
		t.Helper()
		var id string
		if err := tx.Raw(query, args...).Row().Scan(&id); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return id
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	n := 0
	user := func(role string) string {
		n++
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", fmt.Sprintf("e8-%s%d-%s@example.test", role, n, unique), "unused", role, role+" "+unique)
	}
	admin, head := user("admin"), user("head_teacher")
	exec("DELETE FROM school_weekly_off_days")
	exec("INSERT INTO school_weekly_off_days(day_of_week) VALUES (5)")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2190','2190-01-01','2190-12-31',false) RETURNING id")
	term := insert("INSERT INTO exam_terms(session_id,name,sequence,kind,starts_on,ends_on) VALUES (?,'Half-yearly',1,'term','2190-06-01','2190-06-30') RETURNING id", session)
	nine := insert("INSERT INTO academic_classes(name,display_order) VALUES (?,9) RETURNING id", "Nine "+unique)
	six := insert("INSERT INTO academic_classes(name,display_order) VALUES (?,6) RETURNING id", "Six "+unique)
	nineA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, nine)
	nineB := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", session, nine)
	sixA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, six)
	subject := func(code, en, bn string) string {
		return insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", en+" "+unique, bn, code+unique[len(unique)-8:])
	}
	ban, mat, phy, hma, agr := subject("BAN", "Bangla", "বাংলা"), subject("MAT", "Mathematics", "গণিত"), subject("PHY", "Physics", "পদার্থবিজ্ঞান"), subject("HMA", "Higher Math", "উচ্চতর গণিত"), subject("AGR", "Agriculture", "কৃষিশিক্ষা")
	for _, m := range [][3]any{{nine, ban, false}, {nine, mat, false}, {nine, phy, false}, {nine, hma, true}, {nine, agr, true}, {six, ban, false}, {six, mat, false}} {
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,?,?)", m[0], m[1], !m[2].(bool), m[2])
	}
	students := map[string]string{}
	studentUsers := map[string]string{}
	for i, sec := range []struct{ name, class, section string }{{"n1", nine, nineA}, {"n2", nine, nineA}, {"n3", nine, nineA}, {"n4", nine, nineB}, {"n5", nine, nineB}, {"s1", six, sixA}, {"s2", six, sixA}} {
		u := user("student")
		studentUsers[sec.name] = u
		students[sec.name] = insert("INSERT INTO students(admission_no,name_en,session_id,class_id,section_id,user_id,status) VALUES (?,?,?,?,?,?,'active') RETURNING id", fmt.Sprintf("E8-%s-%d", unique, i), "Student "+sec.name, session, sec.class, sec.section, u)
	}
	teacherUsers := []string{}
	teachers := []string{}
	for i, name := range []string{"Karim", "Sara", "Rahim"} {
		u := user("teacher")
		teacherUsers = append(teacherUsers, u)
		teachers = append(teachers, insert("INSERT INTO employees(employee_code,user_id,kind,name_en,name_bn,designation,joined_on) VALUES (?,?,'teacher',?,?,'Teacher','2189-01-01') RETURNING id", fmt.Sprintf("E8%d%s", i, unique), u, "AAA "+name+" "+unique, name))
	}
	exec("INSERT INTO teacher_assignments(employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", teachers[0], session, nine, ban)
	// Other teachers in the database are fine; only these are checked by name.
	insert("INSERT INTO school_events(title_en,title_bn,kind,starts_on,ends_on,audience_roles,status,created_by,updated_by) VALUES ('Founders Day','প্রতিষ্ঠা দিবস','holiday','2190-06-05','2190-06-05',ARRAY['student'],'published',?,?) RETURNING id", admin, admin)

	s := NewTermService(NewTermRepository(tx))
	renderer := &captureRenderer{}
	s.Documents = renderer
	// Rooms.
	room := func(name, kind string, capacity int) Room {
		t.Helper()
		c := capacity
		r, err := s.SaveRoom(ctx, admin, "", RoomInput{Name: name + " " + unique, NameBN: "কক্ষ " + name, Kind: kind, Capacity: &c})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r101, r102, hall, lab := room("101", "classroom", 40), room("102", "classroom", 2), room("Hall", "hall", 100), room("Lab", "lab", 30)
	if _, err := s.SaveRoom(ctx, admin, "", RoomInput{Name: "101 " + unique, Kind: "classroom"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate room name: %v", err)
	}
	if _, err := s.SaveRoom(ctx, admin, "", RoomInput{Name: "X", Kind: "garage"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("room kind: %v", err)
	}
	// Rooms from other tests in the database must not take part: only ours
	// are active while this test runs.
	exec("UPDATE exam_rooms SET is_active=false WHERE name NOT LIKE ?", "% "+unique)

	// Version 1 with its default sittings and the term's papers.
	v, err := s.CreateVersion(ctx, admin, VersionInput{TermID: term})
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || v.Status != "draft" || v.StartsOn != "2190-06-01" || v.EndsOn != "2190-06-30" || len(v.Sittings) != 2 || len(v.Papers) != 7 || len(v.Classes) != 2 {
		t.Fatalf("new version: %+v sittings %d papers %d classes %d", v.VersionSummary, len(v.Sittings), len(v.Papers), len(v.Classes))
	}
	if v.Classes[0].Name != "Six "+unique || len(v.Classes[1].Sections) != 2 || v.Classes[1].Sections[0].Students != 3 {
		t.Fatalf("classes: %+v", v.Classes)
	}
	if !reflect.DeepEqual(v.OffDays, []int{5}) || len(v.Holidays) != 1 || v.Holidays[0].Date != "2190-06-05" {
		t.Fatalf("calendar: %v %+v", v.OffDays, v.Holidays)
	}
	for _, d := range v.Dates {
		if d == "2190-06-05" {
			t.Fatal("holiday listed as an exam day")
		}
		if day, _ := time.Parse(time.DateOnly, d); day.Weekday() == time.Friday {
			t.Fatal("Friday listed as an exam day")
		}
	}
	if _, err := s.CreateVersion(ctx, admin, VersionInput{TermID: term}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second version 1: %v", err)
	}
	if _, err := s.UpdateVersion(ctx, admin, v.ID, VersionInput{StartsOn: "2190-06-01", EndsOn: "2190-07-05"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dates outside the term: %v", err)
	}
	if v, err = s.UpdateVersion(ctx, admin, v.ID, VersionInput{StartsOn: "2190-06-01", EndsOn: "2190-06-20", Instructions: "Bring your admit card."}); err != nil || v.EndsOn != "2190-06-20" {
		t.Fatalf("update: %v", err)
	}
	morning, afternoon := v.Sittings[0], v.Sittings[1]
	if morning.Name != "Morning" || morning.StartTime != "10:00" || morning.Minutes != 180 {
		t.Fatalf("sittings: %+v", v.Sittings)
	}
	if _, err := s.SaveSitting(ctx, admin, v.ID, "", SittingInput{Name: "Overlap", StartTime: "12:00", EndTime: "14:30"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("overlapping sitting: %v", err)
	}
	// A practical in a lab.
	if v, err = s.AddPaper(ctx, admin, v.ID, PaperInput{ClassID: nine, SubjectID: phy, Kind: "practical", Duration: 120, NeedsLab: true}); err != nil || len(v.Papers) != 8 {
		t.Fatalf("add practical: %v", err)
	}
	if _, err := s.AddPaper(ctx, admin, v.ID, PaperInput{ClassID: nine, SubjectID: phy, Kind: "practical", Duration: 120}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate paper: %v", err)
	}
	paper := func(class, subject, kind string) Paper {
		t.Helper()
		for _, x := range v.Papers {
			if x.ClassID == class && x.SubjectID == subject && x.Kind == kind {
				return x
			}
		}
		t.Fatalf("paper %s %s %s missing", class, subject, kind)
		return Paper{}
	}
	nineBan, nineMat, sixBan, phyPrac := paper(nine, ban, "theory"), paper(nine, mat, "theory"), paper(six, ban, "theory"), paper(nine, phy, "practical")
	if nineBan.Students != 5 || paper(nine, hma, "theory").Students != 5 || !paper(nine, hma, "theory").Optional {
		t.Fatalf("students per paper: %+v", nineBan)
	}
	place := func(x Paper, date, sitting string, rooms map[string]string) error {
		in := Placement{Duration: x.Duration, NeedsLab: x.NeedsLab, ExamDate: date, SittingID: sitting}
		for sec, r := range rooms {
			in.Rooms = append(in.Rooms, RoomAssignment{SectionID: sec, RoomID: r})
		}
		nv, err := s.PlacePaper(ctx, admin, v.ID, x.ID, in)
		if err == nil {
			v = nv
		}
		return err
	}
	// Hard rules on manual placement.
	if err := place(nineBan, "2190-06-04", morning.ID, map[string]string{nineA: r101.ID, nineB: r102.ID}); !hasCode(err, "weekly_off") { // a Friday
		t.Fatalf("Friday: %v %v", err, issueCodes(err))
	}
	if err := place(nineBan, "2190-06-05", morning.ID, nil); !hasCode(err, "holiday") {
		t.Fatalf("holiday: %v", issueCodes(err))
	}
	var ie *IssuesError
	if err := place(nineBan, "2190-06-05", morning.ID, nil); !errors.As(err, &ie) || ie.Issues[0].Message != "2190-06-05 is Founders Day (holiday)" {
		t.Fatalf("holiday message: %+v", ie)
	}
	if err := place(nineBan, "2190-06-25", morning.ID, nil); !hasCode(err, "outside_term") {
		t.Fatalf("outside: %v", issueCodes(err))
	}
	if err := place(nineBan, "2190-06-02", morning.ID, map[string]string{nineA: r101.ID, nineB: r102.ID}); err != nil {
		t.Fatalf("place Nine Bangla: %v", err)
	}
	if err := place(sixBan, "2190-06-02", morning.ID, map[string]string{sixA: r101.ID}); !hasCode(err, "room_shared") {
		t.Fatalf("two classes in a classroom: %v", issueCodes(err))
	}
	if err := place(sixBan, "2190-06-02", morning.ID, map[string]string{sixA: r102.ID}); !hasCode(err, "room_capacity") {
		t.Fatalf("capacity: %v", issueCodes(err))
	}
	if err := place(sixBan, "2190-06-02", morning.ID, map[string]string{sixA: hall.ID}); err != nil {
		t.Fatalf("Six Bangla in the hall: %v", err)
	}
	if err := place(nineMat, "2190-06-02", morning.ID, nil); !hasCode(err, "class_clash") {
		t.Fatalf("class clash: %v", issueCodes(err))
	}
	if err := place(phyPrac, "2190-06-03", afternoon.ID, map[string]string{nineA: r101.ID, nineB: lab.ID}); !hasCode(err, "needs_lab") {
		t.Fatalf("lab: %v", issueCodes(err))
	}
	long := nineMat
	long.Duration = 200
	if err := place(long, "2190-06-03", morning.ID, nil); !hasCode(err, "too_long") {
		t.Fatalf("too long: %v", issueCodes(err))
	}
	// Optional subjects: while nobody has chosen, everyone takes both.
	hmaPaper, agrPaper := paper(nine, hma, "theory"), paper(nine, agr, "theory")
	if err := place(hmaPaper, "2190-06-03", morning.ID, map[string]string{nineA: r101.ID, nineB: hall.ID}); err != nil {
		t.Fatal(err)
	}
	if err := place(agrPaper, "2190-06-03", morning.ID, nil); !hasCode(err, "students_clash") {
		t.Fatalf("students clash: %v", issueCodes(err))
	}
	exec("INSERT INTO student_subject_choices(student_id,session_id,subject_id) VALUES (?,?,?),(?,?,?),(?,?,?),(?,?,?),(?,?,?)",
		students["n1"], session, hma, students["n2"], session, agr, students["n3"], session, agr, students["n4"], session, hma, students["n5"], session, agr)
	if err := place(agrPaper, "2190-06-03", morning.ID, map[string]string{nineA: lab.ID, nineB: r102.ID}); err != nil {
		t.Fatalf("alternatives at once: %v", err)
	}
	// Nobody in two rooms at once.
	if v, err = s.AddDuty(ctx, admin, v.ID, DutyInput{ExamDate: "2190-06-02", SittingID: morning.ID, RoomID: r101.ID, EmployeeID: teachers[0]}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddDuty(ctx, admin, v.ID, DutyInput{ExamDate: "2190-06-02", SittingID: morning.ID, RoomID: hall.ID, EmployeeID: teachers[0]}); !hasCode(err, "invigilator_clash") {
		t.Fatalf("invigilator clash: %v", issueCodes(err))
	}
	if _, err := s.DeleteSitting(ctx, admin, v.ID, morning.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete a sitting in use: %v", err)
	}
	// Submission needs every paper placed.
	if _, err := s.Move(ctx, admin, v.ID, "submitted", ""); !hasCode(err, "unscheduled") || !strings.Contains(fmt.Sprint(err), "routine_not_ready") {
		t.Fatalf("submit too early: %v", issueCodes(err))
	}
	// The generator places the rest and keeps the hand-placed papers.
	res, err := s.Generate(ctx, admin, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	v = res.Version
	if res.Report.Kept != 4 || res.Report.Placed != 4 || len(res.Report.Unplaced) != 0 {
		t.Fatalf("generate: %+v", res.Report)
	}
	if hard := planner.Hard(v.Issues); len(hard) != 0 {
		t.Fatalf("generated routine has hard issues: %+v", hard)
	}
	if p := paper(nine, ban, "theory"); *p.ExamDate != "2190-06-02" || !p.Pinned {
		t.Fatalf("pinned paper moved: %+v", p)
	}
	if p := paper(nine, phy, "practical"); len(p.Rooms) != 2 || p.Rooms[0].RoomID != lab.ID || p.Rooms[1].RoomID != lab.ID {
		t.Fatalf("practical rooms: %+v", p.Rooms)
	}
	first := map[string]string{}
	for _, x := range v.Papers {
		first[x.ID] = *x.ExamDate + *x.SittingID
	}
	res, err = s.Generate(ctx, admin, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range res.Version.Papers {
		if first[x.ID] != *x.ExamDate+*x.SittingID {
			t.Fatal("generation is not deterministic")
		}
	}
	v = res.Version
	if len(v.Duties) == 0 {
		t.Fatal("no invigilators assigned")
	}
	// Workflow: submit (admin) → approve (head teacher) → publish (admin).
	if _, err := s.Move(ctx, head, v.ID, "approved", ""); !errors.Is(err, ErrState) {
		t.Fatalf("approve a draft: %v", err)
	}
	if v, err = s.Move(ctx, admin, v.ID, "submitted", ""); err != nil || v.Status != "submitted" {
		t.Fatalf("submit: %v", err)
	}
	if _, err := s.PlacePaper(ctx, admin, v.ID, nineMat.ID, Placement{Duration: 180}); !errors.Is(err, ErrState) {
		t.Fatalf("edit a submitted routine: %v", err)
	}
	if v, err = s.Move(ctx, head, v.ID, "draft", "Move Mathematics to the afternoon"); err != nil || v.Status != "draft" || v.ReturnedReason == nil {
		t.Fatalf("return: %v", err)
	}
	if _, err := s.Move(ctx, head, v.ID, "draft", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("return without a reason: %v", err)
	}
	if v, err = s.Move(ctx, admin, v.ID, "submitted", ""); err != nil {
		t.Fatal(err)
	}
	if v, err = s.Move(ctx, head, v.ID, "approved", ""); err != nil || v.Status != "approved" || v.ApprovedByName == nil {
		t.Fatalf("approve: %v %+v", err, v.VersionSummary)
	}
	if v, err = s.Move(ctx, admin, v.ID, "published", ""); err != nil || v.Status != "published" || v.ContentSHA256 == nil || len(*v.ContentSHA256) != 64 {
		t.Fatalf("publish: %v %+v", err, v.VersionSummary)
	}
	v1 := v
	// Published content cannot change, even directly.
	sp := tx.SavePoint("frozen")
	if err := tx.Exec("UPDATE routine_papers SET duration_minutes=60 WHERE version_id=?", v1.ID).Error; err == nil {
		t.Fatal("a published paper changed")
	}
	sp.RollbackTo("frozen")
	if err := s.DeleteVersion(ctx, admin, v1.ID); !errors.Is(err, ErrState) {
		t.Fatalf("delete published: %v", err)
	}
	// The student's routine: their papers and their own room.
	mine, err := s.StudentRoutines(ctx, studentUsers["n1"])
	if err != nil || len(mine) != 1 {
		t.Fatalf("student routines: %v %d", err, len(mine))
	}
	if mine[0].Version != 1 || mine[0].AmendedOn != nil || len(mine[0].Papers) != 5 { // BAN MAT PHY PHY-practical HMA (not AGR)
		t.Fatalf("student routine: %+v", mine[0])
	}
	for _, p := range mine[0].Papers {
		if p.SubjectName == "Agriculture "+unique {
			t.Fatal("student sees an optional subject they did not choose")
		}
		if p.RoomName == "" {
			t.Fatalf("no room on %+v", p)
		}
		if p.SubjectName == "Bangla "+unique && p.RoomName != "101 "+unique {
			t.Fatalf("own room: %+v", p)
		}
	}
	if other, err := s.StudentRoutines(ctx, studentUsers["n4"]); err != nil || other[0].Papers[0].RoomName != "102 "+unique {
		t.Fatalf("section B room: %+v %v", other, err)
	}
	// The teacher's duties.
	duties, err := s.TeacherDuties(ctx, teacherUsers[0])
	if err != nil || len(duties) == 0 {
		t.Fatalf("duties: %+v %v", duties, err)
	}
	found := false
	for _, d := range duties {
		found = found || (d.ExamDate == "2190-06-02" && d.RoomName == "101 "+unique && len(d.Papers) == 1 && d.Students == 3 && strings.Contains(d.PapersBN[0], "বাংলা"))
	}
	if !found {
		t.Fatalf("hand-made duty missing: %+v", duties)
	}
	// Amendment: a new version with a reason; the old one stays in force.
	if _, err := s.Amend(ctx, admin, v1.ID, " "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("amend without reason: %v", err)
	}
	v2, err := s.Amend(ctx, admin, v1.ID, "Six Mathematics moved for the district sports day")
	if err != nil || v2.Version != 2 || v2.Status != "draft" || v2.BasedOn == nil || *v2.BasedOn != v1.ID || len(v2.Papers) != 8 || len(v2.Changes) != 0 || len(v2.Duties) != len(v1.Duties) {
		t.Fatalf("amend: %v %+v", err, v2.VersionSummary)
	}
	if _, err := s.Amend(ctx, admin, v1.ID, "again"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second open amendment: %v", err)
	}
	v = v2
	sixMat := paper(six, mat, "theory")
	target := "2190-06-17"
	if *sixMat.ExamDate == target {
		target = "2190-06-16"
	}
	if err := place(sixMat, target, v.Sittings[1].ID, map[string]string{sixA: hall.ID}); err != nil {
		t.Fatalf("move Six Mathematics: %v", err)
	}
	if len(v.Changes) != 1 || v.Changes[0].Change != "moved" || v.Changes[0].ToDate != target || v.Changes[0].FromDate != *sixMat.ExamDate || v.PreviousLabel == nil || *v.PreviousLabel != 1 {
		t.Fatalf("changes: %+v", v.Changes)
	}
	for _, step := range []struct{ actor, to string }{{admin, "submitted"}, {head, "approved"}, {admin, "published"}} {
		if v, err = s.Move(ctx, step.actor, v.ID, step.to, ""); err != nil {
			t.Fatalf("%s: %v", step.to, err)
		}
	}
	old, err := s.GetVersion(ctx, v1.ID)
	if err != nil || old.Status != "superseded" || old.SupersededAt == nil {
		t.Fatalf("old version: %v %s", err, old.Status)
	}
	sixRoutine, err := s.StudentRoutines(ctx, studentUsers["s1"])
	if err != nil || len(sixRoutine) != 1 || sixRoutine[0].Version != 2 || sixRoutine[0].AmendedOn == nil || sixRoutine[0].Reason == nil {
		t.Fatalf("amended routine: %+v %v", sixRoutine, err)
	}
	changed := 0
	for _, p := range sixRoutine[0].Papers {
		if p.Changed {
			changed++
			if p.ExamDate != target {
				t.Fatalf("changed paper: %+v", p)
			}
		}
	}
	if changed != 1 {
		t.Fatalf("%d papers marked changed", changed)
	}
	terms, err := s.ListTerms(ctx, session)
	if err != nil || len(terms) != 1 || len(terms[0].Versions) != 2 || terms[0].Versions[0].Version != 2 || terms[0].Versions[0].Placed != 8 {
		t.Fatalf("terms: %+v %v", terms, err)
	}
	// Audit trail.
	for _, action := range []string{"routine.version_created", "routine.paper_placed", "routine.generated", "routine.version_submitted", "routine.version_returned", "routine.version_approved", "routine.version_published", "routine.version_amended", "routine.duty_added", "routine.room_saved"} {
		var count int64
		if err := tx.Table("audit_events").Where("action=? AND created_at >= now()", action).Count(&count).Error; err != nil || count == 0 {
			t.Fatalf("audit %s: %d %v", action, count, err)
		}
	}
	var raw []byte
	if err := tx.Raw("SELECT metadata FROM audit_events WHERE action='routine.generated' AND target_id=? ORDER BY created_at DESC LIMIT 1", v1.ID).Row().Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"placed": 4`) && !strings.Contains(string(raw), `"placed":4`) {
		t.Fatalf("generated audit: %s", raw)
	}
	// Documents.
	if _, err := s.Document(ctx, v.ID, DocumentRequest{Kind: "class", Language: "bn"}); err != nil {
		t.Fatal(err)
	}
	d := renderer.docs[len(renderer.docs)-1]
	if d.Kind != "class" || d.Language != "bn" || d.Version != 2 || d.Status != "published" || len(d.Pages) != 2 || len(d.Holidays) != 1 || d.Reason == "" || d.Instructions != "Bring your admit card." {
		t.Fatalf("class document: %+v", d)
	}
	if len(d.Pages[1].Rows) != 6 || d.Pages[1].Rows[0].Papers[0].Rooms[0].Section != "A" {
		t.Fatalf("Nine page: %+v", d.Pages[1])
	}
	if _, err := s.Document(ctx, v.ID, DocumentRequest{Kind: "room", RoomID: lab.ID}); err != nil {
		t.Fatal(err)
	}
	d = renderer.docs[len(renderer.docs)-1]
	if len(d.Pages) != 1 || len(d.Pages[0].Rows) < 2 || d.Pages[0].Note.EN != "Laboratory · seats 30" {
		t.Fatalf("lab document: %+v", d.Pages)
	}
	if _, err := s.Document(ctx, v.ID, DocumentRequest{Kind: "invigilation"}); err != nil {
		t.Fatal(err)
	}
	d = renderer.docs[len(renderer.docs)-1]
	staffed := 0
	for _, r := range d.Pages[0].Rows {
		staffed += len(r.Invigilators)
	}
	// Three teachers cannot staff every room of a busy sitting: those rooms
	// are listed without an invigilator, and the check warns about them.
	if len(d.Pages) != 1 || staffed != len(v.Duties) || len(d.Pages[0].Rows) < len(v.Duties) {
		t.Fatalf("duty document: rows %d staffed %d duties %d", len(d.Pages[0].Rows), staffed, len(v.Duties))
	}
	unstaffed := 0
	for _, i := range v.Issues {
		if i.Code == "no_invigilator" {
			unstaffed++
		}
	}
	if unstaffed != len(d.Pages[0].Rows)-len(v.Duties) {
		t.Fatalf("no_invigilator warnings %d", unstaffed)
	}
	if _, err := s.StudentDocument(ctx, studentUsers["n4"], v.ID, "en"); err != nil {
		t.Fatal(err)
	}
	d = renderer.docs[len(renderer.docs)-1]
	// n4 (section B) chose Higher Math: Bangla, Mathematics, Physics, the
	// Physics practical and Higher Math, without Agriculture.
	if len(d.Pages) != 1 || len(d.Pages[0].Rows) != 5 || !strings.HasSuffix(d.Pages[0].Title.EN, " B") {
		t.Fatalf("student document: %+v", d.Pages)
	}
	for _, r := range d.Pages[0].Rows {
		for _, p := range r.Papers {
			if len(p.Rooms) != 1 || p.Rooms[0].Section != "B" {
				t.Fatalf("other section on a student's routine: %+v", p.Rooms)
			}
		}
	}
	if _, err := s.StudentDocument(ctx, studentUsers["n4"], v1.ID, "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("superseded version for a student: %v", err)
	}
	s.Documents = nil
	if _, err := s.Document(ctx, v.ID, DocumentRequest{Kind: "class"}); !errors.Is(err, ErrDocuments) {
		t.Fatalf("no renderer: %v", err)
	}
}
