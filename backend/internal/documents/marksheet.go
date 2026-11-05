package documents

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// The data a marksheet template reads (templates/marksheet.typ).

type schoolData struct {
	Name      string `json:"name"`
	NameAlt   string `json:"name_alt"`
	Contact   string `json:"contact"`
	EIIN      string `json:"eiin"` // "EIIN: …" or ""
	Accent    string `json:"accent"`
	Initials  string `json:"initials"`
	Logo      string `json:"logo"`      // asset name or ""
	Signature string `json:"signature"` // asset name or ""
	Signatory string `json:"signatory"`
}

type gradeRow struct {
	Range string `json:"range"`
	Grade string `json:"grade"`
	Point string `json:"point"`
}

type sheetRow struct {
	Code     string   `json:"code"`
	Subject  string   `json:"subject"`
	Optional bool     `json:"optional"`
	Note     string   `json:"note"`
	Parts    []string `json:"parts"`
	CT       string   `json:"ct"`
	Total    string   `json:"total"`
	Full     string   `json:"full"`
	Highest  string   `json:"highest"`
	Grade    string   `json:"grade"`
	GP       string   `json:"gp"`
	Failed   bool     `json:"failed"`
}

type sheetStudent struct {
	Name       string     `json:"name"`
	NameAlt    string     `json:"name_alt"`
	StudentID  string     `json:"student_id"`
	Class      string     `json:"class"`
	Section    string     `json:"section"`
	Roll       string     `json:"roll"`
	Session    string     `json:"session"`
	Exam       string     `json:"exam"`
	Photo      string     `json:"photo"`
	QR         string     `json:"qr"`
	Withheld   bool       `json:"withheld"`
	Rows       []sheetRow `json:"rows"`
	Total      string     `json:"total"`
	GPA        string     `json:"gpa"`
	GPAWithout string     `json:"gpa_without"`
	Letter     string     `json:"letter"`
	Result     string     `json:"result"`
	Failed     bool       `json:"failed"`
	ClassPos   string     `json:"class_pos"`
	SectionPos string     `json:"section_pos"`
	Published  string     `json:"published"`
	Footer     string     `json:"footer"`
}

type sheetData struct {
	Lang        string            `json:"lang"`
	Title       string            `json:"title"`
	Heading     string            `json:"heading"`
	ExamName    string            `json:"exam_name"`
	Provisional bool              `json:"provisional"`
	Labels      map[string]string `json:"labels"`
	School      schoolData        `json:"school"`
	Columns     []string          `json:"columns"`
	ShowCT      bool              `json:"show_ct"`
	Grading     []gradeRow        `json:"grading"`
	Students    []sheetStudent    `json:"students"`
}

// layout is how a set's subject lines are broken down into columns.
type layout struct {
	scope     string
	examKeys  map[string]bool // term exams of the term (their components are shown)
	compKeys  []string        // component keys, in order
	showCT    bool            // a contribution from class tests exists
	termIDs   []string        // annual: the terms, in order
	termNames []string
	bands     []blueprints.BandRef
	rounding  calc.Rounding
}

func layoutOf(set exams.DocSet) layout {
	lo := layout{scope: set.Scope, examKeys: map[string]bool{}}
	switch {
	case set.Term != nil:
		def := set.Term.Definition
		lo.bands = set.Term.Bands
		if def.Policy.GPARounding == "truncate" {
			lo.rounding = calc.Truncate
		}
		others := map[string]bool{}
		for _, a := range def.Assessments {
			if a.TermID != set.ScopeID {
				continue
			}
			if a.Kind == "term_exam" {
				lo.examKeys[a.Key] = true
			} else {
				others[a.Key] = true
			}
		}
		seen := map[string]bool{}
		for _, s := range def.Subjects {
			for _, sh := range s.Sheets {
				if !lo.examKeys[sh.Assessment] {
					continue
				}
				for _, c := range sh.Components {
					if !seen[c.Key] {
						seen[c.Key] = true
						lo.compKeys = append(lo.compKeys, c.Key)
					}
				}
			}
			for _, tr := range s.Terms {
				if tr.TermID != set.ScopeID {
					continue
				}
				for _, c := range tr.Contributions {
					for _, f := range c.From {
						if others[f] {
							lo.showCT = true
						}
					}
				}
			}
		}
		lo.compKeys = orderComponents(lo.compKeys)
	case set.Annual != nil:
		lo.bands = set.Annual.Bands
		if set.Annual.Annual.GPARounding == "truncate" {
			lo.rounding = calc.Truncate
		}
		for _, t := range set.Annual.Annual.Terms {
			lo.termIDs = append(lo.termIDs, t.ID)
			name := t.Name
			if set.Annual.Annual.Method == "weighted" && t.Weight > 0 {
				name += fmt.Sprintf(" (%d%%)", t.Weight)
			}
			lo.termNames = append(lo.termNames, name)
		}
	}
	return lo
}

