package exams

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// Mark statuses (exam_marks.status, migration 1501).
const (
	StatusPresent    = "present"
	StatusAbsent     = "absent"
	StatusExempt     = "exempt"
	StatusIncomplete = "incomplete"
	StatusWithheld   = "withheld"
)

// Sheet statuses (exam_subjects.sheet_status): one exam subject is one sheet.
const (
	SheetNotStarted = "not_started"
	SheetInProgress = "in_progress"
	SheetSubmitted  = "submitted"
	SheetReturned   = "returned"
	SheetApproved   = "approved"
)

// Specific conflicts. Each still satisfies errors.Is(err, ErrConflict).
var (
	ErrExamNotDraft      = fmt.Errorf("%w: the exam is not a draft", ErrConflict)
	ErrSheetState        = fmt.Errorf("%w: the sheet does not allow this now", ErrConflict)
	ErrSheetIncomplete   = fmt.Errorf("%w: a student has no mark or status", ErrConflict)
	ErrSheetsPending     = fmt.Errorf("%w: sheets are awaiting review or were returned", ErrConflict)
	ErrMarksIncomplete   = fmt.Errorf("%w: marks are missing or incomplete", ErrConflict)
	ErrAllSubjectsExempt = fmt.Errorf("%w: every required subject is exempt", ErrInvalid)
)

// MarkEntry is one cell written through PATCH /exams/:id/marks.
type MarkEntry struct {
	ExamSubjectID string   `json:"exam_subject_id"`
	StudentID     string   `json:"student_id"`
	Status        string   `json:"status"`
	Marks         *float64 `json:"marks"`
	Reason        string   `json:"reason"`
	// RowVersion is the version the client last saw; 0 when the cell was empty.
	RowVersion int `json:"row_version"`
	// Components of a component sheet (CQ, MCQ …); the marks are their sum.
	Components map[string]float64 `json:"components,omitempty"`
}

type MarkEntriesInput struct {
	Entries []MarkEntry `json:"entries"`
}

// Cell is a saved (or current) cell.
type Cell struct {
	ExamSubjectID string             `json:"exam_subject_id"`
	StudentID     string             `json:"student_id"`
	Status        string             `json:"status"`
	Marks         *float64           `json:"marks"`
	Reason        string             `json:"reason"`
	RowVersion    int                `json:"row_version"`
	Components    map[string]float64 `json:"components,omitempty"`
}

