package routine

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
	"gorm.io/gorm"
)

// Admit cards (প্রবেশপত্র, migration 1806) come from a term's published
// routine: each student's own papers, with the room their section sits in
// and a seat. Seats are numbered within each room and sitting in a fixed
// order (class, section, roll), so two students never share one and the
// same routine always gives the same seats.
//
// Access: the administrator and the head teacher print every section; a
// teacher prints the sections they are class teacher of; a student
// downloads their own once the routine is published. With the fee hold on,
// a student with a due unpaid on or before the cutoff date gets no card
// until the office releases them for that term.

var (
	// ErrHeld: the student's admit card is held for unpaid dues.
	ErrHeld = errors.New("admit card held for dues")
	// ErrNoCards: nobody in the request may have a card (all held, or no
	// students).
	ErrNoCards = errors.New("no admit cards to print")
)

// AdmitRenderer renders admit cards (the documents service).
type AdmitRenderer interface {
	RenderAdmit(context.Context, documents.AdmitDoc) ([]byte, error)
	RoutineAvailable() bool
}

// AdmitActor is who asks.
type AdmitActor struct {
	ID   string
	Role string
}

// all: the administrator and the head teacher print every section.
func (a AdmitActor) all() bool { return a.Role == "admin" || a.Role == "head_teacher" }

// office: the accountant sees every section's held students.
func (a AdmitActor) office() bool { return a.Role == "accountant" }

// AdmitSettings are the rules printed on cards and the fee hold.
type AdmitSettings struct {
	RulesEN         string  `json:"rules_en"`
	RulesBN         string  `json:"rules_bn"`
	WithholdForDues bool    `json:"withhold_for_dues"`
	DuesCutoff      *string `json:"dues_cutoff"`
	DefaultRulesEN  string  `json:"default_rules_en" gorm:"-"`
	DefaultRulesBN  string  `json:"default_rules_bn" gorm:"-"`
}

// AdmitSection is a section a member of staff may print cards for.
type AdmitSection struct {
	SectionID   string `json:"section_id"`
	SectionName string `json:"section_name"`
	ClassID     string `json:"class_id"`
	ClassName   string `json:"class_name"`
	Students    int    `json:"students"`
	Held        int    `json:"held"`
}

// AdmitTerm is a term with a published routine.
type AdmitTerm struct {
	TermID      string         `json:"term_id"`
	TermName    string         `json:"term_name"`
	SessionID   string         `json:"session_id"`
	SessionName string         `json:"session_name"`
	VersionID   string         `json:"version_id"`
	Version     int            `json:"version"`
	PublishedAt string         `json:"published_at"`
	Sections    []AdmitSection `json:"sections" gorm:"-"`
}

// AdmitStudent is one student of a term's cards, with the fee hold.
type AdmitStudent struct {
	StudentID   string  `json:"student_id"`
	NameEN      string  `json:"name_en"`
	NameBN      string  `json:"name_bn"`
	AdmissionNo string  `json:"admission_no"`
	RollNo      *int    `json:"roll_no"`
	ClassID     string  `json:"class_id"`
	ClassName   string  `json:"class_name"`
	SectionID   string  `json:"section_id"`
	SectionName string  `json:"section_name"`
	Due         string  `json:"due"`        // unpaid by the cutoff, "0.00" when none
	OldestDue   *string `json:"oldest_due"` // YYYY-MM-DD
	Held        bool    `json:"held"`
	Released    bool    `json:"released"`
	ReleaseNote string  `json:"release_note"`
}

// MyAdmitCard is a student's card of one term.
type MyAdmitCard struct {
	TermID      string `json:"term_id"`
	TermName    string `json:"term_name"`
	VersionID   string `json:"version_id"`
	Version     int    `json:"version"`
	PublishedAt string `json:"published_at"`
	Held        bool   `json:"held"`
}