// orderComponents puts the usual components first: written, CQ, MCQ,
// practical, then any others as they came.
func orderComponents(keys []string) []string {
	rank := map[string]int{"TOTAL": 0, "WRITTEN": 1, "CQ": 2, "MCQ": 3, "PRAC": 4, "PRACTICAL": 4}
	out := append([]string{}, keys...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, iok := rank[strings.ToUpper(out[i])]
		rj, jok := rank[strings.ToUpper(out[j])]
		if !iok {
			ri = 10
		}
		if !jok {
			rj = 10
		}
		return ri < rj
	})
	return out
}

func (lo layout) columns(l lang) []string {
	if lo.scope == exams.ScopeAnnual {
		return append([]string{}, lo.termNames...)
	}
	out := make([]string, 0, len(lo.compKeys))
	for _, k := range lo.compKeys {
		out = append(out, l.componentLabel(k))
	}
	return out
}

// highest is each subject line's highest mark in the set (graded lines).
func highest(set exams.DocSet) map[string]float64 {
	out := map[string]float64{}
	for _, r := range set.Results {
		for _, line := range r.Outputs.Subjects {
			if line.Status != "graded" || line.Marks == nil {
				continue
			}
			v, err := strconv.ParseFloat(*line.Marks, 64)
			if err != nil {
				continue
			}
			if cur, ok := out[line.Key]; !ok || v > cur {
				out[line.Key] = v
			}
		}
	}
	return out
}

func markText(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	return s
}

