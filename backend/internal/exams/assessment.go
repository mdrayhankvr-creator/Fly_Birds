package exams

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

// Assessment results: one exam (one assessment of one section). The first
// publication is the exam's own Publish (E3 sheet review is its review);
// corrections are reviewed and approved like term results.

// AssessmentRules are the rules an exam's results are calculated with.
type AssessmentRules struct {
	Format             int                  `json:"format"`
	Scope              string               `json:"scope"`
	Source             string               `json:"source"`
	Bands              []blueprints.BandRef `json:"bands"`
	BandsSource        string               `json:"bands_source"` // blueprint_version or school
	GPARounding        string               `json:"gpa_rounding"`
	Absent             string               `json:"absent"`
	PaperRule          string               `json:"paper_rule"`
	Ranking            string               `json:"ranking"`
	BlueprintVersionID *string              `json:"blueprint_version_id"`
	Combined           map[string]string    `json:"combined"`
}

// AssessmentInputs are the marks a student's assessment result used.
type AssessmentInputs struct {
	Lines []InLine `json:"lines"`
}

// InLine is one mark of an assessment result's inputs.
type InLine struct {
	MarkID     string         `json:"mark_id"`
	SheetID    string         `json:"sheet_id"`
	SubjectID  string         `json:"subject_id"`
	Code       string         `json:"code"`
	NameEN     string         `json:"name_en"`
	NameBN     string         `json:"name_bn"`
	Full       string         `json:"full"`
	Pass       string         `json:"pass"`
	Fourth     bool           `json:"fourth"`
	Combined   string         `json:"combined"`
	Status     string         `json:"status"`
	Marks      *string        `json:"marks"`
	Reason     string         `json:"reason"`
	RowVersion int            `json:"row_version"`
	Components []OutComponent `json:"components"`
}

// freshAssessmentRules are the rules of a first publication: the exam's
// blueprint policy, and the bands of its version (its activation snapshot);
// exams outside a blueprint, and legacy versions, use the school's bands as
// they always have.
func freshAssessmentRules(tx *gorm.DB, e Exam) (AssessmentRules, error) {
	r := AssessmentRules{Format: 1, Scope: ScopeAssessment, Source: "engine", GPARounding: "half_up", Absent: "zero", PaperRule: "combined",
		Ranking: "competition", BlueprintVersionID: e.BlueprintVersionID, Combined: map[string]string{}}
	var err error
	if e.BlueprintVersionID == nil {
		r.BandsSource = "school"
		r.Bands, err = blueprints.SchoolBands(tx)
		return r, err
	}
	var v struct{ GPARounding, AbsentPolicy, RankMode, PaperRule string }
	if err := tx.Raw("SELECT gpa_rounding, absent_policy, rank_mode, paper_rule FROM blueprint_versions WHERE id = ?", *e.BlueprintVersionID).Scan(&v).Error; err != nil {
		return r, err
	}
	for _, s := range []*struct {
		dst *string
		v   string
	}{{&r.GPARounding, v.GPARounding}, {&r.Absent, v.AbsentPolicy}, {&r.Ranking, v.RankMode}, {&r.PaperRule, v.PaperRule}} {
		if s.v != "" {
			*s.dst = s.v
		}
	}
	rows := []struct{ SubjectID, CombinedKey string }{}
	if err := tx.Raw("SELECT subject_id, combined_key FROM blueprint_subjects WHERE version_id = ? AND combined_key IS NOT NULL", *e.BlueprintVersionID).Scan(&rows).Error; err != nil {
		return r, err
	}
	for _, row := range rows {
		r.Combined[row.SubjectID] = row.CombinedKey
	}
	r.Bands, r.BandsSource, err = blueprints.VersionBands(tx, *e.BlueprintVersionID)
	return r, err
}

