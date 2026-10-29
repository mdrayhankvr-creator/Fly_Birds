package blueprints

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// The exam setup wizard creates a whole term in one step: for every chosen
// class with an active blueprint, one exam per section per assessment of the
// term, and one sheet per subject (with its components). Choices are kept on
// the term while it is being planned, so a half-finished setup is not lost
// and nothing reaches teachers until it is created.

// Setup is the wizard's saved choices.
type Setup struct {
	ClassIDs []string          `json:"class_ids"`
	Due      map[string]string `json:"due"`       // assessment key → marks due date
	Assign   map[string]string `json:"assignees"` // "classID|subjectID" → employee id
}

// SetupClass is one class as the wizard sees it.
type SetupClass struct {
	ClassID     string   `json:"class_id"`
	ClassName   string   `json:"class_name"`
	Selected    bool     `json:"selected"`
	BlueprintID *string  `json:"blueprint_id"`
	VersionID   *string  `json:"version_id"`
	Version     int      `json:"version"`
	HasDraft    bool     `json:"has_draft"`
	State       string   `json:"state"` // ready, needs_blueprint, no_assessments
	Sections    int      `json:"sections"`
	Assessments []string `json:"assessments"` // keys in this term
	Sheets      int      `json:"sheets"`
	Existing    int      `json:"existing"` // exams already created for this term
	Students    int      `json:"students"`
}

// SetupAssessment is one assessment key across the chosen classes.
type SetupAssessment struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Classes int    `json:"classes"`
	Due     string `json:"due"`
}

// SetupGap is a subject of a chosen class that no teacher is assigned to.
type SetupGap struct {
	ClassID     string  `json:"class_id"`
	ClassName   string  `json:"class_name"`
	SubjectID   string  `json:"subject_id"`
	SubjectName string  `json:"subject_name"`
	Sheets      int     `json:"sheets"`
	Assignee    *string `json:"assignee"`
}

// SetupTeacher is a teacher who can be assigned to a gap.
type SetupTeacher struct {
	EmployeeID string `json:"employee_id"`
	Name       string `json:"name"`
}

// SetupConflict is an existing exam whose name the new exam would take.
type SetupConflict struct {
	ClassName   string `json:"class_name"`
	SectionName string `json:"section_name"`
	ExamName    string `json:"exam_name"`
}

// SetupPlan is everything the wizard shows before Create.
type SetupPlan struct {
	Term        TermView          `json:"term"`
	Setup       Setup             `json:"setup"`
	Classes     []SetupClass      `json:"classes"`
	Assessments []SetupAssessment `json:"assessments"`
	Gaps        []SetupGap        `json:"gaps"`
	Teachers    []SetupTeacher    `json:"teachers"`
	Conflicts   []SetupConflict   `json:"conflicts"`
	Sheets      int               `json:"sheets"`    // sheets the term will have
	ToCreate    int               `json:"to_create"` // of which are new
	Exams       int               `json:"exams"`
	NewExams    int               `json:"new_exams"`
}

// SetupResult reports what Create made.
type SetupResult struct {
	Plan          SetupPlan `json:"plan"`
	ExamsCreated  int       `json:"exams_created"`
	SheetsCreated int       `json:"sheets_created"`
	Assigned      int       `json:"assigned"`
}

// ErrSetupBlocked: Create is refused (no class ready, or a name conflict).
var ErrSetupBlocked = errors.New("the term cannot be created yet")

type plannedSheet struct {
	SubjectID  string
	Pass       calc.Marks
	Full       calc.Marks
	Components []Component
}

type plannedExam struct {
	ClassID, SectionID, SectionName, VersionID, Key, Name, Type, Due string
	Sheets                                                           []plannedSheet
	Exists                                                           bool
}

func examName(term, assessment string) string {
	if strings.EqualFold(strings.TrimSpace(term), strings.TrimSpace(assessment)) {
		return term
	}
	name := term + " · " + assessment
	if len([]byte(name)) > 120 {
		name = string([]rune(name)[:60])
	}
	return strings.TrimSpace(name)
}

func examType(kind, termKind string) string {
	switch {
	case kind == "class_test":
		return "class_test"
	case kind == "term_exam" && termKind == "annual":
		return "annual"
	default:
		return "other"
	}
}