// CellError explains why one cell was not saved; the others are unaffected.
type CellError struct {
	ExamSubjectID string `json:"exam_subject_id"`
	StudentID     string `json:"student_id"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	Current       *Cell  `json:"current,omitempty"` // for version conflicts
}

type MarksResult struct {
	Saved  []Cell      `json:"saved"`
	Errors []CellError `json:"errors"`
}

// Sheet is the workflow view of one exam subject.
type Sheet struct {
	ID           string  `json:"id"`
	SubjectID    string  `json:"subject_id"`
	NameEN       string  `json:"name_en"`
	NameBN       string  `json:"name_bn"`
	FullMarks    float64 `json:"full_marks"`
	PassMarks    float64 `json:"pass_marks"`
	IsFourth     bool    `json:"is_fourth_subject"`
	Status       string  `json:"status"`
	Students     int     `json:"students"`
	Entered      int     `json:"entered"`
	Missing      int     `json:"missing"`
	Absent       int     `json:"absent"`
	Exempt       int     `json:"exempt"`
	Incomplete   int     `json:"incomplete"`
	Withheld     int     `json:"withheld"`
	SubmittedBy  string  `json:"submitted_by"`
	SubmittedAt  *string `json:"submitted_at"`
	ReviewedBy   string  `json:"reviewed_by"`
	ReviewedAt   *string `json:"reviewed_at"`
	ReturnReason string  `json:"return_reason"`
}

// Blocker is one reason an exam cannot be published yet.
type Blocker struct {
	Code    string `json:"code"`
	SheetID string `json:"sheet_id,omitempty"`
	Subject string `json:"subject,omitempty"`
	Count   int    `json:"count,omitempty"`
}

type Readiness struct {
	Ready    bool      `json:"ready"`
	Blockers []Blocker `json:"blockers"`
}

type SheetsView struct {
	ExamID     string    `json:"exam_id"`
	ExamStatus string    `json:"exam_status"`
	Sheets     []Sheet   `json:"sheets"`
	Readiness  Readiness `json:"readiness"`
}

// Sheet actions.
const (
	ActionSubmit  = "submit"
	ActionApprove = "approve"
	ActionReturn  = "return"
)

// ---------------------------------------------------------------------------
// Scoring with statuses (used by publication and exam-verify)

// MarkLine is one subject of a student's result as stored.
type MarkLine struct {
	Marks     *float64
	FullMarks float64
	PassMarks float64
	Fourth    bool
	Status    string
	// Combined: papers graded together (Bangla 1st + 2nd) share a key (E5).
	Combined string
	// Components of a component sheet; one below its pass mark fails the
	// subject whatever its total (E5).
	Components []ComponentLine
}

// ComponentLine is one component of a present mark.
type ComponentLine struct {
	Key   string
	Marks float64
	Pass  *float64
}

// ScorePolicy holds a blueprint version's choices. The zero value is
// today's behaviour (GPA half up, absent scores 0, combined pass rule).
type ScorePolicy struct {
	GPA       calc.Rounding
	Absent    calc.AbsentPolicy
	EachPaper bool // combined papers must each reach their pass mark
}

// LineGrade is the grade stored for one subject; nil for an exempt subject.
type LineGrade struct {
	Grade  *string
	Point  *calc.Points
	Passed *bool
}

// StudentScore is a student's exam result with mark statuses applied.
type StudentScore struct {
	Withheld bool
	Result   calc.Result // totals and full marks exclude exempt subjects
	Lines    []LineGrade
}

// ScoreLines applies the status rules of the calculation rules document with
// today's policy: absent scores 0 and fails the subject; exempt leaves the
// result (and the GPA denominator); incomplete blocks publication; withheld
// holds the whole result back. A result with only present marks equals Score.
func ScoreLines(lines []MarkLine, bands []Band) (StudentScore, error) {
	return ScoreLinesWith(lines, bands, ScorePolicy{})
}

// paperLine is one line given to the engine and the stored lines it grades.
type paperLine struct {
	line   calc.Line
	owners []int
}

// ScoreLinesWith scores a student's lines under a blueprint version's
// policy. A component below its pass mark fails its subject; papers that
// share a combined key are graded as one subject, and every paper stores the
// combined grade.
func ScoreLinesWith(lines []MarkLine, bands []Band, policy ScorePolicy) (StudentScore, error) {
	scheme, err := exactScheme(bands)
	if err != nil {
		return StudentScore{}, err
	}
	in := []paperLine{}
	groups := map[string]int{}
	for i, l := range lines {
		status, err := calcStatus(l.Status)
		if err != nil {
			return StudentScore{}, err
		}
		f, err1 := calc.MarksOf(l.FullMarks)
		p, err2 := calc.MarksOf(l.PassMarks)
		if err1 != nil || err2 != nil || f <= 0 || p > f {
			return StudentScore{}, ErrInvalid
		}
		line := calc.Line{Full: f, Pass: p, Optional: l.Fourth, Status: status}
		if status == calc.Present {
			if l.Marks == nil {
				return StudentScore{}, ErrMarksIncomplete
			}
			m, err := calc.MarksOf(*l.Marks)
			if err != nil || m > f {
				return StudentScore{}, ErrInvalid
			}
			line.Marks = m
			for _, c := range l.Components {
				if c.Pass == nil {
					continue
				}
				got, err1 := calc.MarksOf(c.Marks)
				pass, err2 := calc.MarksOf(*c.Pass)
				if err1 != nil || err2 != nil {
					return StudentScore{}, ErrInvalid
				}
				if got < pass {
					line.Failed = true
				}
			}
		}
		if l.Combined == "" {
			in = append(in, paperLine{line: line, owners: []int{i}})
			continue
		}
		g, ok := groups[l.Combined]
		if !ok {
			groups[l.Combined] = len(in)
			in = append(in, paperLine{line: calc.Line{Optional: l.Fourth, Status: calc.Exempt}, owners: []int{i}})
			g = len(in) - 1
		} else {
			in[g].owners = append(in[g].owners, i)
		}
		in[g].line = addPaper(in[g].line, line, policy)
	}
	engine := make([]calc.Line, len(in))
	for i, pl := range in {
		engine[i] = pl.line
	}
	r, err := calc.CalculateWithStatuses(engine, scheme, calc.Policy{GPA: policy.GPA}, policy.Absent)
	if err != nil {
		if errors.Is(err, calc.ErrInvalid) && allRequiredExempt(lines) {
			return StudentScore{}, ErrAllSubjectsExempt
		}
		return StudentScore{}, ErrInvalid
	}
	switch r.Outcome {
	case calc.OutcomeWithheld:
		return StudentScore{Withheld: true, Lines: make([]LineGrade, len(lines))}, nil
	case calc.OutcomeIncomplete:
		return StudentScore{}, ErrMarksIncomplete
	}
	out := StudentScore{Result: r.Result, Lines: make([]LineGrade, len(lines))}
	k := 0
	for _, pl := range in {
		if pl.line.Status == calc.Exempt {
			continue
		}
		g := r.Result.Grades[k]
		k++
		for _, i := range pl.owners {
			if lines[i].Status == StatusExempt {
				continue // an exempt paper keeps no grade
			}
			name, point, passed := g.Grade, g.Point, g.Passed
			out.Lines[i] = LineGrade{Grade: &name, Point: &point, Passed: &passed}
		}
	}
	return out, nil
}

// addPaper folds one paper into a combined subject: blocked papers block it,
// exempt papers leave it, an absent paper scores 0 and fails it (or makes it
// incomplete when absence means missing), and present papers add their marks.
func addPaper(group, paper calc.Line, policy ScorePolicy) calc.Line {
	rank := map[calc.Status]int{calc.Exempt: 0, calc.Present: 1, calc.Incomplete: 2, calc.Withheld: 3}
	switch paper.Status {
	case calc.Exempt:
		return group
	case calc.Withheld, calc.Incomplete:
		if rank[paper.Status] > rank[group.Status] {
			group.Status = paper.Status
		}
		return group
	case calc.Absent:
		if policy.Absent == calc.AbsentIsMissing {
			if rank[calc.Incomplete] > rank[group.Status] {
				group.Status = calc.Incomplete
			}
			return group
		}
		paper.Marks, paper.Failed = 0, true
	}
	if group.Status == calc.Exempt {
		group.Status = calc.Present
	}
	group.Marks += paper.Marks
	group.Full += paper.Full
	group.Pass += paper.Pass
	if paper.Failed || (policy.EachPaper && paper.Marks < paper.Pass) {
		group.Failed = true
	}
	return group
}

func allRequiredExempt(lines []MarkLine) bool {
	for _, l := range lines {
		if !l.Fourth && l.Status != StatusExempt {
			return false
		}
	}
	return true
}

func calcStatus(s string) (calc.Status, error) {
	switch s {
	case StatusPresent, "":
		return calc.Present, nil
	case StatusAbsent:
		return calc.Absent, nil
	case StatusExempt:
		return calc.Exempt, nil
	case StatusIncomplete:
		return calc.Incomplete, nil
	case StatusWithheld:
		return calc.Withheld, nil
	}
	return 0, ErrInvalid
}

// ---------------------------------------------------------------------------
// Service

func validEntries(entries []MarkEntry) bool {
	if len(entries) == 0 || len(entries) > 500 {
		return false
	}
	seen := map[string]bool{}
	for _, e := range entries {
		key := e.ExamSubjectID + ":" + e.StudentID
		if !uuid.MatchString(e.ExamSubjectID) || !uuid.MatchString(e.StudentID) || seen[key] || e.RowVersion < 0 || len(e.Reason) > 1000 || !utf8.ValidString(e.Reason) || len(e.Components) > 20 {
			return false
		}
		seen[key] = true
	}
	return true
}

// SaveMarkEntries saves cells independently: an invalid or stale cell is
// reported and the others are saved. role is the actor's role.
func (s *Service) SaveMarkEntries(ctx context.Context, actor, role, examID string, entries []MarkEntry) (MarksResult, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || !validEntries(entries) {
		return MarksResult{}, ErrInvalid
	}
	return s.repo.SaveMarkEntries(ctx, actor, role, examID, entries, false)
}

func (s *Service) Sheets(ctx context.Context, examID, actor, scope string) (SheetsView, error) {
	if !uuid.MatchString(examID) || !uuid.MatchString(actor) {
		return SheetsView{}, ErrInvalid
	}
	return s.repo.Sheets(ctx, examID, actor, scope)
}

func (s *Service) SheetAction(ctx context.Context, actor, role, examID, sheetID, action, reason string) (Sheet, error) {
	reason = strings.TrimSpace(reason)
	if !uuid.MatchString(actor) || !uuid.MatchString(examID) || !uuid.MatchString(sheetID) || utf8.RuneCountInString(reason) > 240 || !utf8.ValidString(reason) {
		return Sheet{}, ErrInvalid
	}
	switch action {
	case ActionSubmit, ActionApprove:
	case ActionReturn:
		if reason == "" {
			return Sheet{}, ErrInvalid
		}
	default:
		return Sheet{}, ErrInvalid
	}
	return s.repo.SheetAction(ctx, actor, role, examID, sheetID, action, reason)
}

// ---------------------------------------------------------------------------
// Repository

type examLock struct {
	ID, Status, SessionID, ClassID, SectionID string
}

// lockExam locks the exam row: FOR SHARE lets mark writes on different sheets
// run in parallel while blocking a state change; FOR UPDATE is for state and
// structure changes.
func lockExam(tx *gorm.DB, id, mode string) (examLock, error) {
	var e examLock
	if err := tx.Raw("SELECT id,status,session_id,class_id,section_id FROM exams WHERE id=? FOR "+mode, id).Scan(&e).Error; err != nil {
		return e, err
	}
	if e.ID == "" {
		return e, ErrNotFound
	}
	return e, nil
}

func writableSheet(status string) bool {
	return status == SheetNotStarted || status == SheetInProgress || status == SheetReturned
}

type cellRow struct {
	ID            string
	ExamSubjectID string
	StudentID     string
	Status        string
	Marks         *float64
	StatusReason  string
	RowVersion    int
}

// SaveMarkEntries writes cells in one transaction with set-based queries and
// per-sheet row locks. legacy keeps PUT /exams/:id/marks semantics: present
// marks only, no version check, all or nothing.
func (r *GormRepository) SaveMarkEntries(ctx context.Context, actor, role, examID string, entries []MarkEntry, legacy bool) (MarksResult, error) {
	out := MarksResult{Saved: []Cell{}, Errors: []CellError{}}
	if role != "admin" && role != "teacher" {
		return out, ErrForbidden
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exam, err := lockExam(tx, examID, "SHARE")
		if err != nil {
			return err
		}
		// A published exam's marks change only through a correction (E6): an
		// administrator, for a student with an open correction.
		correcting := map[string]string{}
		if exam.Status != "draft" {
			if legacy || role != "admin" {
				return ErrExamNotDraft
			}
			if correcting, err = openCorrectionStudents(tx, examID); err != nil {
				return err
			}
			if len(correcting) == 0 {
				return ErrExamNotDraft
			}
		}
		sheetIDs, studentIDs := []string{}, []string{}
		seenSheet, seenStudent := map[string]bool{}, map[string]bool{}
		for _, e := range entries {
			if !seenSheet[e.ExamSubjectID] {
				seenSheet[e.ExamSubjectID] = true
				sheetIDs = append(sheetIDs, e.ExamSubjectID)
			}
			if !seenStudent[e.StudentID] {
				seenStudent[e.StudentID] = true
				studentIDs = append(studentIDs, e.StudentID)
			}
		}
		sort.Strings(sheetIDs)
		sheets := []struct {
			ID, SubjectID, SheetStatus string
			FullMarks                  float64
		}{}
		if err := tx.Raw("SELECT id,subject_id,sheet_status,full_marks FROM exam_subjects WHERE exam_id=? AND id IN ? ORDER BY id FOR UPDATE", examID, sheetIDs).Scan(&sheets).Error; err != nil {
			return err
		}
		sheetByID := map[string]int{}
		for i, s := range sheets {
			sheetByID[s.ID] = i
		}
		assigned := map[string]bool{}
		if role == "teacher" {
			subjects := []struct{ SubjectID string }{}
			if err := tx.Raw("SELECT ta.subject_id FROM teacher_assignments ta JOIN employees emp ON emp.id=ta.employee_id WHERE emp.user_id=? AND emp.is_active AND ta.session_id=? AND ta.class_id=?", actor, exam.SessionID, exam.ClassID).Scan(&subjects).Error; err != nil {
				return err
			}
			for _, s := range subjects {
				assigned[s.SubjectID] = true
			}
		}
		active := map[string]bool{}
		ids := []struct{ ID string }{}
		if err := tx.Raw("SELECT id FROM students WHERE id IN ? AND session_id=? AND class_id=? AND section_id=? AND status='active'", studentIDs, exam.SessionID, exam.ClassID, exam.SectionID).Scan(&ids).Error; err != nil {
			return err
		}
		for _, s := range ids {
			active[s.ID] = true
		}
		// Optional subjects a student did not choose are not theirs (E5).
		notTaken := map[string]bool{}
		pairs := []struct{ StudentID, SubjectID string }{}
		if err := tx.Raw(`SELECT st.id AS student_id, es.subject_id FROM students st CROSS JOIN exam_subjects es
 WHERE st.id IN ? AND es.id IN ? AND NOT student_takes_subject(st.id, ?, ?, es.subject_id)`, studentIDs, sheetIDs, exam.SessionID, exam.ClassID).Scan(&pairs).Error; err != nil {
			return err
		}
		for _, pr := range pairs {
			notTaken[pr.StudentID+"|"+pr.SubjectID] = true
		}
		// Component sheets: their components and full marks (E5).
		sheetComps := map[string][]sheetComponent{}
		comps := []sheetComponent{}
		if err := tx.Raw("SELECT exam_subject_id, component_key AS key, full_marks::float8 AS full FROM exam_subject_components WHERE exam_subject_id IN ? ORDER BY display_order", sheetIDs).Scan(&comps).Error; err != nil {
			return err
		}
		for _, c := range comps {
			sheetComps[c.ExamSubjectID] = append(sheetComps[c.ExamSubjectID], c)
		}
		current := map[string]cellRow{}
		rows := []cellRow{}
		if err := tx.Raw("SELECT id,exam_subject_id,student_id,status,marks,status_reason,row_version FROM exam_marks WHERE exam_id=? AND exam_subject_id IN ? AND student_id IN ? ORDER BY id FOR UPDATE", examID, sheetIDs, studentIDs).Scan(&rows).Error; err != nil {
			return err
		}
		for _, c := range rows {
			current[c.ExamSubjectID+":"+c.StudentID] = c
		}
		oldComps, err := markComponents(tx, rows)
		if err != nil {
			return err
		}

		type accepted struct {
			entry  MarkEntry
			marks  any // two-decimal string or nil
			reason string
			old    *cellRow
			comps  map[string]string // component key → two-decimal marks
		}
		accept := []accepted{}
		fail := func(e MarkEntry, code, message string, cur *Cell) {
			out.Errors = append(out.Errors, CellError{ExamSubjectID: e.ExamSubjectID, StudentID: e.StudentID, Code: code, Message: message, Current: cur})
		}
		touched := map[string]bool{}
		for _, e := range entries {
			if legacy {
				e.Status, e.Reason = StatusPresent, ""
			}
			i, ok := sheetByID[e.ExamSubjectID]
			if !ok {
				fail(e, "unknown_sheet", "this subject is not part of the exam", nil)
				continue
			}
			sheet := sheets[i]
			if role == "teacher" && !assigned[sheet.SubjectID] {
				fail(e, "not_assigned", "you are not assigned to this subject", nil)
				continue
			}
			if exam.Status != "draft" {
				if correcting[e.StudentID] == "" {
					fail(e, "correction_required", "open a correction for this student before changing a published mark", nil)
					continue
				}
			} else if !writableSheet(sheet.SheetStatus) {
				fail(e, "sheet_locked", "the sheet has been submitted or approved; it must be returned before marks change", nil)
				continue
			}
			if !active[e.StudentID] {
				fail(e, "not_in_section", "the student is not an active student of this section", nil)
				continue
			}
			if notTaken[e.StudentID+"|"+sheet.SubjectID] {
				fail(e, "not_enrolled", "the student did not choose this optional subject", nil)
				continue
			}
			newComps, code, msg := entryComponents(e, sheetComps[sheet.ID])
			if code != "" {
				fail(e, code, msg, nil)
				continue
			}
			if newComps != nil {
				sum := calc.Marks(0)
				for _, v := range newComps {
					m, _ := calc.MarksOf(v)
					sum += m
				}
				if e.Marks != nil {
					if given, err := calc.MarksOf(*e.Marks); err != nil || given != sum {
						fail(e, "marks_mismatch", "the marks must equal the sum of the components", nil)
						continue
					}
				}
				total := sum.Float()
				e.Marks = &total
			}
			full, _ := calc.MarksOf(sheet.FullMarks)
			var marks any
			reason := strings.TrimSpace(e.Reason)
			switch e.Status {
			case StatusPresent, StatusWithheld:
				if e.Marks == nil {
					if e.Status == StatusPresent {
						fail(e, "marks_required", "enter marks or choose another status", nil)
						continue
					}
					break
				}
				m, err := calc.MarksOf(*e.Marks)
				if err != nil {
					fail(e, "invalid_marks", "marks must be a number with at most two decimals", nil)
					continue
				}
				if m > full {
					fail(e, "above_full_marks", fmt.Sprintf("marks cannot exceed the full marks (%s)", full), nil)
					continue
				}
				marks = m.String()
			case StatusAbsent, StatusIncomplete, StatusExempt:
				if e.Marks != nil {
					fail(e, "marks_not_allowed", "an absent, exempt or incomplete student has no marks", nil)
					continue
				}
			default:
				fail(e, "invalid_status", "unknown status", nil)
				continue
			}
			if (e.Status == StatusExempt || e.Status == StatusWithheld) && role != "admin" {
				fail(e, "status_not_allowed", "only an administrator can exempt a student or withhold a result", nil)
				continue
			}
			if e.Status == StatusExempt || e.Status == StatusWithheld {
				if reason == "" || utf8.RuneCountInString(reason) > 240 {
					fail(e, "reason_required", "give a reason of up to 240 characters", nil)
					continue
				}
			} else {
				reason = ""
			}
			key := e.ExamSubjectID + ":" + e.StudentID
			old, exists := current[key]
			// An exemption or a withheld result is an administrator's decision;
			// nobody else may change that cell, even back to present.
			if exists && (old.Status == StatusExempt || old.Status == StatusWithheld) && role != "admin" {
				fail(e, "status_not_allowed", "only an administrator can change an exempt or withheld mark", nil)
				continue
			}
			version := 0
			if exists {
				version = old.RowVersion
			}
			if !legacy && e.RowVersion != version {
				var cur *Cell
				if exists {
					cur = &Cell{ExamSubjectID: old.ExamSubjectID, StudentID: old.StudentID, Status: old.Status, Marks: old.Marks, Reason: old.StatusReason, RowVersion: old.RowVersion, Components: floatMap(oldComps[old.ID])}
				}
				fail(e, "version_conflict", "someone else changed this mark; review their value before saving again", cur)
				continue
			}
			compStrings := twoDecimalMap(newComps)
			if exists && old.Status == e.Status && old.StatusReason == reason && sameMarks(old.Marks, marks) && sameComponents(oldComps[old.ID], compStrings) {
				out.Saved = append(out.Saved, Cell{ExamSubjectID: e.ExamSubjectID, StudentID: e.StudentID, Status: old.Status, Marks: old.Marks, Reason: old.StatusReason, RowVersion: old.RowVersion, Components: floatMap(oldComps[old.ID])})
				continue
			}
			a := accepted{entry: e, marks: marks, reason: reason, comps: compStrings}
			if exists {
				a.old = &old
			}
			accept = append(accept, a)
			touched[e.ExamSubjectID] = true
		}
		if legacy && len(out.Errors) > 0 {
			return legacyError(out.Errors[0].Code)
		}
		if len(accept) == 0 {
			return nil
		}
		values := make([]string, 0, len(accept))
		args := make([]any, 0, len(accept)*11)
		for _, a := range accept {
			values = append(values, "(?,?,?,?,?,?,?::numeric,?,?,?,?)")
			args = append(args, examID, a.entry.ExamSubjectID, a.entry.StudentID, exam.SessionID, exam.ClassID, exam.SectionID, a.marks, a.entry.Status, a.reason, actor, actor)
		}
		saved := []cellRow{}
		if err := tx.Raw(`INSERT INTO exam_marks(exam_id,exam_subject_id,student_id,session_id,class_id,section_id,marks,status,status_reason,entered_by,updated_by)
VALUES `+strings.Join(values, ",")+`
ON CONFLICT(exam_subject_id,student_id) DO UPDATE SET marks=EXCLUDED.marks,status=EXCLUDED.status,status_reason=EXCLUDED.status_reason,
 grade=NULL,grade_point=NULL,passed=NULL,updated_by=EXCLUDED.updated_by,updated_at=now(),row_version=exam_marks.row_version+1
RETURNING id,exam_subject_id,student_id,status,marks,status_reason,row_version`, args...).Scan(&saved).Error; err != nil {
			return dbErr(err)
		}
		// Component marks follow their mark: replaced in the same transaction.
		compsOf := map[string]map[string]string{}
		for _, a := range accept {
			compsOf[a.entry.ExamSubjectID+":"+a.entry.StudentID] = a.comps
		}
		for _, c := range saved {
			cs := compsOf[c.ExamSubjectID+":"+c.StudentID]
			if len(sheetComps[c.ExamSubjectID]) > 0 {
				if err := tx.Exec("DELETE FROM exam_component_marks WHERE exam_mark_id = ?", c.ID).Error; err != nil {
					return err
				}
				for key, v := range cs {
					if err := tx.Exec("INSERT INTO exam_component_marks (exam_mark_id, exam_subject_id, component_key, marks) VALUES (?, ?, ?, ?::numeric)", c.ID, c.ExamSubjectID, key, v).Error; err != nil {
						return dbErr(err)
					}
				}
			}
			out.Saved = append(out.Saved, Cell{ExamSubjectID: c.ExamSubjectID, StudentID: c.StudentID, Status: c.Status, Marks: c.Marks, Reason: c.StatusReason, RowVersion: c.RowVersion, Components: floatMap(cs)})
		}
		started := []string{}
		for id := range touched {
			started = append(started, id)
		}
		if err := tx.Exec("UPDATE exam_subjects SET sheet_status='in_progress' WHERE id IN ? AND sheet_status='not_started'", started).Error; err != nil {
			return err
		}
		events := make([]audit.Entry, 0, len(accept))
		for _, a := range accept {
			meta := map[string]any{"student_id": a.entry.StudentID, "exam_subject_id": a.entry.ExamSubjectID, "new_marks": a.marks, "new_status": a.entry.Status, "old_marks": nil, "old_status": nil}
			if a.old != nil {
				meta["old_marks"], meta["old_status"] = a.old.Marks, a.old.Status
			}
			if a.reason != "" {
				meta["reason"] = a.reason
			}
			if id := correcting[a.entry.StudentID]; id != "" {
				meta["correction_id"] = id
			}
			if len(a.comps) > 0 {
				meta["new_components"] = a.comps
			}
			events = append(events, audit.Entry{Action: "exam.mark_saved", TargetType: "exam", TargetID: examID, Metadata: meta})
		}
		return audit.RecordMany(tx, &actor, events)
	})
	if err != nil {
		return MarksResult{}, dbErr(err)
	}
	return out, nil
}

// sheetComponent is one component of a sheet.
type sheetComponent struct {
	ExamSubjectID string
	Key           string
	Full          float64
}

// entryComponents checks an entry's components against its sheet: a
// component sheet needs every component (and nothing else) for marks; other
// statuses and ordinary sheets take none. It returns the components, or an
// error code and message.
func entryComponents(e MarkEntry, comps []sheetComponent) (map[string]float64, string, string) {
	if len(comps) == 0 {
		if len(e.Components) > 0 {
			return nil, "components_not_allowed", "this sheet has no components"
		}
		return nil, "", ""
	}
	if e.Status != StatusPresent && !(e.Status == StatusWithheld && len(e.Components) > 0) {
		if len(e.Components) > 0 {
			return nil, "components_not_allowed", "only marks that are present have components"
		}
		return nil, "", ""
	}
	if len(e.Components) != len(comps) {
		return nil, "components_required", "enter every component's marks"
	}
	out := map[string]float64{}
	for _, c := range comps {
		v, ok := e.Components[c.Key]
		if !ok {
			return nil, "components_required", "enter every component's marks"
		}
		m, err := calc.MarksOf(v)
		if err != nil {
			return nil, "invalid_marks", "marks must be a number with at most two decimals"
		}
		full, _ := calc.MarksOf(c.Full)
		if m > full {
			return nil, "above_full_marks", fmt.Sprintf("%s cannot exceed its full marks (%s)", c.Key, full)
		}
		out[c.Key] = v
	}
	return out, "", ""
}

// markComponents loads the component marks of stored marks as two-decimal
// strings, by mark id.
func markComponents(tx *gorm.DB, rows []cellRow) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if len(rows) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	comps := []struct {
		MarkID, Key string
		Marks       float64
	}{}
	if err := tx.Raw("SELECT exam_mark_id AS mark_id, component_key AS key, marks::float8 AS marks FROM exam_component_marks WHERE exam_mark_id IN ?", ids).Scan(&comps).Error; err != nil {
		return nil, err
	}
	for _, c := range comps {
		if out[c.MarkID] == nil {
			out[c.MarkID] = map[string]string{}
		}
		m, _ := calc.MarksOf(c.Marks)
		out[c.MarkID][c.Key] = m.String()
	}
	return out, nil
}

func twoDecimalMap(in map[string]float64) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		m, _ := calc.MarksOf(v)
		out[k] = m.String()
	}
	return out
}

func floatMap(in map[string]string) map[string]float64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		r, ok := new(big.Rat).SetString(v)
		if !ok {
			continue
		}
		f, _ := r.Float64()
		out[k] = f
	}
	return out
}

func sameComponents(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sameMarks(old *float64, next any) bool {
	if old == nil || next == nil {
		return old == nil && next == nil
	}
	m, err := calc.MarksOf(*old)
	return err == nil && m.String() == next
}

// legacyError maps a cell error to the whole-request errors of PUT
// /exams/:id/marks before E3.
func legacyError(code string) error {
	switch code {
	case "not_assigned":
		return ErrForbidden
	case "sheet_locked":
		return ErrSheetState
	default:
		return ErrInvalid
	}
}

type sheetRow struct {
	ID, SubjectID, NameEN, NameBN, SheetStatus, ReturnReason string
	FullMarks, PassMarks                                     float64
	IsFourth                                                 bool
	Students                                                 int
	Entered, Absent, Exempt, Incomplete, Withheld            int
	SubmittedBy, ReviewedBy                                  string
	SubmittedAt, ReviewedAt                                  *string
}

func (r *GormRepository) Sheets(ctx context.Context, examID, actor, scope string) (SheetsView, error) {
	return sheetsIn(r.db.WithContext(ctx), examID, actor, scope)
}

func sheetsIn(tx *gorm.DB, examID, actor, scope string) (SheetsView, error) {
	e, err := examByID(tx, examID)
	if err != nil {
		return SheetsView{}, err
	}
	view := SheetsView{ExamID: e.ID, ExamStatus: e.Status, Sheets: []Sheet{}, Readiness: Readiness{Blockers: []Blocker{}}}
	var students int64
	if err := tx.Table("students").Where("session_id=? AND class_id=? AND section_id=? AND status='active'", e.SessionID, e.ClassID, e.SectionID).Count(&students).Error; err != nil {
		return view, err
	}
	q := tx.Table("exam_subjects es").
		Joins("JOIN academic_subjects sub ON sub.id=es.subject_id").
		Joins("JOIN academic_class_subjects acs ON acs.class_id=es.class_id AND acs.subject_id=es.subject_id").
		Joins("LEFT JOIN users su ON su.id=es.submitted_by").Joins("LEFT JOIN users ru ON ru.id=es.reviewed_by").
		Joins(`LEFT JOIN LATERAL (SELECT count(*) AS entered, count(*) FILTER (WHERE m.status='absent') AS absent, count(*) FILTER (WHERE m.status='exempt') AS exempt,
 count(*) FILTER (WHERE m.status='incomplete') AS incomplete, count(*) FILTER (WHERE m.status='withheld') AS withheld
 FROM exam_marks m JOIN students st ON st.id=m.student_id AND st.status='active' AND student_takes_subject(st.id, st.session_id, st.class_id, es.subject_id)
 WHERE m.exam_subject_id=es.id) c ON true`).
		// Students who take the subject: optional subjects count only those who chose them (E5).
		Joins(`LEFT JOIN LATERAL (SELECT count(*) AS students FROM students st WHERE st.session_id=? AND st.class_id=? AND st.section_id=? AND st.status='active'
 AND student_takes_subject(st.id, st.session_id, st.class_id, es.subject_id)) n ON true`, e.SessionID, e.ClassID, e.SectionID).
		Where("es.exam_id=?", e.ID)
	if scope == "teacher" {
		q = q.Where("EXISTS (SELECT 1 FROM employees emp JOIN teacher_assignments ta ON ta.employee_id=emp.id WHERE emp.user_id=? AND emp.is_active AND ta.session_id=? AND ta.class_id=? AND ta.subject_id=es.subject_id)", actor, e.SessionID, e.ClassID)
	}
	rows := []sheetRow{}
	if err := q.Select(`es.id,es.subject_id,sub.name_en,sub.name_bn,es.sheet_status,es.return_reason,es.full_marks,es.pass_marks,acs.is_fourth_subject AS is_fourth,
 coalesce(n.students,0) AS students,coalesce(c.entered,0) AS entered,coalesce(c.absent,0) AS absent,coalesce(c.exempt,0) AS exempt,coalesce(c.incomplete,0) AS incomplete,coalesce(c.withheld,0) AS withheld,
 coalesce(nullif(su.display_name,''),su.email,'') AS submitted_by,coalesce(nullif(ru.display_name,''),ru.email,'') AS reviewed_by,
 to_char(es.submitted_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS submitted_at,to_char(es.reviewed_at AT TIME ZONE 'Asia/Dhaka','YYYY-MM-DD HH24:MI') AS reviewed_at`).
		Order("sub.code").Scan(&rows).Error; err != nil {
		return view, err
	}
	if scope == "teacher" && len(rows) == 0 {
		return view, ErrForbidden
	}
	for _, s := range rows {
		missing := s.Students - s.Entered
		if missing < 0 {
			missing = 0
		}
		view.Sheets = append(view.Sheets, Sheet{ID: s.ID, SubjectID: s.SubjectID, NameEN: s.NameEN, NameBN: s.NameBN, FullMarks: s.FullMarks, PassMarks: s.PassMarks, IsFourth: s.IsFourth,
			Status: s.SheetStatus, Students: s.Students, Entered: s.Entered, Missing: missing, Absent: s.Absent, Exempt: s.Exempt, Incomplete: s.Incomplete, Withheld: s.Withheld,
			SubmittedBy: s.SubmittedBy, SubmittedAt: s.SubmittedAt, ReviewedBy: s.ReviewedBy, ReviewedAt: s.ReviewedAt, ReturnReason: s.ReturnReason})
	}
	view.Readiness = readiness(e, view.Sheets, int(students))
	return view, nil
}

// readiness lists what stops publication: missing or incomplete marks and
// sheets awaiting review or returned to the teacher.
func readiness(e Exam, sheets []Sheet, students int) Readiness {
	r := Readiness{Blockers: []Blocker{}}
	if e.Status != "draft" {
		return r
	}
	if len(sheets) == 0 {
		r.Blockers = append(r.Blockers, Blocker{Code: "no_subjects"})
	}
	if students == 0 {
		r.Blockers = append(r.Blockers, Blocker{Code: "no_students"})
	}
	for _, s := range sheets {
		if s.Missing > 0 {
			r.Blockers = append(r.Blockers, Blocker{Code: "marks_missing", SheetID: s.ID, Subject: s.NameEN, Count: s.Missing})
		}
		if s.Incomplete > 0 {
			r.Blockers = append(r.Blockers, Blocker{Code: "marks_incomplete", SheetID: s.ID, Subject: s.NameEN, Count: s.Incomplete})
		}
		switch s.Status {
		case SheetSubmitted:
			r.Blockers = append(r.Blockers, Blocker{Code: "awaiting_review", SheetID: s.ID, Subject: s.NameEN})
		case SheetReturned:
			r.Blockers = append(r.Blockers, Blocker{Code: "returned", SheetID: s.ID, Subject: s.NameEN})
		}
	}
	r.Ready = len(r.Blockers) == 0
	return r
}

func (r *GormRepository) SheetAction(ctx context.Context, actor, role, examID, sheetID, action, reason string) (Sheet, error) {
	var out Sheet
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		exam, err := lockExam(tx, examID, "SHARE")
		if err != nil {
			return err
		}
		if exam.Status != "draft" {
			return ErrExamNotDraft
		}
		var sheet struct{ ID, SubjectID, SheetStatus string }
		if err := tx.Raw("SELECT id,subject_id,sheet_status FROM exam_subjects WHERE id=? AND exam_id=? FOR UPDATE", sheetID, examID).Scan(&sheet).Error; err != nil {
			return err
		}
		if sheet.ID == "" {
			return ErrNotFound
		}
		missing := func() (int64, error) {
			var n int64
			err := tx.Raw("SELECT count(*) FROM students st WHERE st.session_id=? AND st.class_id=? AND st.section_id=? AND st.status='active' AND student_takes_subject(st.id, st.session_id, st.class_id, ?) AND NOT EXISTS (SELECT 1 FROM exam_marks m WHERE m.exam_subject_id=? AND m.student_id=st.id)", exam.SessionID, exam.ClassID, exam.SectionID, sheet.SubjectID, sheetID).Scan(&n).Error
			return n, err
		}
		switch action {
		case ActionSubmit:
			switch role {
			case "admin":
			case "teacher":
				var n int64
				if err := tx.Raw("SELECT count(*) FROM teacher_assignments ta JOIN employees emp ON emp.id=ta.employee_id WHERE emp.user_id=? AND emp.is_active AND ta.session_id=? AND ta.class_id=? AND ta.subject_id=?", actor, exam.SessionID, exam.ClassID, sheet.SubjectID).Scan(&n).Error; err != nil {
					return err
				}
				if n == 0 {
					return ErrForbidden
				}
			default:
				return ErrForbidden
			}
			if !writableSheet(sheet.SheetStatus) {
				return ErrSheetState
			}
			if n, err := missing(); err != nil {
				return err
			} else if n > 0 {
				return ErrSheetIncomplete
			}
			err = tx.Exec("UPDATE exam_subjects SET sheet_status='submitted',submitted_by=?,submitted_at=now(),return_reason='' WHERE id=?", actor, sheetID).Error
		case ActionApprove:
			if role != "admin" && role != "head_teacher" {
				return ErrForbidden
			}
			if sheet.SheetStatus != SheetSubmitted {
				return ErrSheetState
			}
			if n, err := missing(); err != nil {
				return err
			} else if n > 0 {
				return ErrSheetIncomplete
			}
			err = tx.Exec("UPDATE exam_subjects SET sheet_status='approved',reviewed_by=?,reviewed_at=now(),return_reason='' WHERE id=?", actor, sheetID).Error
		case ActionReturn:
			if role != "admin" && role != "head_teacher" {
				return ErrForbidden
			}
			if sheet.SheetStatus != SheetSubmitted && sheet.SheetStatus != SheetApproved {
				return ErrSheetState
			}
			err = tx.Exec("UPDATE exam_subjects SET sheet_status='returned',reviewed_by=?,reviewed_at=now(),return_reason=? WHERE id=?", actor, reason, sheetID).Error
		}
		if err != nil {
			return err
		}
		meta := map[string]any{"sheet_id": sheetID, "subject_id": sheet.SubjectID, "from": sheet.SheetStatus}
		if reason != "" {
			meta["reason"] = reason
		}
		event := map[string]string{ActionSubmit: "exam.sheet_submitted", ActionApprove: "exam.sheet_approved", ActionReturn: "exam.sheet_returned"}[action]
		if err := audit.Record(tx, &actor, event, "exam", examID, meta); err != nil {
			return err
		}
		view, err := sheetsIn(tx, examID, actor, "admin")
		if err != nil {
			return err
		}
		for _, s := range view.Sheets {
			if s.ID == sheetID {
				out = s
			}
		}
		return nil
	})
	return out, dbErr(err)
}