// engine turns the rules into what ScoreLinesWith takes.
func (r AssessmentRules) engine() ([]Band, ScorePolicy, calc.RankMode) {
	bands := make([]Band, 0, len(r.Bands))
	for _, b := range r.Bands {
		bands = append(bands, Band{Grade: b.Grade, MinMarks: b.Min, MaxMarks: b.Max, GPA: b.GPA})
	}
	p := ScorePolicy{EachPaper: r.PaperRule == "each_paper"}
	if r.GPARounding == "truncate" {
		p.GPA = calc.Truncate
	}
	if r.Absent == "missing" {
		p.Absent = calc.AbsentIsMissing
	}
	mode := calc.Competition
	if r.Ranking == "dense" {
		mode = calc.Dense
	}
	return bands, p, mode
}

// storedAssessmentRules are the rules the exam's current results were
// published with; corrections reuse them so a correction never regrades
// the class under different bands.
func storedAssessmentRules(tx *gorm.DB, examID, classID string) (AssessmentRules, *publicationRow, error) {
	p, err := rulesPublication(tx, ScopeAssessment, examID, classID)
	if err != nil || p == nil {
		if p == nil && err == nil {
			err = ErrNotPublished
		}
		return AssessmentRules{}, nil, err
	}
	var r AssessmentRules
	if err := json.Unmarshal([]byte(p.Rules), &r); err != nil {
		return r, nil, err
	}
	if r.Combined == nil {
		r.Combined = map[string]string{}
	}
	return r, p, nil
}

func parseFloat(s *string) *float64 {
	if s == nil {
		return nil
	}
	v, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		return nil
	}
	return &v
}

