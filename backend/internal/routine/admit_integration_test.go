package routine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/database"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
)

type admitCapture struct{ docs []documents.AdmitDoc }

func (c *admitCapture) RenderAdmit(_ context.Context, d documents.AdmitDoc) ([]byte, error) {
	c.docs = append(c.docs, d)
	return []byte("%PDF-1.7 fake"), nil
}
func (c *admitCapture) RoutineAvailable() bool { return true }

// TestPostgresAdmitCards covers admit cards from a published routine: who
// may print what, each student's own papers with their room and seat
// (seats numbered across the classes sharing a hall), the fee hold with its
// release, the student's own card, and the public check of the QR code.
func TestPostgresAdmitCards(t *testing.T) {
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
		return insert("INSERT INTO users(email,password_hash,role,display_name) VALUES (?,?,?,?) RETURNING id", fmt.Sprintf("ac-%s%d-%s@example.test", role, n, unique), "unused", role, role+" "+unique)
	}
	admin, head, accountant := user("admin"), user("head_teacher"), user("accountant")
	exec("DELETE FROM school_weekly_off_days")
	exec("INSERT INTO school_weekly_off_days(day_of_week) VALUES (5)")
	exec("UPDATE admit_card_settings SET rules_en = '', rules_bn = '', withhold_for_dues = false, dues_cutoff = NULL WHERE id = 1")
	session := insert("INSERT INTO academic_sessions(name,start_date,end_date,is_active) VALUES ('2191','2191-01-01','2191-12-31',false) RETURNING id")
	term := insert("INSERT INTO exam_terms(session_id,name,sequence,kind,starts_on,ends_on) VALUES (?,'Annual',2,'term','2191-06-01','2191-06-20') RETURNING id", session)
	nine := insert("INSERT INTO academic_classes(name,display_order) VALUES (?,9) RETURNING id", "Nine "+unique)
	six := insert("INSERT INTO academic_classes(name,display_order) VALUES (?,6) RETURNING id", "Six "+unique)
	nineA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, nine)
	nineB := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'B','B') RETURNING id", session, nine)
	sixA := insert("INSERT INTO academic_sections(session_id,class_id,name,code) VALUES (?,?,'A','A') RETURNING id", session, six)
	subject := func(code, en, bn string) string {
		return insert("INSERT INTO academic_subjects(name_en,name_bn,code) VALUES (?,?,?) RETURNING id", en+" "+unique, bn, code+unique[len(unique)-8:])
	}
	ban, mat := subject("BAN", "Bangla", "বাংলা"), subject("MAT", "Mathematics", "গণিত")
	for _, m := range [][2]string{{nine, ban}, {nine, mat}, {six, ban}, {six, mat}} {
		exec("INSERT INTO academic_class_subjects(class_id,subject_id,is_required,is_fourth_subject) VALUES (?,?,true,false)", m[0], m[1])
	}
	students, users := map[string]string{}, map[string]string{}
	for i, x := range []struct {
		name, class, section string
		roll                 int
	}{{"n1", nine, nineA, 2}, {"n2", nine, nineA, 1}, {"n3", nine, nineB, 1}, {"s1", six, sixA, 1}} {
		u := user("student")
		users[x.name] = u
		students[x.name] = insert("INSERT INTO students(admission_no,name_en,name_bn,roll_no,session_id,class_id,section_id,user_id,status) VALUES (?,?,?,?,?,?,?,?,'active') RETURNING id",
			fmt.Sprintf("AC-%s-%d", unique, i), "Student "+x.name, "শিক্ষার্থী "+x.name, x.roll, session, x.class, x.section, u)
	}
	classTeacher, otherTeacher := user("teacher"), user("teacher")
	emp := insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Class Teacher','Teacher','2190-01-01') RETURNING id", "AC1"+unique[len(unique)-10:], classTeacher)
	insert("INSERT INTO employees(employee_code,user_id,kind,name_en,designation,joined_on) VALUES (?,?,'teacher','Other','Teacher','2190-01-01') RETURNING id", "AC2"+unique[len(unique)-10:], otherTeacher)
	exec("INSERT INTO section_class_teachers(section_id,class_id,session_id,employee_id) VALUES (?,?,?,?)", nineA, nine, session, emp)

	// A routine: Bangla for Nine (A and B) and Six (A) in one hall at the
	// same sitting; Mathematics in room 101.
	ts := NewTermService(NewTermRepository(tx))
	room := func(name, kind string, capacity int) Room {
		t.Helper()
		r, err := ts.SaveRoom(ctx, admin, "", RoomInput{Name: name + " " + unique, NameBN: "কক্ষ " + name, Kind: kind, Capacity: &capacity})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	hall, r101 := room("Hall", "hall", 100), room("101", "classroom", 40)
	exec("UPDATE exam_rooms SET is_active=false WHERE name NOT LIKE ?", "% "+unique)
	v, err := ts.CreateVersion(ctx, admin, VersionInput{TermID: term})
	if err != nil {
		t.Fatal(err)
	}
	morning, afternoon := v.Sittings[0], v.Sittings[1]
	place := func(class, subject, date, sitting string, rooms ...RoomAssignment) {
		t.Helper()
		for _, x := range v.Papers {
			if x.ClassID == class && x.SubjectID == subject {
				if v, err = ts.PlacePaper(ctx, admin, v.ID, x.ID, Placement{Duration: x.Duration, ExamDate: date, SittingID: sitting, Rooms: rooms}); err != nil {
					t.Fatalf("place %s: %v %v", date, err, issueCodes(err))
				}
				return
			}
		}
		t.Fatalf("no paper %s %s", class, subject)
	}
	place(nine, ban, "2191-06-02", morning.ID, RoomAssignment{nineA, hall.ID}, RoomAssignment{nineB, hall.ID})
	place(six, ban, "2191-06-02", morning.ID, RoomAssignment{sixA, hall.ID})
	place(nine, mat, "2191-06-04", morning.ID, RoomAssignment{nineA, r101.ID}, RoomAssignment{nineB, r101.ID})
	place(six, mat, "2191-06-04", afternoon.ID, RoomAssignment{sixA, r101.ID})

	as := NewAdmitService(tx)
	capture := &admitCapture{}
	as.Documents = capture
	adminA, headA, ctA, otherA, accA := AdmitActor{admin, "admin"}, AdmitActor{head, "head_teacher"}, AdmitActor{classTeacher, "teacher"}, AdmitActor{otherTeacher, "teacher"}, AdmitActor{accountant, "accountant"}

	// Nothing before publication.
	if _, _, err := as.Print(ctx, adminA, term, AdmitPrint{ClassID: nine}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cards of an unpublished routine: %v", err)
	}
	if mine, err := as.Mine(ctx, users["n1"]); err != nil || len(mine) != 0 {
		t.Fatalf("a student's cards before publication: %+v %v", mine, err)
	}
	for _, to := range []struct{ who, status string }{{admin, "submitted"}, {head, "approved"}, {admin, "published"}} {
		if v, err = ts.Move(ctx, to.who, v.ID, to.status, ""); err != nil {
			t.Fatalf("%s: %v", to.status, err)
		}
	}

	// Who may print what.
	terms, err := as.Terms(ctx, ctA)
	if err != nil || len(terms) == 0 || terms[0].TermID != term || len(terms[0].Sections) != 1 || terms[0].Sections[0].SectionID != nineA || terms[0].Sections[0].Students != 2 {
		t.Fatalf("the class teacher's terms: %+v %v", terms, err)
	}
	if all, err := as.Terms(ctx, headA); err != nil || len(all[0].Sections) != 3 {
		t.Fatalf("the head teacher's terms: %+v %v", all, err)
	}
	if _, _, err := as.Print(ctx, ctA, term, AdmitPrint{SectionID: nineA}); err != nil {
		t.Fatalf("the class teacher's section: %v", err)
	}
	for name, try := range map[string]func() error{
		"another section":         func() error { _, _, err := as.Print(ctx, ctA, term, AdmitPrint{SectionID: nineB}); return err },
		"a whole class (teacher)": func() error { _, _, err := as.Print(ctx, ctA, term, AdmitPrint{ClassID: nine}); return err },
		"a teacher of no section": func() error { _, _, err := as.Print(ctx, otherA, term, AdmitPrint{SectionID: nineA}); return err },
		"the accountant":          func() error { _, _, err := as.Print(ctx, accA, term, AdmitPrint{SectionID: nineA}); return err },
		"settings by the head":    func() error { _, err := as.SaveSettings(ctx, headA, AdmitSettings{}); return err },
		"a release by a teacher":  func() error { return as.Release(ctx, ctA, term, students["n1"], "") },
		"a release by the head":   func() error { return as.Release(ctx, headA, term, students["n1"], "") },
	} {
		if err := try(); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s: %v", name, err)
		}
	}

	// The class's cards: each student's own papers, room and seat. In the
	// hall, seats run Six A (class order 6), then Nine A by roll, then Nine B.
	exec("UPDATE students SET student_code = 'N2-CODE' WHERE name_en = 'Student n2' AND session_id = ?", session)
	pdf, held, err := as.Print(ctx, headA, term, AdmitPrint{ClassID: nine, Language: "bn"})
	if err != nil || len(pdf) == 0 || held != 0 {
		t.Fatalf("print: %v held %d", err, held)
	}
	doc := capture.docs[len(capture.docs)-1]
	if doc.Language != "bn" || doc.Term != "Annual" || doc.Version != 1 || len(doc.Cards) != 3 {
		t.Fatalf("document: %+v", doc)
	}
	if doc.Cards[0].StudentCode != "N2-CODE" || doc.Cards[1].StudentCode != "" {
		t.Fatalf("student IDs: %q %q", doc.Cards[0].StudentCode, doc.Cards[1].StudentCode)
	}
	order := []string{doc.Cards[0].Name.EN, doc.Cards[1].Name.EN, doc.Cards[2].Name.EN}
	if order[0] != "Student n2" || order[1] != "Student n1" || order[2] != "Student n3" {
		t.Fatalf("card order (section, roll): %v", order)
	}
	seat := func(card documents.AdmitCard, date string) (string, int) {
		for _, p := range card.Papers {
			if p.Date == date {
				return p.Room.EN, p.Seat
			}
		}
		return "", -1
	}
	for i, want := range []struct {
		hallSeat, roomSeat int
	}{{2, 1}, {3, 2}, {4, 3}} {
		c := doc.Cards[i]
		if len(c.Papers) != 2 || c.Token == "" {
			t.Fatalf("card %d: %+v", i, c)
		}
		if r, s := seat(c, "2191-06-02"); r != hall.Name || s != want.hallSeat {
			t.Fatalf("%s in the hall: %s seat %d, want %d", c.Name.EN, r, s, want.hallSeat)
		}
		if r, s := seat(c, "2191-06-04"); r != r101.Name || s != want.roomSeat {
			t.Fatalf("%s in 101: %s seat %d, want %d", c.Name.EN, r, s, want.roomSeat)
		}
	}
	if p := doc.Cards[0].Papers[0]; p.Subject.BN != "বাংলা" || p.Start != morning.Start || p.End != morning.End {
		t.Fatalf("paper: %+v", p)
	}
	token := doc.Cards[0].Token

	// The fee hold: off by default; on, n1 (a due unpaid before the cutoff)
	// is held, n2 (paid) and n3 (a due after the cutoff) are not.
	cat := insert("INSERT INTO fee_categories(code,name_en,frequency,created_by,updated_by) VALUES (?,'Tuition','monthly',?,?) RETURNING id", "AC"+unique[len(unique)-8:], admin, admin)
	structure := insert("INSERT INTO fee_structures(session_id,class_id,category_id,amount,created_by,updated_by) VALUES (?,?,?,500,?,?) RETURNING id", session, nine, cat, admin, admin)
	due := func(student, date string, paid float64) {
		exec("INSERT INTO student_fees(student_id,structure_id,session_id,class_id,period,due_date,amount,paid_amount,created_by) VALUES (?,?,?,?,date_trunc('month', ?::date)::date,?,500,?,?)",
			student, structure, session, nine, date, date, paid, admin)
	}
	due(students["n1"], "2191-05-10", 0)
	due(students["n2"], "2191-05-10", 500)
	due(students["n3"], "2191-07-10", 0)
	if _, err := as.SaveSettings(ctx, adminA, AdmitSettings{WithholdForDues: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a hold without a cutoff: %v", err)
	}
	cutoff := "2191-05-31"
	if s, err := as.SaveSettings(ctx, adminA, AdmitSettings{RulesEN: "Rule one.\nRule two.", WithholdForDues: true, DuesCutoff: &cutoff}); err != nil || !s.WithholdForDues || s.DefaultRulesEN == "" {
		t.Fatalf("settings: %+v %v", s, err)
	}
	list, err := as.Students(ctx, accA, term, "", "", true)
	if err != nil || len(list) != 1 || list[0].StudentID != students["n1"] || !list[0].Held || list[0].Due != "500.00" || list[0].OldestDue == nil || *list[0].OldestDue != "2191-05-10" {
		t.Fatalf("held list: %+v %v", list, err)
	}
	if _, held, err = as.Print(ctx, adminA, term, AdmitPrint{ClassID: nine}); err != nil || held != 1 || len(capture.docs[len(capture.docs)-1].Cards) != 2 {
		t.Fatalf("print with a hold: held %d %v", held, err)
	}
	if got := capture.docs[len(capture.docs)-1].Rules.EN; got != "Rule one.\nRule two." {
		t.Fatalf("rules: %q", got)
	}
	if mine, err := as.Mine(ctx, users["n1"]); err != nil || len(mine) != 1 || !mine[0].Held {
		t.Fatalf("the held student's cards: %+v %v", mine, err)
	}
	if _, err := as.MinePDF(ctx, users["n1"], term, "en"); !errors.Is(err, ErrHeld) {
		t.Fatalf("a held student's own card: %v", err)
	}
	// The office releases n1.
	if err := as.Release(ctx, accA, term, students["n1"], "Promised to pay on Sunday"); err != nil {
		t.Fatal(err)
	}
	if list, _ = as.Students(ctx, accA, term, "", "", true); len(list) != 1 || list[0].Held || !list[0].Released || list[0].ReleaseNote != "Promised to pay on Sunday" {
		t.Fatalf("after release: %+v", list)
	}
	if _, err := as.MinePDF(ctx, users["n1"], term, "en"); err != nil {
		t.Fatalf("a released student's own card: %v", err)
	}
	if own := capture.docs[len(capture.docs)-1]; len(own.Cards) != 1 || own.Cards[0].Name.EN != "Student n1" || own.Cards[0].Token == "" {
		t.Fatalf("own card: %+v", own.Cards)
	}
	if err := as.Unrelease(ctx, adminA, term, students["n1"]); err != nil {
		t.Fatal(err)
	}
	if _, err := as.MinePDF(ctx, users["n1"], term, "en"); !errors.Is(err, ErrHeld) {
		t.Fatalf("held again: %v", err)
	}
	// A section where everyone is held prints nothing.
	exec("UPDATE student_fees SET paid_amount = 0 WHERE student_id = ?", students["n2"])
	if _, _, err := as.Print(ctx, ctA, term, AdmitPrint{SectionID: nineA}); !errors.Is(err, ErrNoCards) {
		t.Fatalf("everyone held: %v", err)
	}
	// Another student's card does not exist for a student.
	if _, err := as.MinePDF(ctx, users["s1"], "00000000-0000-0000-0000-000000000000", "en"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a term without a routine: %v", err)
	}

	// The QR check: current, then superseded once an amendment is
	// published. No name in it.
	check, err := as.Verify(ctx, token)
	if err != nil || check.Status != "current" || check.Term != "Annual" || check.Section != "A" || check.Version != 1 {
		t.Fatalf("verify: %+v %v", check, err)
	}
	if _, err := as.Verify(ctx, "AAAAAAAAAAAAAAAAAAAAAA"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown code: %v", err)
	}
	v2, err := ts.Amend(ctx, admin, v.ID, "Mathematics moved")
	if err != nil {
		t.Fatal(err)
	}
	for _, to := range []struct{ who, status string }{{admin, "submitted"}, {head, "approved"}, {admin, "published"}} {
		if v2, err = ts.Move(ctx, to.who, v2.ID, to.status, ""); err != nil {
			t.Fatalf("amendment %s: %v", to.status, err)
		}
	}
	if check, err = as.Verify(ctx, token); err != nil || check.Status != "superseded" {
		t.Fatalf("after the amendment: %+v %v", check, err)
	}
}
