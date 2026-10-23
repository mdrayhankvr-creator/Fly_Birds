package exams

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"gorm.io/gorm"
)

// Term and annual results of a class (E6). Both are computed by the exact
// engine (internal/blueprints over internal/results/calc), reviewed and
// approved by the head teacher, then published by an administrator.

// TermRules are the rules a class's term results are calculated with.
type TermRules struct {
	Format             int                   `json:"format"`
	Scope              string                `json:"scope"`
	TermID             string                `json:"term_id"`
	TermName           string                `json:"term_name"`
	TermSequence       int                   `json:"term_sequence"`
	BlueprintVersionID string                `json:"blueprint_version_id"`
	Version            int                   `json:"version"`
	Legacy             bool                  `json:"legacy"`
	DefinitionSHA256   string                `json:"definition_sha256"`
	Definition         blueprints.Definition `json:"definition"`
	Bands              []blueprints.BandRef  `json:"bands"`
	BandsSource        string                `json:"bands_source"`
}

// TermInputs are the marks a student's term result used.
type TermInputs struct {
	// Marks by assessment key, then subject id.
	Marks    map[string]map[string]TermMarkIn `json:"marks"`
	NotTaken []string                         `json:"not_taken"`
}

// TermMarkIn is one mark of a term result's inputs.
type TermMarkIn struct {
	Status     string         `json:"status"`
	Marks      *string        `json:"marks"`
	Full       string         `json:"full"`
	Pass       string         `json:"pass"`
	Components []OutComponent `json:"components"`
}

// AnnualRulesSnapshot are the rules a class's annual results use.
type AnnualRulesSnapshot struct {
	Format             int                    `json:"format"`
	Scope              string                 `json:"scope"`
	SessionID          string                 `json:"session_id"`
	BlueprintVersionID string                 `json:"blueprint_version_id"`
	Version            int                    `json:"version"`
	Legacy             bool                   `json:"legacy"`
	Annual             blueprints.AnnualRules `json:"annual"`
	Bands              []blueprints.BandRef   `json:"bands"`
	BandsSource        string                 `json:"bands_source"`
}

// AnnualInputs are the published term results an annual result used.
type AnnualInputs struct {
	Terms []blueprints.AnnualTermInput `json:"terms"`
}

func marksText(v *float64) *string {
	if v == nil {
		return nil
	}
	s, err := marksString(*v)
	if err != nil {
		return nil
	}
	return &s
}

func floatText(v float64) string {
	s, err := marksString(v)
	if err != nil {
		return ""
	}
	return s
}

// termInputsOf converts a loaded student's marks into snapshot inputs.
func termInputsOf(st blueprints.TermStudent) TermInputs {
	in := TermInputs{Marks: map[string]map[string]TermMarkIn{}, NotTaken: st.NotTaken}
	for key, bySubject := range st.Input.Marks {
		in.Marks[key] = map[string]TermMarkIn{}
		for subject, m := range bySubject {
			mi := TermMarkIn{Status: m.Status, Marks: marksText(m.Marks), Full: floatText(m.Full), Pass: floatText(m.Pass), Components: []OutComponent{}}
			for _, c := range m.Components {
				oc := OutComponent{Key: c.Key, Marks: marksText(&c.Marks), Full: floatText(c.Full)}
				if c.Pass != nil {
					oc.Pass = marksText(c.Pass)
				}
				mi.Components = append(mi.Components, oc)
			}
			in.Marks[key][subject] = mi
		}
	}
	return in
}

// studentOfTermInputs rebuilds the engine input from snapshot inputs.
func studentOfTermInputs(id, sectionID string, in TermInputs) blueprints.Student {
	skip := map[string]bool{}
	for _, s := range in.NotTaken {
		skip[s] = true
	}
	st := blueprints.Student{ID: id, SectionID: sectionID, Marks: map[string]map[string]blueprints.Mark{}, Takes: func(s string) bool { return !skip[s] }}
	for key, bySubject := range in.Marks {
		st.Marks[key] = map[string]blueprints.Mark{}
		for subject, m := range bySubject {
			mk := blueprints.Mark{Status: m.Status, Marks: parseFloat(m.Marks)}
			if f := parseFloat(&m.Full); f != nil {
				mk.Full = *f
			}
			if p := parseFloat(&m.Pass); p != nil {
				mk.Pass = *p
			}
			for _, c := range m.Components {
				cm := blueprints.ComponentMark{Key: c.Key, Pass: parseFloat(c.Pass)}
				if v := parseFloat(c.Marks); v != nil {
					cm.Marks = *v
				}
				if f := parseFloat(&c.Full); f != nil {
					cm.Full = *f
				}
				mk.Components = append(mk.Components, cm)
			}
			st.Marks[key][subject] = mk
		}
	}
	return st
}