// assessmentBatch computes every student's result of an exam: the active
// students of its section and anyone who already has a published version.
func assessmentBatch(tx *gorm.DB, e Exam, rules AssessmentRules) (batch, error) {
	b := batch{Scope: ScopeAssessment, ScopeID: e.ID, SessionID: e.SessionID, ClassID: e.ClassID, SectionID: &e.SectionID,
		BlueprintVersionID: e.BlueprintVersionID, Rules: rules, Candidates: []candidate{}, Blockers: []ReviewBlocker{}}
	bands, policy, _ := rules.engine()
	students := []string{}
	if err := tx.Raw(`SELECT student_id FROM student_rolls WHERE session_id = ? AND class_id = ? AND section_id = ?
 UNION SELECT student_id FROM result_versions WHERE scope = 'assessment' AND scope_id = ? AND status = 'published' ORDER BY 1`,
		e.SessionID, e.ClassID, e.SectionID, e.ID).Scan(&students).Error; err != nil {
		return b, err
	}
	if len(students) == 0 {
		return b, nil
	}
	rows := []struct {
		StudentID, NameEN, NameBN, AdmissionNo         string
		RollNo                                         *int
		SheetID, SubjectID, Code, SubjectEN, SubjectBN string
		Full, Pass                                     string
		Fourth                                         bool
		MarkID, Status, Marks                          *string
		Reason                                         string
		RowVersion                                     int
		Takes                                          bool
	}{}
	if err := tx.Raw(`SELECT st.id AS student_id, st.name_en, coalesce(st.name_bn, '') AS name_bn, st.admission_no, en.roll_no,
       es.id AS sheet_id, es.subject_id, sub.code, sub.name_en AS subject_en, sub.name_bn AS subject_bn,
       es.full_marks::text AS full, es.pass_marks::text AS pass, acs.is_fourth_subject AS fourth,
       m.id::text AS mark_id, m.status, m.marks::text AS marks, coalesce(m.status_reason, '') AS reason, coalesce(m.row_version, 0) AS row_version,
       student_takes_subject(st.id, ?, ?, es.subject_id) AS takes
  FROM students st JOIN student_enrolments en ON en.student_id = st.id AND en.session_id = ?
  CROSS JOIN exam_subjects es
  JOIN academic_subjects sub ON sub.id = es.subject_id
  JOIN academic_class_subjects acs ON acs.class_id = es.class_id AND acs.subject_id = es.subject_id
  LEFT JOIN exam_marks m ON m.exam_subject_id = es.id AND m.student_id = st.id
 WHERE es.exam_id = ? AND st.id IN ?
 ORDER BY st.id, es.subject_id`, e.SessionID, e.ClassID, e.SessionID, e.ID, students).Scan(&rows).Error; err != nil {
		return b, err
	}
	comps := []struct {
		MarkID, Key, Full string
		Marks, Pass       *string
	}{}
	if err := tx.Raw(`SELECT cm.exam_mark_id::text AS mark_id, c.component_key AS key, cm.marks::text AS marks, c.full_marks::text AS full, c.pass_marks::text AS pass
  FROM exam_component_marks cm JOIN exam_subject_components c ON c.exam_subject_id = cm.exam_subject_id AND c.component_key = cm.component_key
  JOIN exam_marks m ON m.id = cm.exam_mark_id
 WHERE m.exam_id = ? ORDER BY c.display_order, c.component_key`, e.ID).Scan(&comps).Error; err != nil {
		return b, err
	}
	compsOf := map[string][]OutComponent{}
	for _, c := range comps {
		compsOf[c.MarkID] = append(compsOf[c.MarkID], OutComponent{Key: c.Key, Marks: c.Marks, Full: c.Full, Pass: c.Pass})
	}
	termName := ""
	if e.TermName != nil {
		termName = *e.TermName
	}
	assessment := ""
	if e.AssessmentKey != nil {
		assessment = *e.AssessmentKey
	}
	ctx := SnapshotContext{ExamID: e.ID, ExamName: e.Name, ExamType: e.ExamType, TermID: e.TermID, TermName: termName, AssessmentKey: assessment}
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].StudentID == rows[i].StudentID {
			j++
		}
		first := rows[i]
		c := candidate{StudentID: first.StudentID, Context: ctx,
			Student: SnapshotStudent{ID: first.StudentID, NameEN: first.NameEN, NameBN: first.NameBN, AdmissionNo: first.AdmissionNo, RollNo: first.RollNo,
				SectionID: e.SectionID, SectionName: e.SectionName, ClassID: e.ClassID, ClassName: e.ClassName, SessionID: e.SessionID, SessionName: e.SessionName}}
		inputs := AssessmentInputs{Lines: []InLine{}}
		lines := []MarkLine{}
		for _, row := range rows[i:j] {
			if !row.Takes {
				continue
			}
			if row.MarkID == nil {
				c.Blocker = "missing_marks"
				continue
			}
			if *row.Status == StatusIncomplete {
				c.Blocker = "incomplete"
			}
			in := InLine{MarkID: *row.MarkID, SheetID: row.SheetID, SubjectID: row.SubjectID, Code: row.Code, NameEN: row.SubjectEN, NameBN: row.SubjectBN,
				Full: row.Full, Pass: row.Pass, Fourth: row.Fourth, Combined: rules.Combined[row.SubjectID], Status: *row.Status, Marks: row.Marks,
				Reason: row.Reason, RowVersion: row.RowVersion, Components: compsOf[*row.MarkID]}
			if in.Components == nil {
				in.Components = []OutComponent{}
			}
			inputs.Lines = append(inputs.Lines, in)
			lines = append(lines, inLineToMark(in))
		}
		c.Inputs = inputs
		i = j
		if c.Blocker == "" {
			if len(inputs.Lines) == 0 {
				c.Blocker = "missing_marks"
			} else if err := scoreAssessment(&c, inputs, lines, bands, policy, rules); err != nil {
				return b, err
			}
		}
		b.Candidates = append(b.Candidates, c)
	}
	return b, nil
}

func inLineToMark(in InLine) MarkLine {
	full, pass := parseFloat(&in.Full), parseFloat(&in.Pass)
	ml := MarkLine{Marks: parseFloat(in.Marks), Fourth: in.Fourth, Status: in.Status, Combined: in.Combined}
	if full != nil {
		ml.FullMarks = *full
	}
	if pass != nil {
		ml.PassMarks = *pass
	}
	for _, c := range in.Components {
		v := parseFloat(c.Marks)
		if v == nil {
			continue
		}
		ml.Components = append(ml.Components, ComponentLine{Key: c.Key, Marks: *v, Pass: parseFloat(c.Pass)})
	}
	return ml
}