func readSetup(raw json.RawMessage) Setup {
	s := Setup{}
	_ = json.Unmarshal(raw, &s)
	if s.Due == nil {
		s.Due = map[string]string{}
	}
	if s.Assign == nil {
		s.Assign = map[string]string{}
	}
	if s.ClassIDs == nil {
		s.ClassIDs = []string{}
	}
	return s
}

// plan works out the wizard's view and the exams Create would make.
func (r *Repository) plan(tx *gorm.DB, termID string) (SetupPlan, []plannedExam, error) {
	term, err := r.term(tx, termID)
	if err != nil {
		return SetupPlan{}, nil, err
	}
	setup := readSetup(term.Setup)
	chosen := map[string]bool{}
	for _, id := range setup.ClassIDs {
		chosen[id] = true
	}
	p := SetupPlan{Term: term, Setup: setup, Classes: []SetupClass{}, Assessments: []SetupAssessment{}, Gaps: []SetupGap{}, Teachers: []SetupTeacher{}, Conflicts: []SetupConflict{}}
	classes := []struct {
		ClassID, ClassName string
		BlueprintID        *string
		VersionID          *string
		Version            *int
		Drafts             int
		Sections, Students int
	}{}
	if err := tx.Raw(`SELECT c.id AS class_id, c.name AS class_name, b.id AS blueprint_id, v.id AS version_id, v.version,
       (SELECT count(*) FROM blueprint_versions d WHERE d.blueprint_id = b.id AND d.status = 'draft') AS drafts,
       (SELECT count(*) FROM academic_sections s WHERE s.session_id = ? AND s.class_id = c.id AND s.is_active) AS sections,
       (SELECT count(*) FROM student_rolls st WHERE st.session_id = ? AND st.class_id = c.id) AS students
  FROM academic_classes c
  LEFT JOIN assessment_blueprints b ON b.class_id = c.id AND b.session_id = ?
  LEFT JOIN blueprint_versions v ON v.blueprint_id = b.id AND v.status = 'active'
 WHERE c.is_active ORDER BY c.display_order, c.name`, term.SessionID, term.SessionID, term.SessionID).Scan(&classes).Error; err != nil {
		return p, nil, err
	}
	keyIndex := map[string]int{}
	var exams []plannedExam
	for _, c := range classes {
		sc := SetupClass{ClassID: c.ClassID, ClassName: c.ClassName, Selected: chosen[c.ClassID], BlueprintID: c.BlueprintID, VersionID: c.VersionID,
			HasDraft: c.Drafts > 0, Sections: c.Sections, Students: c.Students, Assessments: []string{}, State: "needs_blueprint"}
		if c.Version != nil {
			sc.Version = *c.Version
		}
		if c.VersionID != nil {
			row, err := r.versionRow(tx, *c.VersionID, false)
			if err != nil {
				return p, nil, err
			}
			d, err := r.definitionOf(tx, row)
			if err != nil {
				return p, nil, err
			}
			for _, a := range d.Assessments {
				if a.TermID == termID {
					sc.Assessments = append(sc.Assessments, a.Key)
				}
			}
			sc.State = "no_assessments"
			if len(sc.Assessments) > 0 {
				sc.State = "ready"
			}
			if sc.Selected && sc.State == "ready" {
				planned, err := r.planClass(tx, term, setup, c.ClassID, *c.VersionID, d)
				if err != nil {
					return p, nil, err
				}
				for _, e := range planned {
					sc.Sheets += len(e.Sheets)
					if e.Exists {
						sc.Existing++
					}
				}
				exams = append(exams, planned...)
				for _, key := range sc.Assessments {
					a, _ := d.Assessment(key)
					if i, ok := keyIndex[key]; ok {
						p.Assessments[i].Classes++
						continue
					}
					keyIndex[key] = len(p.Assessments)
					p.Assessments = append(p.Assessments, SetupAssessment{Key: key, Name: a.Name, Kind: a.Kind, Classes: 1, Due: setup.Due[key]})
				}
			}
		}
		p.Classes = append(p.Classes, sc)
	}
	// Totals, name conflicts and subjects nobody teaches.
	needsTeacher := map[string]*SetupGap{}
	for _, e := range exams {
		p.Exams++
		p.Sheets += len(e.Sheets)
		if !e.Exists {
			p.NewExams++
			p.ToCreate += len(e.Sheets)
			var clash string
			err := tx.Raw("SELECT name FROM exams WHERE session_id = ? AND section_id = ? AND lower(name) = lower(?)", term.SessionID, e.SectionID, e.Name).Row().Scan(&clash)
			if err == nil {
				p.Conflicts = append(p.Conflicts, SetupConflict{ClassName: className(p.Classes, e.ClassID), SectionName: e.SectionName, ExamName: clash})
			}
		}
		// Gaps cover created exams too: their sheets still need a teacher.
		for _, s := range e.Sheets {
			key := e.ClassID + "|" + s.SubjectID
			if g, ok := needsTeacher[key]; ok {
				g.Sheets++
				continue
			}
			var assigned int64
			if err := tx.Raw("SELECT count(*) FROM teacher_assignments ta JOIN employees emp ON emp.id = ta.employee_id WHERE ta.session_id = ? AND ta.class_id = ? AND ta.subject_id = ? AND emp.is_active",
				term.SessionID, e.ClassID, s.SubjectID).Row().Scan(&assigned); err != nil {
				return p, nil, err
			}
			if assigned > 0 {
				continue
			}
			var name string
			tx.Raw("SELECT name_en FROM academic_subjects WHERE id = ?", s.SubjectID).Row().Scan(&name)
			g := &SetupGap{ClassID: e.ClassID, ClassName: className(p.Classes, e.ClassID), SubjectID: s.SubjectID, SubjectName: name, Sheets: 1}
			if a, ok := setup.Assign[key]; ok {
				g.Assignee = &a
			}
			needsTeacher[key] = g
		}
	}
	for _, key := range sortedGapKeys(needsTeacher) {
		p.Gaps = append(p.Gaps, *needsTeacher[key])
	}
	if err := tx.Raw("SELECT id AS employee_id, name_en AS name FROM employees WHERE kind = 'teacher' AND is_active ORDER BY name_en").Scan(&p.Teachers).Error; err != nil {
		return p, nil, err
	}
	return p, exams, nil
}