// outputsOf turns an engine result into published outputs, with subject
// names (combined papers joined with " + ").
func outputsOf(r blueprints.StudentResult, subjects map[string]blueprints.SubjectRef) Outputs {
	out := Outputs{Outcome: r.Outcome, Passed: r.Outcome == "passed", Subjects: []OutLine{}}
	if r.Outcome == "passed" || r.Outcome == "failed" {
		out.Total, out.Full, out.GPA = ptr(r.Total), ptr(r.Full), ptr(r.GPA)
		if r.ClassPosition > 0 {
			out.ClassPosition = ptr(r.ClassPosition)
		}
		if r.SectionPosition > 0 {
			out.SectionPosition = ptr(r.SectionPosition)
		}
	}
	for _, s := range r.Subjects {
		line := OutLine{SubjectIDs: s.SubjectIDs, Optional: s.Optional, Combined: s.CombinedKey, Status: s.Status, Full: s.Full, Pass: s.Pass,
			Reasons: s.Reasons, Components: []OutComponent{}, Contributions: s.Contributions, Terms: s.Terms}
		line.Key = s.CombinedKey
		if line.Key == "" && len(s.SubjectIDs) > 0 {
			line.Key = s.SubjectIDs[0]
		}
		en, bn := []string{}, []string{}
		for i, id := range s.SubjectIDs {
			ref := subjects[id]
			if i == 0 {
				line.Code = ref.Code
			}
			en, bn = append(en, ref.NameEN), append(bn, firstNonEmpty(ref.NameBN, ref.NameEN))
		}
		line.NameEN, line.NameBN = strings.Join(en, " + "), strings.Join(bn, " + ")
		if s.Status == "graded" {
			line.Marks = ptr(s.Marks)
			if s.Grade != "" {
				line.Grade, line.Point, line.Passed = ptr(s.Grade), ptr(s.Point), ptr(s.Passed)
			}
		}
		if line.Reasons == nil {
			line.Reasons = []blueprints.Reason{}
		}
		out.Subjects = append(out.Subjects, line)
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// sessionClassNames are the session and class names for snapshots.
func sessionClassNames(tx *gorm.DB, sessionID, classID string) (string, string, error) {
	var row struct{ Session, Class string }
	err := tx.Raw("SELECT (SELECT name FROM academic_sessions WHERE id = ?) AS session, (SELECT name FROM academic_classes WHERE id = ?) AS class", sessionID, classID).Scan(&row).Error
	return row.Session, row.Class, err
}

// termBatch computes a class's term results. stored are the rules of the
// first publication (corrections reuse them); nil means fresh rules.
func termBatch(tx *gorm.DB, termID, classID string, stored *TermRules) (batch, blueprints.TermData, error) {
	d, err := blueprints.NewRepository(tx).LoadTerm(tx, termID, classID)
	if err != nil {
		return batch{}, d, err
	}
	b := batch{Scope: ScopeTerm, ScopeID: termID, SessionID: d.Term.SessionID, ClassID: classID, Candidates: []candidate{}, Blockers: []ReviewBlocker{}, Group: []string{}}
	switch d.State {
	case "no_exams", "mixed_versions":
		b.Blockers = append(b.Blockers, ReviewBlocker{Code: d.State})
		b.Rules = TermRules{Format: 1, Scope: ScopeTerm, TermID: termID}
		return b, d, nil
	}
	b.BlueprintVersionID = &d.VersionID
	rules := TermRules{Format: 1, Scope: ScopeTerm, TermID: termID, TermName: d.Term.Name, TermSequence: d.Term.Sequence, BlueprintVersionID: d.VersionID,
		Version: d.Version, Legacy: d.Legacy, DefinitionSHA256: d.DefinitionSHA, Definition: d.Definition, Bands: d.Bands, BandsSource: d.BandsSource}
	if stored != nil {
		rules = *stored
	}
	b.Rules = rules
	for _, p := range d.Pending {
		b.Blockers = append(b.Blockers, ReviewBlocker{Code: "sheet_" + p.Status, Detail: p.SectionName + " · " + p.Assessment + " · " + p.SubjectName, Count: p.Missing, ExamID: p.ExamID, SheetID: p.SheetID})
	}
	scheme, err := blueprints.SchemeOf(rules.Bands)
	if err != nil {
		return b, d, ErrInvalid
	}
	results, err := blueprints.TermResults(rules.Definition, termID, d.Inputs(), scheme)
	if err != nil {
		return b, d, ErrInvalid
	}
	sessionName, className, err := sessionClassNames(tx, d.Term.SessionID, classID)
	if err != nil {
		return b, d, err
	}
	subjects := map[string]blueprints.SubjectRef{}
	for _, s := range d.Subjects {
		subjects[s.ID] = s
	}
	ctx := SnapshotContext{TermID: &termID, TermName: d.Term.Name, TermSequence: d.Term.Sequence}
	for i, st := range d.Students {
		c := candidate{StudentID: st.ID, Context: ctx, Inputs: termInputsOf(st), Outputs: outputsOf(results[i], subjects),
			Student: SnapshotStudent{ID: st.ID, NameEN: st.Name, NameBN: st.NameBN, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo, SectionID: st.SectionID,
				SectionName: st.SectionName, ClassID: classID, ClassName: className, SessionID: d.Term.SessionID, SessionName: sessionName}}
		if results[i].Outcome == "incomplete" {
			c.Blocker = "incomplete"
		}
		b.Candidates = append(b.Candidates, c)
	}
	return b, d, nil
}

// activeVersion is a class's active blueprint version in a session.
func activeVersion(tx *gorm.DB, sessionID, classID string) (string, int, error) {
	var v struct {
		ID      string
		Version int
	}
	err := tx.Raw(`SELECT v.id, v.version FROM assessment_blueprints b JOIN blueprint_versions v ON v.blueprint_id = b.id AND v.status = 'active'
 WHERE b.session_id = ? AND b.class_id = ?`, sessionID, classID).Scan(&v).Error
	return v.ID, v.Version, err
}

// annualBatch computes a class's annual results from its published term
// results. stored are the rules of the first publication; nil means fresh.
func annualBatch(tx *gorm.DB, sessionID, classID string, stored *AnnualRulesSnapshot) (batch, error) {
	b := batch{Scope: ScopeAnnual, ScopeID: sessionID, SessionID: sessionID, ClassID: classID, Candidates: []candidate{}, Blockers: []ReviewBlocker{}, Group: []string{}}
	var rules AnnualRulesSnapshot
	if stored != nil {
		rules = *stored
	} else {
		versionID, version, err := activeVersion(tx, sessionID, classID)
		if err != nil {
			return b, err
		}
		if versionID == "" {
			b.Blockers = append(b.Blockers, ReviewBlocker{Code: "no_blueprint"})
			b.Rules = AnnualRulesSnapshot{Format: 1, Scope: ScopeAnnual, SessionID: sessionID}
			return b, nil
		}
		repo := blueprints.NewRepository(tx)
		d, legacy, err := repo.DefinitionOf(tx, versionID)
		if err != nil {
			return b, err
		}
		termIDs := map[string]bool{}
		for _, a := range d.Assessments {
			termIDs[a.TermID] = true
		}
		ids := make([]string, 0, len(termIDs))
		for id := range termIDs {
			ids = append(ids, id)
		}
		terms := []blueprints.AnnualTerm{}
		if len(ids) > 0 {
			if err := tx.Raw("SELECT id, name, sequence FROM exam_terms WHERE id IN ? ORDER BY sequence", ids).Scan(&terms).Error; err != nil {
				return b, err
			}
		}
		for i := range terms {
			terms[i].Weight = d.Annual.Weights[terms[i].ID]
		}
		bands, source, err := blueprints.VersionBands(tx, versionID)
		if err != nil {
			return b, err
		}
		rules = AnnualRulesSnapshot{Format: 1, Scope: ScopeAnnual, SessionID: sessionID, BlueprintVersionID: versionID, Version: version, Legacy: legacy,
			Annual: blueprints.AnnualRules{Method: d.Annual.Method, FinalMustPass: d.Annual.FinalMustPass, WholeMarks: d.Policy.WholeMarks,
				GPARounding: d.Policy.GPARounding, Ranking: d.Policy.Ranking, Terms: terms}, Bands: bands, BandsSource: source}
	}
	b.Rules = rules
	b.BlueprintVersionID = &rules.BlueprintVersionID
	if len(rules.Annual.Terms) == 0 {
		b.Blockers = append(b.Blockers, ReviewBlocker{Code: "no_terms"})
		return b, nil
	}
	current := make([]map[string]versionRow, len(rules.Annual.Terms))
	for i, t := range rules.Annual.Terms {
		p, err := latestPublished(tx, ScopeTerm, t.ID, classID)
		if err != nil {
			return b, err
		}
		if p == nil {
			b.Blockers = append(b.Blockers, ReviewBlocker{Code: "term_not_published", Detail: t.Name})
		}
		if current[i], err = currentVersions(tx, ScopeTerm, t.ID, classID); err != nil {
			return b, err
		}
		for _, v := range current[i] {
			b.Group = append(b.Group, v.ID)
		}
	}
	students := []struct {
		ID, Name, NameBN, AdmissionNo, SectionID, SectionName string
		RollNo                                                *int
	}{}
	if err := tx.Raw(`SELECT st.id, st.name_en AS name, coalesce(st.name_bn, '') AS name_bn, st.admission_no, st.roll_no, st.section_id, sec.name AS section_name
  FROM students st JOIN academic_sections sec ON sec.id = st.section_id
 WHERE st.session_id = ? AND st.class_id = ? AND st.status = 'active'
 ORDER BY sec.display_order, sec.name, st.roll_no NULLS LAST, st.admission_no, st.id`, sessionID, classID).Scan(&students).Error; err != nil {
		return b, err
	}
	sessionName, className, err := sessionClassNames(tx, sessionID, classID)
	if err != nil {
		return b, err
	}
	in := make([]blueprints.AnnualStudent, 0, len(students))
	inputs := make([]AnnualInputs, 0, len(students))
	for _, st := range students {
		as := blueprints.AnnualStudent{ID: st.ID, SectionID: st.SectionID}
		ai := AnnualInputs{Terms: []blueprints.AnnualTermInput{}}
		for i, t := range rules.Annual.Terms {
			ti := blueprints.AnnualTermInput{TermID: t.ID, Lines: []blueprints.AnnualLineInput{}}
			if v, ok := current[i][st.ID]; ok {
				snap, err := v.snapshot()
				if err != nil {
					return b, err
				}
				ti.VersionID, ti.Version, ti.Outcome = v.ID, v.Version, v.Outcome
				for _, l := range snap.Outputs.Subjects {
					line := blueprints.AnnualLineInput{Key: l.Key, SubjectIDs: l.SubjectIDs, CombinedKey: l.Combined, Optional: l.Optional, Status: l.Status, Full: l.Full, Pass: l.Pass}
					if l.Marks != nil {
						line.Marks = *l.Marks
					}
					if l.Passed != nil {
						line.Passed = *l.Passed
					}
					ti.Lines = append(ti.Lines, line)
				}
			}
			as.Terms = append(as.Terms, ti)
			ai.Terms = append(ai.Terms, ti)
		}
		in = append(in, as)
		inputs = append(inputs, ai)
	}
	scheme, err := blueprints.SchemeOf(rules.Bands)
	if err != nil {
		return b, ErrInvalid
	}
	results, err := blueprints.AnnualResults(rules.Annual, in, scheme)
	if err != nil {
		return b, ErrInvalid
	}
	subjects := map[string]blueprints.SubjectRef{}
	refs := []blueprints.SubjectRef{}
	if err := tx.Raw(`SELECT s.id, s.code, s.name_en, s.name_bn, acs.is_fourth_subject AS fourth FROM academic_class_subjects acs JOIN academic_subjects s ON s.id = acs.subject_id WHERE acs.class_id = ?`, classID).Scan(&refs).Error; err != nil {
		return b, err
	}
	for _, s := range refs {
		subjects[s.ID] = s
	}
	ctx := SnapshotContext{Method: rules.Annual.Method}
	for i, st := range students {
		c := candidate{StudentID: st.ID, Context: ctx, Inputs: inputs[i], Outputs: outputsOf(results[i], subjects),
			Student: SnapshotStudent{ID: st.ID, NameEN: st.Name, NameBN: st.NameBN, AdmissionNo: st.AdmissionNo, RollNo: st.RollNo, SectionID: st.SectionID,
				SectionName: st.SectionName, ClassID: classID, ClassName: className, SessionID: sessionID, SessionName: sessionName}}
		if results[i].Outcome == "incomplete" {
			c.Blocker = "incomplete"
		}
		b.Candidates = append(b.Candidates, c)
	}
	sort.Strings(b.Group)
	return b, nil
}

// storedTermRules and storedAnnualRules read the rules of a scope's first
// publication (since its last withdrawal), if it was published.
func storedTermRules(tx *gorm.DB, termID, classID string) (*TermRules, error) {
	p, err := rulesPublication(tx, ScopeTerm, termID, classID)
	if err != nil || p == nil {
		return nil, err
	}
	var r TermRules
	return &r, json.Unmarshal([]byte(p.Rules), &r)
}

func storedAnnualRules(tx *gorm.DB, sessionID, classID string) (*AnnualRulesSnapshot, error) {
	p, err := rulesPublication(tx, ScopeAnnual, sessionID, classID)
	if err != nil || p == nil {
		return nil, err
	}
	var r AnnualRulesSnapshot
	return &r, json.Unmarshal([]byte(p.Rules), &r)
}

// Recompute reproduces a version's outputs from its snapshot inputs and the
// rules of its publication (positions, which depend on the whole class, are
// checked per publication, not here).
func recompute(scope string, snap Snapshot, rules string, sectionID string) (Outputs, error) {
	switch scope {
	case ScopeAssessment:
		var r AssessmentRules
		var in AssessmentInputs
		if err := json.Unmarshal([]byte(rules), &r); err != nil {
			return Outputs{}, err
		}
		if err := json.Unmarshal(snap.Inputs, &in); err != nil {
			return Outputs{}, err
		}
		bands, policy, _ := r.engine()
		lines := make([]MarkLine, 0, len(in.Lines))
		for _, l := range in.Lines {
			lines = append(lines, inLineToMark(l))
		}
		c := candidate{}
		if err := scoreAssessment(&c, in, lines, bands, policy, r); err != nil {
			return Outputs{}, err
		}
		if c.Blocker != "" {
			return Outputs{Outcome: "incomplete"}, nil
		}
		return c.Outputs, nil
	case ScopeTerm:
		var r TermRules
		var in TermInputs
		if err := json.Unmarshal([]byte(rules), &r); err != nil {
			return Outputs{}, err
		}
		if err := json.Unmarshal(snap.Inputs, &in); err != nil {
			return Outputs{}, err
		}
		scheme, err := blueprints.SchemeOf(r.Bands)
		if err != nil {
			return Outputs{}, err
		}
		res, err := blueprints.TermResults(r.Definition, r.TermID, []blueprints.Student{studentOfTermInputs(snap.Student.ID, sectionID, in)}, scheme)
		if err != nil {
			return Outputs{}, err
		}
		return outputsOf(res[0], namesOf(snap.Outputs)), nil
	case ScopeAnnual:
		var r AnnualRulesSnapshot
		var in AnnualInputs
		if err := json.Unmarshal([]byte(rules), &r); err != nil {
			return Outputs{}, err
		}
		if err := json.Unmarshal(snap.Inputs, &in); err != nil {
			return Outputs{}, err
		}
		scheme, err := blueprints.SchemeOf(r.Bands)
		if err != nil {
			return Outputs{}, err
		}
		res, err := blueprints.AnnualResults(r.Annual, []blueprints.AnnualStudent{{ID: snap.Student.ID, SectionID: sectionID, Terms: in.Terms}}, scheme)
		if err != nil {
			return Outputs{}, err
		}
		return outputsOf(res[0], namesOf(snap.Outputs)), nil
	}
	return Outputs{}, ErrInvalid
}

// namesOf recovers subject names from published outputs, for recomputation.
func namesOf(o Outputs) map[string]blueprints.SubjectRef {
	m := map[string]blueprints.SubjectRef{}
	for _, l := range o.Subjects {
		ens, bns := strings.Split(l.NameEN, " + "), strings.Split(l.NameBN, " + ")
		for i, id := range l.SubjectIDs {
			ref := blueprints.SubjectRef{ID: id}
			if i < len(ens) {
				ref.NameEN = ens[i]
			}
			if i < len(bns) {
				ref.NameBN = bns[i]
			}
			if i == 0 {
				ref.Code = l.Code
			}
			m[id] = ref
		}
	}
	return m
}

// withoutPositions drops what depends on the whole class.
func withoutPositions(o Outputs) Outputs {
	o.ClassPosition, o.SectionPosition = nil, nil
	return o
}