// scoreAssessment grades a student's lines and fills the candidate.
func scoreAssessment(c *candidate, inputs AssessmentInputs, lines []MarkLine, bands []Band, policy ScorePolicy, rules AssessmentRules) error {
	score, err := ScoreLinesWith(lines, bands, policy)
	switch {
	case errors.Is(err, ErrMarksIncomplete):
		c.Blocker = "incomplete"
		return nil
	case errors.Is(err, ErrAllSubjectsExempt):
		c.Blocker = "all_exempt"
		return nil
	case err != nil:
		return err
	}
	if score.Withheld {
		c.Outputs = Outputs{Outcome: "withheld", Subjects: []OutLine{}}
		return nil
	}
	out := Outputs{Total: two(score.Result.Total), Full: two(score.Result.Full), Passed: score.Result.Passed, Subjects: []OutLine{}}
	gpa := score.Result.GPA.String()
	out.GPA = &gpa
	out.Outcome = "failed"
	if score.Result.Passed {
		out.Outcome = "passed"
	}
	for i, in := range inputs.Lines {
		g := score.Lines[i]
		line := OutLine{Key: in.SubjectID, SheetID: in.SheetID, SubjectIDs: []string{in.SubjectID}, Code: in.Code, NameEN: in.NameEN, NameBN: in.NameBN,
			Optional: in.Fourth, Combined: in.Combined, Status: in.Status, Marks: in.Marks, Full: in.Full, Pass: in.Pass,
			Grade: g.Grade, Passed: g.Passed, Reasons: []blueprints.Reason{}, Components: in.Components}
		if g.Point != nil {
			line.Point = ptr(g.Point.String())
		}
		if in.Status == StatusPresent {
			for _, comp := range in.Components {
				m, p := parseFloat(comp.Marks), parseFloat(comp.Pass)
				if m != nil && p != nil {
					mm, _ := calc.MarksOf(*m)
					pp, _ := calc.MarksOf(*p)
					if mm < pp {
						line.Reasons = append(line.Reasons, blueprints.Reason{Code: "component_below_pass", SubjectID: in.SubjectID, Component: comp.Key})
					}
				}
			}
			if rules.PaperRule == "each_paper" && in.Combined != "" && in.Marks != nil {
				mm, _ := calc.MarksOf(*parseFloat(in.Marks))
				pp, _ := calc.MarksOf(*parseFloat(&in.Pass))
				if mm < pp {
					line.Reasons = append(line.Reasons, blueprints.Reason{Code: "paper_below_pass", SubjectID: in.SubjectID})
				}
			}
		}
		out.Subjects = append(out.Subjects, line)
		mg := markGrade{MarkID: in.MarkID, Grade: g.Grade, Passed: g.Passed}
		if g.Point != nil {
			mg.Point = ptr(g.Point.String())
		}
		c.grades = append(c.grades, mg)
	}
	sort.SliceStable(out.Subjects, func(i, j int) bool {
		if out.Subjects[i].Code != out.Subjects[j].Code {
			return out.Subjects[i].Code < out.Subjects[j].Code
		}
		return out.Subjects[i].Key < out.Subjects[j].Key
	})
	c.Outputs = out
	c.ranked = &calc.Ranked{Key: c.StudentID, GPA: score.Result.GPA, Total: score.Result.Total, Passed: score.Result.Passed}
	return nil
}

// ---------------------------------------------------------------------------
// Positions across the sections of a class

// positionChange is a published student of another exam whose positions
// change because this exam's results are published, corrected or withdrawn.
type positionChange struct {
	Version         versionRow
	ClassPosition   int
	SectionPosition int
}

// groupExamIDs are the other published exams ranked with e: the same term
// and assessment (E1-05), or for exams outside a term the same name and type.
func groupExamIDs(tx *gorm.DB, e Exam) ([]string, error) {
	ids := []string{}
	q := tx.Table("exams x").Where("x.id <> ? AND x.session_id = ? AND x.class_id = ? AND x.status IN ('published','locked')", e.ID, e.SessionID, e.ClassID)
	if e.TermID != nil && e.AssessmentKey != nil {
		q = q.Where("x.term_id = ? AND x.assessment_key = ?", *e.TermID, *e.AssessmentKey)
	} else {
		q = q.Where("lower(x.name) = lower(?) AND x.exam_type = ? AND x.term_id IS NULL", e.Name, e.ExamType)
	}
	err := q.Order("x.id").Pluck("x.id", &ids).Error
	return ids, err
}