// AdmitVerification is what the public check of a card's QR code shows: no
// name or identifier.
type AdmitVerification struct {
	Status      string `json:"status"` // current or superseded
	SchoolName  string `json:"school_name"`
	SchoolBN    string `json:"school_name_bn"`
	Term        string `json:"term"`
	Session     string `json:"session"`
	Class       string `json:"class"`
	Section     string `json:"section"`
	Version     int    `json:"version"`
	PublishedAt string `json:"published_at"`
	IssuedAt    string `json:"issued_at"`
}

// AdmitPrint names the cards to print: a section, or a whole class
// (administrator and head teacher).
type AdmitPrint struct {
	ClassID   string
	SectionID string
	Language  string
}

type AdmitService struct {
	db *gorm.DB
	// Documents renders the cards; nil turns the PDF routes off.
	Documents AdmitRenderer
}

func NewAdmitService(db *gorm.DB) *AdmitService { return &AdmitService{db: db} }

// Available reports whether cards can be rendered.
func (s *AdmitService) Available() bool {
	return s != nil && s.Documents != nil && s.Documents.RoutineAvailable()
}

// ---------------------------------------------------------------------------
// Settings

func (s *AdmitService) Settings(ctx context.Context) (AdmitSettings, error) {
	var out AdmitSettings
	err := s.db.WithContext(ctx).Raw("SELECT rules_en, rules_bn, withhold_for_dues, to_char(dues_cutoff, 'YYYY-MM-DD') AS dues_cutoff FROM admit_card_settings WHERE id = 1").Scan(&out).Error
	out.DefaultRulesEN, out.DefaultRulesBN = documents.DefaultAdmitRules.EN, documents.DefaultAdmitRules.BN
	return out, err
}

// SaveSettings changes the rules and the fee hold (administrators).
func (s *AdmitService) SaveSettings(ctx context.Context, a AdmitActor, in AdmitSettings) (AdmitSettings, error) {
	if a.Role != "admin" {
		return AdmitSettings{}, ErrForbidden
	}
	in.RulesEN, in.RulesBN = strings.TrimSpace(in.RulesEN), strings.TrimSpace(in.RulesBN)
	if !utf8.ValidString(in.RulesEN) || !utf8.ValidString(in.RulesBN) || utf8.RuneCountInString(in.RulesEN) > 2000 || utf8.RuneCountInString(in.RulesBN) > 2000 {
		return AdmitSettings{}, ErrInvalid
	}
	if in.DuesCutoff != nil && *in.DuesCutoff == "" {
		in.DuesCutoff = nil
	}
	if (in.DuesCutoff != nil && !validDay(*in.DuesCutoff)) || (in.WithholdForDues && in.DuesCutoff == nil) {
		return AdmitSettings{}, ErrInvalid
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE admit_card_settings SET rules_en = ?, rules_bn = ?, withhold_for_dues = ?, dues_cutoff = ?, updated_by = ?, updated_at = now() WHERE id = 1",
			in.RulesEN, in.RulesBN, in.WithholdForDues, in.DuesCutoff, a.ID).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &a.ID, "admit_card.settings_saved", "admit_card_settings", "1", map[string]any{"withhold_for_dues": in.WithholdForDues, "dues_cutoff": in.DuesCutoff})
	})
	if err != nil {
		return AdmitSettings{}, err
	}
	return s.Settings(ctx)
}

// ---------------------------------------------------------------------------
// Who may print what

// sectionFilter narrows a query of sections (alias sec) to the actor's.
func sectionFilter(a AdmitActor) (string, []any) {
	if a.all() || a.office() {
		return "", nil
	}
	return ` AND EXISTS (SELECT 1 FROM section_class_teachers ct JOIN employees e ON e.id = ct.employee_id
 WHERE ct.section_id = sec.id AND e.user_id = ? AND e.is_active)`, []any{a.ID}
}

// held is true for a student (alias st) whose card is held for the term
// (an SQL expression, "?" for a parameter); cfg is admit_card_settings.
func held(term string) string {
	return `(cfg.withhold_for_dues AND EXISTS (SELECT 1 FROM student_fees f WHERE f.student_id = st.id AND f.balance > 0 AND f.due_date <= cfg.dues_cutoff)
 AND NOT EXISTS (SELECT 1 FROM admit_card_releases rel WHERE rel.term_id = ` + term + ` AND rel.student_id = st.id))`
}