func sortedGapKeys(m map[string]*SetupGap) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := m[keys[i]], m[keys[j]]
		if a.ClassName != b.ClassName {
			return a.ClassName < b.ClassName
		}
		return a.SubjectName < b.SubjectName
	})
	return keys
}

func className(cs []SetupClass, id string) string {
	for _, c := range cs {
		if c.ClassID == id {
			return c.ClassName
		}
	}
	return ""
}

// planClass lists the exams of one class: a section × assessment each, with
// the subjects that have a sheet in that assessment.
func (r *Repository) planClass(tx *gorm.DB, term TermView, setup Setup, classID, versionID string, d Definition) ([]plannedExam, error) {
	sections := []struct{ ID, Name string }{}
	if err := tx.Raw("SELECT id, name FROM academic_sections WHERE session_id = ? AND class_id = ? AND is_active ORDER BY display_order, name", term.SessionID, classID).Scan(&sections).Error; err != nil {
		return nil, err
	}
	out := []plannedExam{}
	for _, a := range d.Assessments {
		if a.TermID != term.ID {
			continue
		}
		sheets := []plannedSheet{}
		for _, s := range d.Subjects {
			if s.Role == "non_graded" {
				continue
			}
			sh, ok := s.Sheet(a.Key)
			if !ok {
				continue
			}
			full, err := sh.Full()
			if err != nil {
				return nil, ErrInvalid
			}
			pass, err := calc.MarksOf(sh.Pass)
			if err != nil {
				return nil, ErrInvalid
			}
			sheets = append(sheets, plannedSheet{SubjectID: s.SubjectID, Pass: pass, Full: full, Components: sh.Components})
		}
		for _, sec := range sections {
			e := plannedExam{ClassID: classID, SectionID: sec.ID, SectionName: sec.Name, VersionID: versionID, Key: a.Key,
				Name: examName(term.Name, a.Name), Type: examType(a.Kind, term.Kind), Due: setup.Due[a.Key], Sheets: sheets}
			var n int64
			if err := tx.Raw("SELECT count(*) FROM exams WHERE term_id = ? AND assessment_key = ? AND section_id = ?", term.ID, a.Key, sec.ID).Row().Scan(&n); err != nil {
				return nil, err
			}
			e.Exists = n > 0
			out = append(out, e)
		}
	}
	return out, nil
}

// SetupPlan returns the wizard's view of a term.
func (r *Repository) SetupPlan(ctx context.Context, termID string) (SetupPlan, error) {
	p, _, err := r.plan(r.db.WithContext(ctx), termID)
	return p, dbErr(err)
}