// rankGroup sets the candidates' positions among the class's other
// published sections, and lists the other students whose positions change.
// With include false the exam's own results leave the ranking (withdrawal).
func rankGroup(tx *gorm.DB, e Exam, b *batch, mode calc.RankMode, include bool) error {
	others, err := groupExamIDs(tx, e)
	if err != nil {
		return err
	}
	versions := []versionRow{}
	if len(others) > 0 {
		if err := tx.Table("result_versions v").Select(versionColumns).Where("v.scope = 'assessment' AND v.scope_id IN ? AND v.status = 'published'", others).Order("v.id").Find(&versions).Error; err != nil {
			return err
		}
	}
	entries := []calc.Ranked{}
	sectionOf := map[string]string{}
	stored := map[string]versionRow{}
	b.Group = []string{}
	for _, v := range versions {
		b.Group = append(b.Group, v.ID)
		if v.Outcome == "withheld" || v.GPA == nil || v.TotalMarks == nil {
			continue
		}
		gpa, err1 := calc.PointsOf(*parseFloat(v.GPA))
		total, err2 := calc.MarksOf(*parseFloat(v.TotalMarks))
		if err1 != nil || err2 != nil {
			return ErrInvalid
		}
		key := "v:" + v.ID
		entries = append(entries, calc.Ranked{Key: key, GPA: gpa, Total: total, Passed: v.Outcome == "passed"})
		sectionOf[key] = v.SectionID
		stored[key] = v
	}
	if include {
		for _, c := range b.Candidates {
			if c.ranked != nil && c.Blocker == "" {
				key := "c:" + c.StudentID
				entries = append(entries, calc.Ranked{Key: key, GPA: c.ranked.GPA, Total: c.ranked.Total, Passed: c.ranked.Passed})
				sectionOf[key] = e.SectionID
			}
		}
	}
	class := calc.Rank(entries, mode)
	bySection := map[string][]calc.Ranked{}
	for _, en := range entries {
		bySection[sectionOf[en.Key]] = append(bySection[sectionOf[en.Key]], en)
	}
	section := map[string]int{}
	for _, list := range bySection {
		for k, p := range calc.Rank(list, mode) {
			section[k] = p
		}
	}
	for i := range b.Candidates {
		c := &b.Candidates[i]
		if include && c.ranked != nil && c.Blocker == "" {
			c.Outputs.ClassPosition, c.Outputs.SectionPosition = ptr(class["c:"+c.StudentID]), ptr(section["c:"+c.StudentID])
		}
	}
	b.others = nil
	keys := make([]string, 0, len(stored))
	for k := range stored {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := stored[k]
		cp, sp := class[k], section[k]
		if v.ClassPosition == nil || v.SectionPosition == nil || *v.ClassPosition != cp || *v.SectionPosition != sp {
			b.others = append(b.others, positionChange{Version: v, ClassPosition: cp, SectionPosition: sp})
		}
	}
	return nil
}

