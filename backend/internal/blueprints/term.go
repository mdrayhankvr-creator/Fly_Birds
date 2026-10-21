package blueprints

import (
	"errors"
	"math/big"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// ErrInvalidInput reports marks the term calculation cannot use (more than
// two decimals, marks above full marks, a definition that was not validated).
var ErrInvalidInput = errors.New("invalid term result input")

// Mark is one student's mark on one sheet, with the sheet's own full and
// pass marks (each exam stores them, so legacy sections may differ).
type Mark struct {
	Status     string // present, absent, exempt, incomplete or withheld
	Marks      *float64
	Full       float64
	Pass       float64
	Components []ComponentMark
}

// ComponentMark is a component of a present mark.
type ComponentMark struct {
	Key   string
	Marks float64
	Full  float64
	Pass  *float64
}

// Student is one student's input to a term result.
type Student struct {
	ID        string
	SectionID string
	// Marks by assessment key, then subject id. A missing entry is a missing
	// mark (the sheet does not exist or has no row for the student).
	Marks map[string]map[string]Mark
	// Takes reports whether the student takes a subject (optional subjects
	// they did not choose are not taken).
	Takes func(subjectID string) bool
}

// Reason is why a subject fails whatever its total: a component below its
// pass mark, or (each-paper rule) a paper below its pass mark.
type Reason struct {
	Code       string `json:"code"` // component_below_pass or paper_below_pass
	SubjectID  string `json:"subject_id"`
	Assessment string `json:"assessment,omitempty"`
	Component  string `json:"component,omitempty"`
}

// ContributionLine is one contribution's exact value, for the explanation.
type ContributionLine struct {
	From    []string `json:"from"`
	Combine string   `json:"combine"`
	Worth   string   `json:"worth"`
	Value   string   `json:"value"` // two decimals for display; "" when exempt or blocked
	Exempt  bool     `json:"exempt"`
}

// SubjectResult is one graded line of a term result: a subject, or papers
// combined under one key.
type SubjectResult struct {
	SubjectIDs    []string           `json:"subject_ids"`
	CombinedKey   string             `json:"combined_key,omitempty"`
	Optional      bool               `json:"optional"`
	Status        string             `json:"status"` // graded, exempt, incomplete or withheld
	Marks         string             `json:"marks"`
	Full          string             `json:"full"`
	Grade         string             `json:"grade"`
	Point         string             `json:"point"`
	Passed        bool               `json:"passed"`
	Reasons       []Reason           `json:"reasons"`
	Contributions []ContributionLine `json:"contributions"`
}

// StudentResult is a student's term result.
type StudentResult struct {
	StudentID       string          `json:"student_id"`
	SectionID       string          `json:"section_id"`
	Outcome         string          `json:"outcome"` // passed, failed, incomplete or withheld
	GPA             string          `json:"gpa"`
	Total           string          `json:"total"`
	Full            string          `json:"full"`
	SectionPosition int             `json:"section_position"`
	ClassPosition   int             `json:"class_position"`
	Subjects        []SubjectResult `json:"subjects"`
}

// line is one subject's term mark before GPA.
type line struct {
	subject  SubjectRule
	status   calc.Status // Present, Exempt, Incomplete or Withheld
	marks    calc.Marks
	full     calc.Marks
	pass     calc.Marks
	reasons  []Reason
	contribs []ContributionLine
	optional bool
	papers   []string
	combined string
}

// TermResults computes the term results of a class: every student's subject
// lines, GPA and outcome, then section and class positions. Results that are
// incomplete or withheld are not ranked.
func TermResults(d Definition, termID string, students []Student, scheme calc.Scheme) ([]StudentResult, error) {
	out := make([]StudentResult, 0, len(students))
	ranked := []calc.Ranked{}
	sectionOf := map[string]string{}
	for _, st := range students {
		r, key, err := studentTerm(d, termID, st, scheme)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
		if r.Outcome == "passed" || r.Outcome == "failed" {
			ranked = append(ranked, key)
			sectionOf[r.StudentID] = r.SectionID
		}
	}
	mode := calc.Competition
	if d.Policy.Ranking == "dense" {
		mode = calc.Dense
	}
	class := calc.Rank(ranked, mode)
	bySection := map[string][]calc.Ranked{}
	for _, e := range ranked {
		bySection[sectionOf[e.Key]] = append(bySection[sectionOf[e.Key]], e)
	}
	section := map[string]int{}
	for _, entries := range bySection {
		for k, p := range calc.Rank(entries, mode) {
			section[k] = p
		}
	}
	for i := range out {
		out[i].ClassPosition = class[out[i].StudentID]
		out[i].SectionPosition = section[out[i].StudentID]
	}
	return out, nil
}

// studentTerm computes one student's term result and their merit key.
func studentTerm(d Definition, termID string, st Student, scheme calc.Scheme) (StudentResult, calc.Ranked, error) {
	none := calc.Ranked{}
	policy := calc.TermPolicy{RoundContributions: d.Policy.RoundContributions, WholeMarks: d.Policy.WholeMarks}
	if d.Policy.Absent == "missing" {
		policy.Absent = calc.AbsentIsMissing
	}
	lines := []*line{}
	for _, s := range d.Subjects {
		if s.Role == "non_graded" || (st.Takes != nil && !st.Takes(s.SubjectID)) {
			continue
		}
		rule, ok := s.Term(termID)
		if !ok {
			continue
		}
		l, err := subjectTerm(d, s, rule, st, policy)
		if err != nil {
			return StudentResult{}, none, err
		}
		lines = append(lines, l)
	}
	lines, err := combine(lines, d.PaperRule)
	if err != nil {
		return StudentResult{}, none, err
	}
	res := StudentResult{StudentID: st.ID, SectionID: st.SectionID, Subjects: []SubjectResult{}}
	in := []calc.Line{}
	graded := []int{}
	for i, l := range lines {
		sr := SubjectResult{SubjectIDs: l.papers, CombinedKey: l.combined, Optional: l.optional, Reasons: l.reasons, Contributions: l.contribs}
		if sr.Reasons == nil {
			sr.Reasons = []Reason{}
		}
		if sr.Contributions == nil {
			sr.Contributions = []ContributionLine{}
		}
		switch l.status {
		case calc.Exempt:
			sr.Status = "exempt"
		case calc.Incomplete:
			sr.Status = "incomplete"
		case calc.Withheld:
			sr.Status = "withheld"
		default:
			sr.Status = "graded"
			sr.Marks, sr.Full = l.marks.String(), l.full.String()
			in = append(in, calc.Line{Marks: l.marks, Full: l.full, Pass: l.pass, Optional: l.optional, Failed: len(l.reasons) > 0})
			graded = append(graded, i)
		}
		res.Subjects = append(res.Subjects, sr)
	}
	// Blocked lines block the result exactly as statuses do.
	for _, l := range lines {
		if l.status == calc.Withheld || l.status == calc.Incomplete {
			in = append(in, calc.Line{Status: l.status})
		}
	}
	gpaRounding := calc.HalfUp
	if d.Policy.GPARounding == "truncate" {
		gpaRounding = calc.Truncate
	}
	// Without a graded required subject there is no GPA to compute: the
	// result waits (or is held back) instead of failing the whole class.
	required := false
	for _, i := range graded {
		required = required || !lines[i].optional
	}
	if !required {
		res.Outcome = "incomplete"
		for _, l := range lines {
			if l.status == calc.Withheld {
				res.Outcome = "withheld"
			}
		}
		return res, none, nil
	}
	r, err := calc.CalculateWithStatuses(in, scheme, calc.Policy{GPA: gpaRounding}, calc.AbsentScoresZero)
	if err != nil {
		if errors.Is(err, calc.ErrInvalid) {
			return StudentResult{}, none, ErrInvalidInput
		}
		return StudentResult{}, none, err
	}
	switch r.Outcome {
	case calc.OutcomeWithheld:
		res.Outcome = "withheld"
		return res, none, nil
	case calc.OutcomeIncomplete:
		res.Outcome = "incomplete"
		return res, none, nil
	case calc.Passed:
		res.Outcome = "passed"
	default:
		res.Outcome = "failed"
	}
	res.GPA, res.Total, res.Full = r.Result.GPA.String(), r.Result.Total.String(), r.Result.Full.String()
	for k, i := range graded {
		g := r.Result.Grades[k]
		res.Subjects[i].Grade, res.Subjects[i].Point, res.Subjects[i].Passed = g.Grade, g.Point.String(), g.Passed
	}
	return res, calc.Ranked{Key: st.ID, GPA: r.Result.GPA, Total: r.Result.Total, Passed: r.Outcome == calc.Passed}, nil
}

// subjectTerm computes one subject's term mark from its contributions.
func subjectTerm(d Definition, s SubjectRule, rule TermRule, st Student, policy calc.TermPolicy) (*line, error) {
	ruleFull, err1 := calc.MarksOf(rule.Full)
	rulePass, err2 := calc.MarksOf(rule.Pass)
	if err1 != nil || err2 != nil || ruleFull <= 0 || rulePass > ruleFull {
		return nil, ErrInvalidInput
	}
	l := &line{subject: s, optional: s.Role == "optional", papers: []string{s.SubjectID}, combined: s.CombinedKey}
	cs := make([]calc.Contribution, 0, len(rule.Contributions))
	for _, c := range rule.Contributions {
		worth, err := calc.MarksOf(c.Worth)
		if err != nil {
			return nil, ErrInvalidInput
		}
		cc := calc.Contribution{Worth: worth, N: c.Best}
		switch c.Combine {
		case "single":
			cc.Combine = calc.Single
		case "sum":
			cc.Combine = calc.Sum
		case "average":
			cc.Combine = calc.Average
		case "best":
			cc.Combine = calc.BestN
		case "latest":
			cc.Combine = calc.Latest
		default:
			return nil, ErrInvalidInput
		}
		for _, key := range c.From {
			score, reasons, err := source(s, key, st)
			if err != nil {
				return nil, err
			}
			l.reasons = append(l.reasons, reasons...)
			cc.Sources = append(cc.Sources, score)
		}
		cs = append(cs, cc)
		cr, err := calc.Contribute(cc, policy.Absent)
		if err != nil {
			return nil, ErrInvalidInput
		}
		cl := ContributionLine{From: c.From, Combine: c.Combine, Worth: worth.String(), Exempt: cr.Exempt}
		if cr.Value != nil {
			cl.Value = calc.RoundMarks(cr.Value, calc.HalfUp).String()
		}
		l.contribs = append(l.contribs, cl)
	}
	marks, full, blocked, err := calc.TermMark(cs, policy)
	if err != nil {
		return nil, ErrInvalidInput
	}
	switch {
	case blocked != calc.Present:
		l.status = blocked
	case full == 0:
		l.status = calc.Exempt // every contribution exempt: the subject leaves the result
	default:
		l.status = calc.Present
		l.marks, l.full = marks, full
		l.pass = scaledPass(rulePass, ruleFull, full)
	}
	return l, nil
}

// source turns one assessment's sheet mark into a calc score. A component
// below its pass mark is a reason the subject fails.
func source(s SubjectRule, key string, st Student) (calc.Score, []Reason, error) {
	m, ok := st.Marks[key][s.SubjectID]
	if !ok || m.Status == "" {
		return calc.Score{Full: 1, Status: calc.Incomplete}, nil, nil // missing: the result waits for it
	}
	full, err := calc.MarksOf(m.Full)
	if err != nil || full <= 0 {
		return calc.Score{}, nil, ErrInvalidInput
	}
	score := calc.Score{Full: full}
	switch m.Status {
	case "present":
		if m.Marks == nil {
			return calc.Score{Full: full, Status: calc.Incomplete}, nil, nil
		}
		marks, err := calc.MarksOf(*m.Marks)
		if err != nil || marks > full {
			return calc.Score{}, nil, ErrInvalidInput
		}
		score.Marks = marks
	case "absent":
		score.Status = calc.Absent
	case "exempt":
		score.Status = calc.Exempt
	case "incomplete":
		score.Status = calc.Incomplete
	case "withheld":
		score.Status = calc.Withheld
	default:
		return calc.Score{}, nil, ErrInvalidInput
	}
	reasons := []Reason{}
	if score.Status == calc.Present {
		for _, c := range m.Components {
			if c.Pass == nil {
				continue
			}
			got, err1 := calc.MarksOf(c.Marks)
			pass, err2 := calc.MarksOf(*c.Pass)
			if err1 != nil || err2 != nil {
				return calc.Score{}, nil, ErrInvalidInput
			}
			if got < pass {
				reasons = append(reasons, Reason{Code: "component_below_pass", SubjectID: s.SubjectID, Assessment: key, Component: c.Key})
			}
		}
	}
	return score, reasons, nil
}

// scaledPass is the pass mark on the full marks that remain after exempt
// contributions left: pass × full / ruleFull, rounded up to two decimals so
// that a two-decimal mark reaches it exactly when it reaches the true value.
func scaledPass(pass, ruleFull, full calc.Marks) calc.Marks {
	if full == ruleFull {
		return pass
	}
	num := new(big.Int).Mul(big.NewInt(int64(pass)), big.NewInt(int64(full)))
	den := big.NewInt(int64(ruleFull))
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return calc.Marks(q.Int64())
}

// combine grades papers that share a combined key as one subject. A paper
// that is blocked blocks the combined subject; exempt papers leave it.
func combine(lines []*line, rule string) ([]*line, error) {
	out := []*line{}
	groups := map[string][]*line{}
	order := []string{}
	for _, l := range lines {
		if l.combined == "" {
			out = append(out, l)
			continue
		}
		if _, ok := groups[l.combined]; !ok {
			order = append(order, l.combined)
			out = append(out, nil) // placeholder keeps display order
		}
		groups[l.combined] = append(groups[l.combined], l)
	}
	paperRule := calc.CombinedPass
	if rule == "each_paper" {
		paperRule = calc.EachPaperPass
	}
	k := 0
	for i, l := range out {
		if l != nil {
			continue
		}
		key := order[k]
		k++
		papers := groups[key]
		c := &line{combined: key, optional: papers[0].optional, status: calc.Present}
		var present []*line
		for _, p := range papers {
			c.papers = append(c.papers, p.papers...)
			c.contribs = append(c.contribs, p.contribs...)
			c.reasons = append(c.reasons, p.reasons...)
			switch p.status {
			case calc.Withheld:
				c.status = calc.Withheld
			case calc.Incomplete:
				if c.status != calc.Withheld {
					c.status = calc.Incomplete
				}
			case calc.Present:
				present = append(present, p)
			}
		}
		switch {
		case c.status != calc.Present:
		case len(present) == 0:
			c.status = calc.Exempt
		default:
			for _, p := range present {
				c.marks += p.marks
				c.full += p.full
				c.pass += p.pass
				if paperRule == calc.EachPaperPass && p.marks < p.pass {
					c.reasons = append(c.reasons, Reason{Code: "paper_below_pass", SubjectID: p.papers[0]})
				}
			}
		}
		out[i] = c
	}
	return out, nil
}