// rows breaks a result's subject lines into the table's rows.
func (lo layout) rows(l lang, r exams.DocResult, top map[string]float64) []sheetRow {
	var inputs exams.TermInputs
	if lo.scope == exams.ScopeTerm {
		inputs, _ = exams.TermInputsOf(r.Inputs)
	}
	rows := make([]sheetRow, 0, len(r.Outputs.Subjects))
	for _, line := range r.Outputs.Subjects {
		row := sheetRow{Code: l.num(line.Code), Subject: l.pick(line.NameEN, line.NameBN), Optional: line.Optional, CT: "—", Full: l.marks(&line.Full), Highest: "—", Grade: "—", GP: "—"}
		switch line.Status {
		case "graded":
			row.Total = l.marks(line.Marks)
		case "exempt":
			row.Total, row.Full = l.t("exempt"), ""
		default:
			row.Total, row.Full = l.t(line.Status), ""
			if row.Total == "" {
				row.Total = "—"
			}
		}
		if line.Grade != nil {
			row.Grade = *line.Grade
		}
		if line.Point != nil {
			row.GP = l.points(line.Point)
		}
		row.Failed = line.Passed != nil && !*line.Passed
		if v, ok := top[line.Key]; ok {
			s := markText(v)
			row.Highest = l.marks(&s)
		}
		switch lo.scope {
		case exams.ScopeTerm:
			row.Parts = make([]string, len(lo.compKeys))
			for i, key := range lo.compKeys {
				sum, any := 0.0, false
				for _, sid := range line.SubjectIDs {
					for ak := range lo.examKeys {
						m, ok := inputs.Marks[ak][sid]
						if !ok {
							continue
						}
						for _, c := range m.Components {
							if c.Key == key && c.Marks != nil {
								if v, err := strconv.ParseFloat(*c.Marks, 64); err == nil {
									sum, any = sum+v, true
								}
							}
						}
						// An ordinary paper keeps its mark without components.
						if strings.EqualFold(key, "TOTAL") && len(m.Components) == 0 && m.Marks != nil {
							if v, err := strconv.ParseFloat(*m.Marks, 64); err == nil {
								sum, any = sum+v, true
							}
						}
						if m.Status == "absent" && len(m.Components) == 0 && i == 0 {
							row.Note = l.t("absent")
						}
					}
				}
				row.Parts[i] = "—"
				if any {
					s := markText(sum)
					row.Parts[i] = l.marks(&s)
				}
			}
			ct, any := 0.0, false
			for _, c := range line.Contributions {
				exam := false
				for _, f := range c.From {
					if lo.examKeys[f] {
						exam = true
					}
				}
				if exam || c.Value == "" {
					continue
				}
				if v, err := strconv.ParseFloat(c.Value, 64); err == nil {
					ct, any = ct+v, true
				}
			}
			if any {
				s := markText(ct)
				row.CT = l.marks(&s)
			}
		case exams.ScopeAnnual:
			row.Parts = make([]string, len(lo.termIDs))
			for i, id := range lo.termIDs {
				row.Parts[i] = "—"
				for _, t := range line.Terms {
					if t.TermID == id && t.Marks != "" {
						m := t.Marks
						row.Parts[i] = l.marks(&m)
					}
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// requiredGPA is the GPA without the fourth subject, by the same rounding.
func (lo layout) requiredGPA(o exams.Outputs) *string {
	if o.Outcome != "passed" && o.Outcome != "failed" {
		return nil
	}
	points := []calc.Points{}
	for _, line := range o.Subjects {
		if line.Optional || line.Status != "graded" || line.Point == nil {
			continue
		}
		v, err := strconv.ParseFloat(*line.Point, 64)
		if err != nil {
			return nil
		}
		p, err := calc.PointsOf(v)
		if err != nil {
			return nil
		}
		points = append(points, p)
	}
	gpa, err := calc.RequiredGPA(points, o.Outcome == "passed", calc.Policy{GPA: lo.rounding})
	if err != nil {
		return nil
	}
	s := gpa.String()
	return &s
}

// letter is the overall grade: the band whose point the GPA reaches.
func (lo layout) letter(o exams.Outputs) string {
	if o.Outcome == "failed" {
		for _, b := range lo.bands {
			if b.GPA == 0 {
				return b.Grade
			}
		}
		return "F"
	}
	if o.GPA == nil {
		return "—"
	}
	gpa, err := strconv.ParseFloat(*o.GPA, 64)
	if err != nil {
		return "—"
	}
	best, grade := -1.0, "—"
	for _, b := range lo.bands {
		if b.GPA <= gpa+1e-9 && b.GPA > best {
			best, grade = b.GPA, b.Grade
		}
	}
	return grade
}

func (lo layout) grading(l lang) []gradeRow {
	bands := append([]blueprints.BandRef{}, lo.bands...)
	sort.Slice(bands, func(i, j int) bool { return bands[i].Min > bands[j].Min })
	out := make([]gradeRow, 0, len(bands))
	for _, b := range bands {
		out = append(out, gradeRow{Range: l.int(b.Min) + "–" + l.int(b.Max), Grade: b.Grade, Point: l.num(strconv.FormatFloat(b.GPA, 'f', 2, 64))})
	}
	return out
}

// outcomeText is a result's outcome in words.
func outcomeText(l lang, outcome string) string {
	switch outcome {
	case "passed", "failed", "withheld":
		return l.t(outcome)
	}
	return l.t("incomplete")
}

func posText(l lang, p *int) string {
	if p == nil || *p <= 0 {
		return "—"
	}
	return l.int(*p)
}

func rollText(l lang, roll *int) string {
	if roll == nil {
		return "—"
	}
	return l.int(*roll)
}