// SaveSetup keeps the wizard's choices on the term.
func (r *Repository) SaveSetup(ctx context.Context, actor, termID string, s Setup) (SetupPlan, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		term, err := r.term(tx, termID)
		if err != nil {
			return err
		}
		if term.Status == "closed" {
			return ErrInUse
		}
		body, _ := json.Marshal(s)
		return tx.Exec("UPDATE exam_terms SET setup = ?::jsonb, updated_at = now() WHERE id = ?", string(body), termID).Error
	})
	if err != nil {
		return SetupPlan{}, dbErr(err)
	}
	return r.SetupPlan(ctx, termID)
}

// CreateTerm makes every planned exam and sheet in one transaction. Exams
// that already exist for the term are left alone, so running it again
// creates nothing new.
func (r *Repository) CreateTerm(ctx context.Context, actor, termID string) (SetupResult, error) {
	var res SetupResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT id FROM exam_terms WHERE id = ? FOR UPDATE", termID).Error; err != nil {
			return err
		}
		plan, exams, err := r.plan(tx, termID)
		if err != nil {
			return err
		}
		if plan.Term.Status == "closed" {
			return ErrInUse
		}
		if len(exams) == 0 || len(plan.Conflicts) > 0 {
			return ErrSetupBlocked
		}
		for _, a := range plan.Assessments {
			if a.Due == "" || !validDate(a.Due) {
				return ErrSetupBlocked // every assessment needs its marks due date
			}
		}
		for _, e := range exams {
			if e.Exists {
				continue
			}
			var examID string
			if err := tx.Raw(`INSERT INTO exams (name, exam_type, session_id, class_id, section_id, status, created_by, updated_by, term_id, assessment_key, blueprint_version_id, marks_due_on)
VALUES (?, ?, ?, ?, ?, 'draft', ?, ?, ?, ?, ?, ?) RETURNING id`,
				e.Name, e.Type, plan.Term.SessionID, e.ClassID, e.SectionID, actor, actor, termID, e.Key, e.VersionID, nullDate(e.Due)).Row().Scan(&examID); err != nil {
				return dbErr(err)
			}
			res.ExamsCreated++
			for _, s := range e.Sheets {
				var sheetID string
				if err := tx.Raw("INSERT INTO exam_subjects (exam_id, class_id, subject_id, full_marks, pass_marks) VALUES (?, ?, ?, ?::numeric, ?::numeric) RETURNING id",
					examID, e.ClassID, s.SubjectID, s.Full.String(), s.Pass.String()).Row().Scan(&sheetID); err != nil {
					return dbErr(err)
				}
				res.SheetsCreated++
				if len(s.Components) == 1 && s.Components[0].Key == "TOTAL" {
					continue
				}
				for k, c := range s.Components {
					full, _ := calc.MarksOf(c.Full)
					var pass any
					if c.Pass != nil {
						p, _ := calc.MarksOf(*c.Pass)
						pass = p.String()
					}
					if err := tx.Exec("INSERT INTO exam_subject_components (exam_subject_id, component_key, full_marks, pass_marks, display_order) VALUES (?, ?, ?::numeric, ?::numeric, ?)",
						sheetID, c.Key, full.String(), pass, k+1).Error; err != nil {
						return dbErr(err)
					}
				}
			}
		}
		for _, g := range plan.Gaps {
			if g.Assignee == nil || !isUUID(*g.Assignee) {
				continue
			}
			result := tx.Exec(`INSERT INTO teacher_assignments (employee_id, session_id, class_id, subject_id)
SELECT ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM employees WHERE id = ? AND kind = 'teacher' AND is_active)
ON CONFLICT DO NOTHING`, *g.Assignee, plan.Term.SessionID, g.ClassID, g.SubjectID, *g.Assignee)
			if result.Error != nil {
				return dbErr(result.Error)
			}
			res.Assigned += int(result.RowsAffected)
		}
		if err := tx.Exec("UPDATE exam_terms SET status = 'running', updated_at = now() WHERE id = ? AND status = 'planning'", termID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.term_created", "exam_term", termID, map[string]any{"exams": res.ExamsCreated, "sheets": res.SheetsCreated, "assigned": res.Assigned})
	})
	if err != nil {
		return SetupResult{}, dbErr(err)
	}
	res.Plan, err = r.SetupPlan(ctx, termID)
	return res, err
}