// Terms lists the terms with a published routine, newest session first,
// each with the sections the actor may print.
func (s *AdmitService) Terms(ctx context.Context, a AdmitActor) ([]AdmitTerm, error) {
	if !a.all() && !a.office() && a.Role != "teacher" {
		return nil, ErrForbidden
	}
	db := s.db.WithContext(ctx)
	out := []AdmitTerm{}
	if err := db.Raw(`SELECT t.id AS term_id, t.name AS term_name, ses.id AS session_id, ses.name AS session_name, v.id AS version_id, v.version,
 to_char(v.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS published_at
 FROM routine_versions v JOIN exam_terms t ON t.id = v.term_id JOIN academic_sessions ses ON ses.id = t.session_id
 WHERE v.status = 'published' ORDER BY ses.start_date DESC, t.sequence DESC LIMIT 50`).Scan(&out).Error; err != nil {
		return nil, err
	}
	filter, args := sectionFilter(a)
	kept := out[:0]
	for _, t := range out {
		t.Sections = []AdmitSection{}
		q := `SELECT sec.id AS section_id, sec.name AS section_name, c.id AS class_id, c.name AS class_name,
 count(st.id) AS students, count(st.id) FILTER (WHERE ` + held("?") + `) AS held
 FROM academic_sections sec JOIN academic_classes c ON c.id = sec.class_id CROSS JOIN admit_card_settings cfg
 LEFT JOIN students st ON st.section_id = sec.id AND st.session_id = sec.session_id AND st.status = 'active'
 WHERE sec.session_id = ? AND sec.is_active AND EXISTS (SELECT 1 FROM routine_papers p WHERE p.version_id = ? AND p.class_id = sec.class_id)` + filter + `
 GROUP BY sec.id, sec.name, c.id, c.name, c.display_order, sec.display_order ORDER BY c.display_order, c.name, sec.display_order, sec.name`
		if err := db.Raw(q, append([]any{t.TermID, t.SessionID, t.VersionID}, args...)...).Scan(&t.Sections).Error; err != nil {
			return nil, err
		}
		if len(t.Sections) > 0 {
			kept = append(kept, t)
		}
	}
	return kept, nil
}

type publishedRoutine struct {
	VersionID, TermName, SessionID, SessionName, PublishedAt string
	Version                                                  int
}

func (s *AdmitService) published(tx *gorm.DB, termID string) (publishedRoutine, error) {
	var p publishedRoutine
	if !uuid.MatchString(termID) {
		return p, ErrInvalid
	}
	err := tx.Raw(`SELECT v.id AS version_id, v.version, t.name AS term_name, ses.id AS session_id, ses.name AS session_name,
 to_char(v.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS published_at
 FROM routine_versions v JOIN exam_terms t ON t.id = v.term_id JOIN academic_sessions ses ON ses.id = t.session_id
 WHERE v.term_id = ? AND v.status = 'published'`, termID).Scan(&p).Error
	if err == nil && p.VersionID == "" {
		err = ErrNotFound
	}
	return p, err
}

