// Package blueprints describes how a school assesses a class in a session:
// its assessments (class tests, term exams …), each subject's sheets and
// components, how assessments combine into a term subject mark, and the
// school's rounding, ranking and annual policies. A definition is data,
// validated before activation and frozen afterwards; the arithmetic is done
// by the exact engine in internal/results/calc.
package blueprints

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// Definition is the full, versioned design of one blueprint version.
type Definition struct {
	Assessments []Assessment  `json:"assessments"`
	Subjects    []SubjectRule `json:"subjects"`
	// PaperRule is how combined papers pass: "combined" (their combined mark
	// reaches the combined pass mark) or "each_paper" (every paper too).
	PaperRule string `json:"paper_rule"`
	Annual    Annual `json:"annual"`
	Policy    Policy `json:"policy"`
}

// Assessment is something marks are recorded for, in one term.
type Assessment struct {
	Key    string `json:"key"` // CT1, TERM1 …
	Name   string `json:"name"`
	TermID string `json:"term_id"`
	// Kind: class_test, continuous, performance or term_exam.
	Kind string `json:"kind"`
	// Visibility of this assessment's own results to students and guardians:
	// hidden, after_term_publication or with_assessment_publication.
	Visibility string `json:"visibility"`
}

// SubjectRule is how one subject is assessed and combined.
type SubjectRule struct {
	SubjectID string `json:"subject_id"`
	// Role: required, optional (a fourth subject a student may choose) or
	// non_graded (not assessed by this blueprint).
	Role        string     `json:"role"`
	CombinedKey string     `json:"combined_key,omitempty"` // papers graded together share a key
	Sheets      []Sheet    `json:"sheets"`
	Terms       []TermRule `json:"terms"`
}

// Sheet is the subject's mark sheet in one assessment: its components and
// the subject pass mark for that sheet.
type Sheet struct {
	Assessment string      `json:"assessment"`
	Pass       float64     `json:"pass"`
	Components []Component `json:"components"`
}

// Component is a separately marked part (CQ, MCQ, PRAC …). A sheet with one
// TOTAL component is an ordinary sheet.
type Component struct {
	Key  string   `json:"key"`
	Full float64  `json:"full"`
	Pass *float64 `json:"pass,omitempty"` // a component pass mark, if any
}

// TermRule is how the subject's term mark is made: contributions scaled and
// summed to the term full marks.
type TermRule struct {
	TermID        string         `json:"term_id"`
	Full          float64        `json:"full"`
	Pass          float64        `json:"pass"`
	Contributions []Contribution `json:"contributions"`
}

// Contribution draws from assessments of the term, combines them and is
// worth a number of marks.
type Contribution struct {
	From []string `json:"from"`
	// Combine: single, sum, average, best or latest.
	Combine string  `json:"combine"`
	Best    int     `json:"best,omitempty"` // N for "best"
	Worth   float64 `json:"worth"`
}

// Annual is the annual result method. It is validated and stored in E5;
// annual results are computed and published from E6.
type Annual struct {
	Method        string         `json:"method"`            // final_only, equal or weighted
	Weights       map[string]int `json:"weights,omitempty"` // term id → percent
	FinalMustPass bool           `json:"final_must_pass"`
}

// Policy holds the school's rounding, absence and ranking choices. The zero
// value of each choice is today's behaviour.
type Policy struct {
	RoundContributions bool   `json:"round_contributions"` // R1
	WholeMarks         bool   `json:"whole_marks"`         // R3
	GPARounding        string `json:"gpa_rounding"`        // half_up or truncate (R4)
	Absent             string `json:"absent"`              // zero or missing
	Ranking            string `json:"ranking"`             // competition or dense
}

// Defaults fills empty policy choices with today's behaviour.
func (d *Definition) Defaults() {
	if d.PaperRule == "" {
		d.PaperRule = "combined"
	}
	if d.Annual.Method == "" {
		d.Annual.Method = "final_only"
	}
	if d.Policy.GPARounding == "" {
		d.Policy.GPARounding = "half_up"
	}
	if d.Policy.Absent == "" {
		d.Policy.Absent = "zero"
	}
	if d.Policy.Ranking == "" {
		d.Policy.Ranking = "competition"
	}
	if d.Assessments == nil {
		d.Assessments = []Assessment{}
	}
	if d.Subjects == nil {
		d.Subjects = []SubjectRule{}
	}
	for i := range d.Subjects {
		if d.Subjects[i].Sheets == nil {
			d.Subjects[i].Sheets = []Sheet{}
		}
		if d.Subjects[i].Terms == nil {
			d.Subjects[i].Terms = []TermRule{}
		}
	}
}

// Canonical is the definition's stable encoding: struct fields in declared
// order, map keys sorted (encoding/json), no whitespace.
func (d Definition) Canonical() ([]byte, error) { return json.Marshal(d) }

// SHA256 identifies a definition: the hex digest of its canonical encoding.
func (d Definition) SHA256() (string, error) {
	b, err := d.Canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// Assessment returns the assessment with a key.
func (d Definition) Assessment(key string) (Assessment, bool) {
	for _, a := range d.Assessments {
		if a.Key == key {
			return a, true
		}
	}
	return Assessment{}, false
}

// Sheet returns the subject's sheet in an assessment.
func (s SubjectRule) Sheet(assessment string) (Sheet, bool) {
	for _, sh := range s.Sheets {
		if sh.Assessment == assessment {
			return sh, true
		}
	}
	return Sheet{}, false
}

// Term returns the subject's rule for a term.
func (s SubjectRule) Term(termID string) (TermRule, bool) {
	for _, t := range s.Terms {
		if t.TermID == termID {
			return t, true
		}
	}
	return TermRule{}, false
}

// Full is the sheet's full marks: the exact sum of its components.
func (sh Sheet) Full() (calc.Marks, error) {
	var total calc.Marks
	for _, c := range sh.Components {
		m, err := calc.MarksOf(c.Full)
		if err != nil {
			return 0, err
		}
		total += m
	}
	return total, nil
}