// applyPositionChanges publishes a position update for each affected exam:
// a new version per student with the new positions (the snapshot is copied
// with only its positions changed) that supersedes the previous one.
func applyPositionChanges(tx *gorm.DB, actor string, changes []positionChange, reason string) (int, error) {
	byExam := map[string][]positionChange{}
	order := []string{}
	for _, c := range changes {
		if _, ok := byExam[c.Version.ScopeID]; !ok {
			order = append(order, c.Version.ScopeID)
		}
		byExam[c.Version.ScopeID] = append(byExam[c.Version.ScopeID], c)
	}
	sort.Strings(order)
	n := 0
	for _, examID := range order {
		list := byExam[examID]
		first := list[0].Version
		src, err := latestPublished(tx, ScopeAssessment, examID, first.ClassID)
		if err != nil {
			return n, err
		}
		if src == nil {
			return n, ErrConflict
		}
		hashParts := []any{}
		for _, c := range list {
			hashParts = append(hashParts, c.Version.ID, c.ClassPosition, c.SectionPosition)
		}
		raw, _ := json.Marshal(hashParts)
		var pubID string
		if err := tx.Raw(`INSERT INTO result_publications (scope, scope_id, session_id, class_id, section_id, revision, kind, status, source, blueprint_version_id, rules, rules_sha256, inputs_sha256, reason, students, changed, published_by, published_at)
SELECT scope, scope_id, session_id, class_id, section_id,
       (SELECT max(revision) + 1 FROM result_publications x WHERE x.scope = p.scope AND x.scope_id = p.scope_id AND x.class_id = p.class_id),
       'position_update', 'published', source, blueprint_version_id, rules, '', encode(sha256(convert_to(?, 'UTF8')), 'hex'), ?, ?, ?, ?, now()
  FROM result_publications p WHERE p.id = ? RETURNING id`, string(raw), reason, len(list), len(list), actor, src.ID).Row().Scan(&pubID); err != nil {
			return n, dbErr(err)
		}
		for _, c := range list {
			if err := closeVersion(tx, c.Version.ID, "superseded", actor, reason); err != nil {
				return n, err
			}
			var id string
			if err := tx.Raw(`INSERT INTO result_versions (publication_id, scope, scope_id, session_id, class_id, section_id, student_id, version, status, outcome,
 total_marks, full_marks, gpa, class_position, section_position, inputs_sha256, snapshot, reason, supersedes_id, published_by, published_at)
SELECT ?, v.scope, v.scope_id, v.session_id, v.class_id, v.section_id, v.student_id,
       (SELECT max(x.version) + 1 FROM result_versions x WHERE x.scope = v.scope AND x.scope_id = v.scope_id AND x.student_id = v.student_id),
       'published', v.outcome, v.total_marks, v.full_marks, v.gpa, ?, ?, '',
       jsonb_set(jsonb_set(v.snapshot, '{outputs,class_position}', to_jsonb(?::integer)), '{outputs,section_position}', to_jsonb(?::integer)),
       ?, v.id, ?, now()
  FROM result_versions v WHERE v.id = ? RETURNING id`, pubID, c.ClassPosition, c.SectionPosition, c.ClassPosition, c.SectionPosition, reason, actor, c.Version.ID).Row().Scan(&id); err != nil {
				return n, dbErr(err)
			}
			if err := tx.Exec("UPDATE exam_results SET class_position = ?, section_position = ?, result_version_id = ? WHERE exam_id = ? AND student_id = ?",
				c.ClassPosition, c.SectionPosition, id, examID, c.Version.StudentID).Error; err != nil {
				return n, err
			}
			n++
		}
		if err := recordAudit(tx, actor, "exam.result_positions_updated", "exam", examID, map[string]any{"publication_id": pubID, "students": len(list), "reason": reason}); err != nil {
			return n, err
		}
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Writing assessment results

// writeAssessment publishes candidates of an exam: grades on the marks, the
// versions (superseding current ones) and the exam_results projection.
func writeAssessment(tx *gorm.DB, actor, pubID string, b batch, list []candidate, current map[string]versionRow, reason string) error {
	for _, c := range list {
		var supersedes *string
		if old, ok := current[c.StudentID]; ok {
			if err := closeVersion(tx, old.ID, "superseded", actor, reason); err != nil {
				return err
			}
			supersedes = &old.ID
		}
		id, err := insertVersion(tx, pubID, b, c, supersedes, reason, actor)
		if err != nil {
			return err
		}
		if err := tx.Exec("UPDATE exam_marks SET grade = NULL, grade_point = NULL, passed = NULL WHERE exam_id = ? AND student_id = ?", b.ScopeID, c.StudentID).Error; err != nil {
			return err
		}
		for _, g := range c.grades {
			if err := tx.Exec("UPDATE exam_marks SET grade = ?, grade_point = ?::numeric, passed = ? WHERE id = ?", g.Grade, g.Point, g.Passed, g.MarkID).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("DELETE FROM exam_results WHERE exam_id = ? AND student_id = ?", b.ScopeID, c.StudentID).Error; err != nil {
			return err
		}
		if c.Outputs.Outcome == "withheld" {
			continue // a withheld result has no projection: the student sees "withheld"
		}
		if err := tx.Exec(`INSERT INTO exam_results (exam_id, student_id, session_id, class_id, section_id, total_marks, full_marks, gpa, passed, class_position, section_position, result_version_id)
VALUES (?, ?, ?, ?, ?, ?::numeric, ?::numeric, ?::numeric, ?, ?, ?, ?)`, b.ScopeID, c.StudentID, b.SessionID, b.ClassID, *b.SectionID,
			c.Outputs.Total, c.Outputs.Full, c.Outputs.GPA, c.Outputs.Passed, c.Outputs.ClassPosition, c.Outputs.SectionPosition, id).Error; err != nil {
			return dbErr(err)
		}
	}
	return nil
}

// publishAssessment is the first publication of an exam (or a new one after
// a withdrawal): every student's result becomes a version.
func publishAssessment(tx *gorm.DB, actor string, e Exam) (string, int, error) {
	rules, err := freshAssessmentRules(tx, e)
	if err != nil {
		return "", 0, err
	}
	if len(rules.Bands) == 0 {
		return "", 0, ErrInvalid
	}
	b, err := assessmentBatch(tx, e, rules)
	if err != nil {
		return "", 0, err
	}
	for _, c := range b.Candidates {
		switch c.Blocker {
		case "":
		case "all_exempt":
			return "", 0, ErrAllSubjectsExempt
		default:
			return "", 0, ErrMarksIncomplete
		}
	}
	_, _, mode := rules.engine()
	if err := rankGroup(tx, e, &b, mode, true); err != nil {
		return "", 0, err
	}
	hash, err := b.hash()
	if err != nil {
		return "", 0, err
	}
	pubID, err := insertPublication(tx, b, "publication", "published", "", hash, actor, len(b.Candidates), len(b.Candidates))
	if err != nil {
		return "", 0, err
	}
	if err := writeAssessment(tx, actor, pubID, b, b.Candidates, map[string]versionRow{}, ""); err != nil {
		return "", 0, err
	}
	moved, err := applyPositionChanges(tx, actor, b.others, fmt.Sprintf("Class position changed: %s · %s was published", e.Name, e.SectionName))
	return pubID, moved, err
}

// withdrawAssessment takes an exam's results back: its versions are
// withdrawn (kept, never deleted), the projection rows go, and the other
// sections' class positions are updated.
func withdrawAssessment(tx *gorm.DB, actor string, e Exam, reason string) (int, int, error) {
	current, err := currentVersions(tx, ScopeAssessment, e.ID, "")
	if err != nil {
		return 0, 0, err
	}
	for _, v := range current {
		if err := closeVersion(tx, v.ID, "withdrawn", actor, reason); err != nil {
			return 0, 0, err
		}
	}
	if err := tx.Exec("DELETE FROM exam_results WHERE exam_id = ?", e.ID).Error; err != nil {
		return 0, 0, err
	}
	if err := tx.Exec("UPDATE result_corrections SET status = 'cancelled', closed_by = ?, closed_at = now() WHERE exam_id = ? AND status = 'open'", actor, e.ID).Error; err != nil {
		return 0, 0, err
	}
	rules, err := freshAssessmentRules(tx, e)
	if err != nil {
		return 0, 0, err
	}
	_, _, mode := rules.engine()
	b := batch{Scope: ScopeAssessment, ScopeID: e.ID}
	if err := rankGroup(tx, e, &b, mode, false); err != nil {
		return 0, 0, err
	}
	moved, err := applyPositionChanges(tx, actor, b.others, fmt.Sprintf("Class position changed: %s · %s was withdrawn", e.Name, e.SectionName))
	return len(current), moved, err
}