// Students lists a term's students with the fee hold: one section, a
// class, or (empty) every section the actor may print. dueOnly keeps the
// students with dues by the cutoff (held or released); the office (an
// accountant) sees every section.
func (s *AdmitService) Students(ctx context.Context, a AdmitActor, termID, classID, sectionID string, dueOnly bool) ([]AdmitStudent, error) {
	if (classID != "" && !uuid.MatchString(classID)) || (sectionID != "" && !uuid.MatchString(sectionID)) {
		return nil, ErrInvalid
	}
	db := s.db.WithContext(ctx)
	p, err := s.published(db, termID)
	if err != nil {
		return nil, err
	}
	q := `SELECT st.id AS student_id, st.name_en, st.name_bn, st.admission_no, st.roll_no, c.id AS class_id, c.name AS class_name, sec.id AS section_id, sec.name AS section_name,
 to_char(coalesce((SELECT sum(f.balance) FROM student_fees f WHERE f.student_id = st.id AND f.balance > 0 AND f.due_date <= cfg.dues_cutoff), 0), 'FM999999990.00') AS due,
 (SELECT to_char(min(f.due_date), 'YYYY-MM-DD') FROM student_fees f WHERE f.student_id = st.id AND f.balance > 0 AND f.due_date <= cfg.dues_cutoff) AS oldest_due,
 ` + held("?") + ` AS held,
 EXISTS (SELECT 1 FROM admit_card_releases r WHERE r.term_id = ? AND r.student_id = st.id) AS released,
 coalesce((SELECT r.note FROM admit_card_releases r WHERE r.term_id = ? AND r.student_id = st.id), '') AS release_note
 FROM students st JOIN academic_sections sec ON sec.id = st.section_id JOIN academic_classes c ON c.id = st.class_id CROSS JOIN admit_card_settings cfg
 WHERE st.session_id = ? AND st.status = 'active' AND EXISTS (SELECT 1 FROM routine_papers p WHERE p.version_id = ? AND p.class_id = st.class_id)`
	args := []any{termID, termID, termID, p.SessionID, p.VersionID}
	if classID != "" {
		q += " AND st.class_id = ?"
		args = append(args, classID)
	}
	if sectionID != "" {
		q += " AND st.section_id = ?"
		args = append(args, sectionID)
	}
	if dueOnly {
		q += " AND cfg.dues_cutoff IS NOT NULL AND EXISTS (SELECT 1 FROM student_fees f WHERE f.student_id = st.id AND f.balance > 0 AND f.due_date <= cfg.dues_cutoff)"
	}
	filter, more := sectionFilter(a)
	q += filter
	args = append(args, more...)
	q += " ORDER BY c.display_order, c.name, sec.display_order, sec.name, st.roll_no NULLS LAST, st.admission_no"
	out := []AdmitStudent{}
	if err := db.Raw(q, args...).Scan(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Release lets a held student have their card for a term (the office).
func (s *AdmitService) Release(ctx context.Context, a AdmitActor, termID, studentID, note string) error {
	if a.Role != "admin" && a.Role != "accountant" {
		return ErrForbidden
	}
	note = strings.TrimSpace(note)
	if !uuid.MatchString(termID) || !uuid.MatchString(studentID) || utf8.RuneCountInString(note) > 240 {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ok int64
		if err := tx.Raw(`SELECT count(*) FROM students st JOIN exam_terms t ON t.session_id = st.session_id WHERE st.id = ? AND t.id = ?`, studentID, termID).Scan(&ok).Error; err != nil {
			return err
		}
		if ok == 0 {
			return ErrNotFound
		}
		if err := tx.Exec(`INSERT INTO admit_card_releases (term_id, student_id, note, released_by) VALUES (?, ?, ?, ?)
 ON CONFLICT (term_id, student_id) DO UPDATE SET note = EXCLUDED.note, released_by = EXCLUDED.released_by, released_at = now()`, termID, studentID, note, a.ID).Error; err != nil {
			return dbError(err)
		}
		return audit.Record(tx, &a.ID, "admit_card.released", "student", studentID, map[string]any{"term_id": termID})
	})
}

// Unrelease holds a released student's card again.
func (s *AdmitService) Unrelease(ctx context.Context, a AdmitActor, termID, studentID string) error {
	if a.Role != "admin" && a.Role != "accountant" {
		return ErrForbidden
	}
	if !uuid.MatchString(termID) || !uuid.MatchString(studentID) {
		return ErrInvalid
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Exec("DELETE FROM admit_card_releases WHERE term_id = ? AND student_id = ?", termID, studentID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.Record(tx, &a.ID, "admit_card.release_removed", "student", studentID, map[string]any{"term_id": termID})
	})
}

// ---------------------------------------------------------------------------
// Cards

// Print renders the cards of a section (or of a whole class, for the
// administrator and the head teacher), leaving out held students.
func (s *AdmitService) Print(ctx context.Context, a AdmitActor, termID string, in AdmitPrint) ([]byte, int, error) {
	if !s.Available() {
		return nil, 0, ErrDocuments
	}
	if in.Language == "" {
		in.Language = "en"
	}
	if (in.Language != "en" && in.Language != "bn") || (in.SectionID == "" && in.ClassID == "") {
		return nil, 0, ErrInvalid
	}
	if !a.all() {
		if a.Role != "teacher" || in.SectionID == "" {
			return nil, 0, ErrForbidden
		}
	}
	students, err := s.Students(ctx, a, termID, in.ClassID, in.SectionID, false)
	if err != nil {
		return nil, 0, err
	}
	if len(students) == 0 && !a.all() {
		// Not their section (or nobody in it).
		var mine int64
		filter, args := sectionFilter(a)
		if err := s.db.WithContext(ctx).Raw("SELECT count(*) FROM academic_sections sec WHERE sec.id = ?"+filter, append([]any{in.SectionID}, args...)...).Scan(&mine).Error; err != nil {
			return nil, 0, err
		}
		if mine == 0 {
			return nil, 0, ErrForbidden
		}
	}
	ids := []string{}
	for _, st := range students {
		if !st.Held {
			ids = append(ids, st.StudentID)
		}
	}
	if len(ids) == 0 {
		return nil, 0, ErrNoCards
	}
	doc, err := s.cards(ctx, termID, ids, in.Language)
	if err != nil {
		return nil, 0, err
	}
	pdf, err := s.Documents.RenderAdmit(ctx, doc)
	if err != nil {
		return nil, 0, err
	}
	_ = audit.Record(s.db.WithContext(ctx), &a.ID, "admit_card.printed", "exam_term", termID, map[string]any{"cards": len(ids), "held": len(students) - len(ids), "class_id": in.ClassID, "section_id": in.SectionID, "language": in.Language})
	return pdf, len(students) - len(ids), nil
}

// ownStudent is the signed-in student's record.
func (s *AdmitService) ownStudent(tx *gorm.DB, actor string) (string, string, error) {
	var st struct{ ID, SessionID string }
	if err := tx.Raw("SELECT id, session_id FROM students WHERE user_id = ? AND status = 'active' LIMIT 1", actor).Scan(&st).Error; err != nil {
		return "", "", err
	}
	if st.ID == "" {
		return "", "", ErrNotFound
	}
	return st.ID, st.SessionID, nil
}

// Mine lists the signed-in student's cards: one per term of their session
// with a published routine that has papers for their class.
func (s *AdmitService) Mine(ctx context.Context, actor string) ([]MyAdmitCard, error) {
	db := s.db.WithContext(ctx)
	if !uuid.MatchString(actor) {
		return nil, ErrInvalid
	}
	id, session, err := s.ownStudent(db, actor)
	if errors.Is(err, ErrNotFound) {
		return []MyAdmitCard{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []MyAdmitCard{}
	err = db.Raw(`SELECT t.id AS term_id, t.name AS term_name, v.id AS version_id, v.version, to_char(v.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS published_at,
 `+held("t.id")+` AS held
 FROM routine_versions v JOIN exam_terms t ON t.id = v.term_id CROSS JOIN admit_card_settings cfg JOIN students st ON st.id = ?
 WHERE v.status = 'published' AND t.session_id = ? AND EXISTS (SELECT 1 FROM routine_papers p WHERE p.version_id = v.id AND p.class_id = st.class_id)
 ORDER BY t.sequence, t.name`, id, session).Scan(&out).Error
	return out, err
}

// MinePDF is the signed-in student's own card of a term.
func (s *AdmitService) MinePDF(ctx context.Context, actor, termID, language string) ([]byte, error) {
	if !s.Available() {
		return nil, ErrDocuments
	}
	if language == "" {
		language = "en"
	}
	if !uuid.MatchString(actor) || !uuid.MatchString(termID) || (language != "en" && language != "bn") {
		return nil, ErrInvalid
	}
	cards, err := s.Mine(ctx, actor)
	if err != nil {
		return nil, err
	}
	for _, c := range cards {
		if c.TermID != termID {
			continue
		}
		if c.Held {
			return nil, ErrHeld
		}
		id, _, err := s.ownStudent(s.db.WithContext(ctx), actor)
		if err != nil {
			return nil, err
		}
		doc, err := s.cards(ctx, termID, []string{id}, language)
		if err != nil {
			return nil, err
		}
		return s.Documents.RenderAdmit(ctx, doc)
	}
	return nil, ErrNotFound
}

// cards gathers the cards of some students of a term's published routine.
func (s *AdmitService) cards(ctx context.Context, termID string, ids []string, language string) (documents.AdmitDoc, error) {
	db := s.db.WithContext(ctx)
	p, err := s.published(db, termID)
	if err != nil {
		return documents.AdmitDoc{}, err
	}
	settings, err := s.Settings(ctx)
	if err != nil {
		return documents.AdmitDoc{}, err
	}
	doc := documents.AdmitDoc{Language: language, Term: p.TermName, Session: p.SessionName, Version: p.Version, PublishedAt: p.PublishedAt,
		Rules: documents.Name{EN: settings.RulesEN, BN: settings.RulesBN}}
	var people []struct {
		ID, NameEN, NameBN, AdmissionNo, ClassName, SectionName string
		RollNo                                                  *int
		Photo                                                   []byte
	}
	if err := db.Raw(`SELECT st.id, st.name_en, st.name_bn, st.admission_no, st.roll_no, c.name AS class_name, sec.name AS section_name, ph.content AS photo
 FROM students st JOIN academic_classes c ON c.id = st.class_id JOIN academic_sections sec ON sec.id = st.section_id
 LEFT JOIN student_photos ph ON ph.student_id = st.id
 WHERE st.id IN ? ORDER BY c.display_order, c.name, sec.display_order, sec.name, st.roll_no NULLS LAST, st.admission_no`, ids).Scan(&people).Error; err != nil {
		return doc, err
	}
	var papers []struct {
		StudentID, ExamDate, Code, NameEN, NameBN, Kind, RoomEN, RoomBN string
		StartMinute, EndMinute, PaperNo, Seat                           int
		Optional, Many                                                  bool
	}
	if err := db.Raw(`WITH seats AS (
  SELECT p.id AS paper_id, st.id AS student_id, pr.room_id,
    row_number() OVER (PARTITION BY p.exam_date, p.sitting_id, pr.room_id ORDER BY c.display_order, c.name, sec.display_order, sec.name, st.roll_no NULLS LAST, st.admission_no) AS seat
  FROM routine_papers p JOIN routine_paper_rooms pr ON pr.paper_id = p.id
  JOIN academic_sections sec ON sec.id = pr.section_id JOIN academic_classes c ON c.id = p.class_id
  JOIN students st ON st.section_id = sec.id AND st.session_id = sec.session_id AND st.status = 'active'
    AND student_takes_subject(st.id, st.session_id, st.class_id, p.subject_id)
  WHERE p.version_id = ? AND p.exam_date IS NOT NULL)
SELECT st.id AS student_id, to_char(p.exam_date, 'YYYY-MM-DD') AS exam_date, si.start_minute, si.end_minute, sub.code, sub.name_en, sub.name_bn,
 p.paper_kind AS kind, p.paper_no, acs.is_fourth_subject AS optional, coalesce(r.name, '') AS room_en, coalesce(r.name_bn, '') AS room_bn, coalesce(seats.seat, 0) AS seat,
 (SELECT count(*) FROM routine_papers q WHERE q.version_id = p.version_id AND q.class_id = p.class_id AND q.subject_id = p.subject_id AND q.paper_kind = p.paper_kind) > 1 AS many
FROM students st
JOIN routine_papers p ON p.version_id = ? AND p.class_id = st.class_id AND p.exam_date IS NOT NULL AND student_takes_subject(st.id, st.session_id, st.class_id, p.subject_id)
JOIN routine_sittings si ON si.id = p.sitting_id JOIN academic_subjects sub ON sub.id = p.subject_id
JOIN academic_class_subjects acs ON acs.class_id = p.class_id AND acs.subject_id = p.subject_id
LEFT JOIN seats ON seats.paper_id = p.id AND seats.student_id = st.id
LEFT JOIN exam_rooms r ON r.id = seats.room_id
WHERE st.id IN ? AND st.session_id = ?
ORDER BY p.exam_date, si.start_minute, sub.code, p.paper_no`, p.VersionID, p.VersionID, ids, p.SessionID).Scan(&papers).Error; err != nil {
		return doc, err
	}
	byStudent := map[string][]documents.AdmitPaper{}
	for _, x := range papers {
		byStudent[x.StudentID] = append(byStudent[x.StudentID], documents.AdmitPaper{Date: x.ExamDate, Start: x.StartMinute, End: x.EndMinute, Code: x.Code,
			Subject: documents.Name{EN: x.NameEN, BN: x.NameBN}, Kind: x.Kind, PaperNo: x.PaperNo, Many: x.Many, Optional: x.Optional,
			Room: documents.Name{EN: x.RoomEN, BN: x.RoomBN}, Seat: x.Seat})
	}
	for _, person := range people {
		token, err := s.token(db, termID, person.ID, p.VersionID)
		if err != nil {
			return doc, err
		}
		doc.Cards = append(doc.Cards, documents.AdmitCard{AdmissionNo: person.AdmissionNo, Name: documents.Name{EN: person.NameEN, BN: person.NameBN},
			Class: person.ClassName, Section: person.SectionName, Roll: person.RollNo, Photo: person.Photo, Token: token, Papers: byStudent[person.ID]})
	}
	return doc, nil
}

// token is the card's verification code for this routine version.
func (s *AdmitService) token(tx *gorm.DB, termID, studentID, versionID string) (string, error) {
	var token string
	if err := tx.Raw("SELECT token FROM admit_card_tokens WHERE routine_version_id = ? AND student_id = ?", versionID, studentID).Scan(&token).Error; err != nil {
		return "", err
	}
	if token != "" {
		return token, nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	if err := tx.Raw(`INSERT INTO admit_card_tokens (token, term_id, student_id, routine_version_id) VALUES (?, ?, ?, ?)
 ON CONFLICT (routine_version_id, student_id) DO UPDATE SET routine_version_id = EXCLUDED.routine_version_id RETURNING token`, token, termID, studentID, versionID).Scan(&token).Error; err != nil {
		return "", err
	}
	return token, nil
}

// Verify is the public check behind a card's QR code.
func (s *AdmitService) Verify(ctx context.Context, token string) (AdmitVerification, error) {
	if !documents.IsToken(token) {
		return AdmitVerification{}, ErrNotFound
	}
	var out AdmitVerification
	err := s.db.WithContext(ctx).Raw(`SELECT CASE WHEN v.status = 'published' THEN 'current' ELSE 'superseded' END AS status,
 sch.name_en AS school_name, sch.name_bn AS school_bn, t.name AS term, ses.name AS session, c.name AS class, sec.name AS section, v.version,
 to_char(v.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS published_at, to_char(k.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS issued_at
 FROM admit_card_tokens k JOIN routine_versions v ON v.id = k.routine_version_id JOIN exam_terms t ON t.id = k.term_id
 JOIN academic_sessions ses ON ses.id = t.session_id JOIN students st ON st.id = k.student_id
 JOIN academic_classes c ON c.id = st.class_id JOIN academic_sections sec ON sec.id = st.section_id CROSS JOIN school_settings sch
 WHERE k.token = ? AND sch.id = 1 AND v.status IN ('published', 'superseded')`, token).Scan(&out).Error
	if err == nil && out.Status == "" {
		err = ErrNotFound
	}
	return out, err
}
