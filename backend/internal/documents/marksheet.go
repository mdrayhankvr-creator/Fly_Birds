package documents

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// The data a marksheet template reads (templates/marksheet.typ).

type schoolData struct {
	Name        string `json:"name"`
	NameAlt     string `json:"name_alt"`
	Contact     string `json:"contact"`
	EIIN        string `json:"eiin"`        // "EIIN: …" or ""
	Established string `json:"established"` // "Established 1985" or ""
	Accent      string `json:"accent"`
	Initials    string `json:"initials"`
	Logo        string `json:"logo"`      // asset name or ""
	Signature   string `json:"signature"` // asset name or ""
	Signatory   string `json:"signatory"`
	// Transcript v3 decorations (school settings, both on by default): the
	// certificate border and the faint logo watermark.
	Border    bool `json:"border"`
	Watermark bool `json:"watermark"`
}

type gradeRow struct {
	Range string `json:"range"`
	Grade string `json:"grade"`
	Point string `json:"point"`
}

// sheetCell is a mark and the full marks it is out of ("" when not shown).
// Failed marks a component below its own pass marks.
type sheetCell struct {
	Obt    string `json:"obt"`
	Full   string `json:"full"`
	Failed bool   `json:"failed"`
}

// paperRow is one paper of a combined subject (Bangla 1st and 2nd paper …).
type paperRow struct {
	Code    string      `json:"code"`
	Subject string      `json:"subject"`
	Parts   []sheetCell `json:"parts"`
}

type sheetRow struct {
	Code     string      `json:"code"`
	Subject  string      `json:"subject"`
	Optional bool        `json:"optional"`
	Note     string      `json:"note"`
	Parts    []sheetCell `json:"parts"`
	Papers   []paperRow  `json:"papers"` // two or more: a combined subject, one row per paper
	CT       string      `json:"ct"`
	Total    string      `json:"total"`
	Full     string      `json:"full"`
	Highest  string      `json:"highest"`
	Grade    string      `json:"grade"`
	GP       string      `json:"gp"`
	Failed   bool        `json:"failed"`
}

type signature struct {
	Title string `json:"title"`
	Name  string `json:"name"`
	Image string `json:"image"` // asset name or ""
	Date  bool   `json:"date"`  // a blank date line under it (guardian)
}

type sheetStudent struct {
	Name        string      `json:"name"`
	NameAlt     string      `json:"name_alt"`
	Father      string      `json:"father"`
	Mother      string      `json:"mother"`
	DOB         string      `json:"dob"`
	StudentID   string      `json:"student_id"`
	Class       string      `json:"class"`
	Section     string      `json:"section"`
	Roll        string      `json:"roll"`
	Session     string      `json:"session"`
	Exam        string      `json:"exam"`
	Medium      string      `json:"medium"` // version: Bangla/English medium, "" hides the row
	Shift       string      `json:"shift"`  // "" hides the row
	Group       string      `json:"group"`  // "" hides the row
	Photo       string      `json:"photo"`
	QR          string      `json:"qr"`
	Withheld    bool        `json:"withheld"`
	Rows        []sheetRow  `json:"rows"`
	Optional    []sheetRow  `json:"optional"`
	Total       string      `json:"total"`
	GPA         string      `json:"gpa"`
	GPAWithout  string      `json:"gpa_without"`
	Letter      string      `json:"letter"`
	Result      string      `json:"result"`
	Failed      bool        `json:"failed"`
	FailedCount string      `json:"failed_count"`
	ClassPos    string      `json:"class_pos"`
	SectionPos  string      `json:"section_pos"`
	Held        string      `json:"held"`         // classes held in the period
	PresentDays string      `json:"present_days"` // days present
	AbsentDays  string      `json:"absent_days"`  // days absent
	Published   string      `json:"published"`
	Comment     string      `json:"comment"`
	Signatures  []signature `json:"signatures"`
	Footer      string      `json:"footer"`
}

type sheetData struct {
	Lang        string            `json:"lang"`
	Title       string            `json:"title"`
	Heading     string            `json:"heading"`
	HeadingAlt  string            `json:"heading_alt"`
	ExamName    string            `json:"exam_name"`
	SessionName string            `json:"session_name"`
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
	case set.Assessment != nil:
		// An exam's own result: the component columns come from the
		// published lines (an exam without components shows totals only).
		lo.bands = set.Assessment.Bands
		if set.Assessment.GPARounding == "truncate" {
			lo.rounding = calc.Truncate
		}
		seen := map[string]bool{}
		for _, r := range set.Results {
			for _, line := range r.Outputs.Subjects {
				for _, c := range line.Components {
					if !seen[c.Key] {
						seen[c.Key] = true
						lo.compKeys = append(lo.compKeys, c.Key)
					}
				}
			}
		}
		lo.compKeys = orderComponents(lo.compKeys)
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

// rows breaks a result's subject lines into the table's rows: the required
// subjects, then the optional (fourth) one. A combined subject keeps one
// row per paper; its total, grade and point cover them all.
func (lo layout) rows(l lang, r exams.DocResult, top map[string]float64, names map[string]subjectName) (required, optional []sheetRow) {
	var inputs exams.TermInputs
	if lo.scope == exams.ScopeTerm {
		inputs, _ = exams.TermInputsOf(r.Inputs)
	}
	var examInputs exams.AssessmentInputs
	if lo.scope == exams.ScopeAssessment {
		examInputs, _ = exams.AssessmentInputsOf(r.Inputs)
	}
	required, optional = []sheetRow{}, []sheetRow{}
	for _, line := range r.Outputs.Subjects {
		row := sheetRow{Code: l.code(line.Code), Subject: l.pick(line.NameEN, line.NameBN), Optional: line.Optional, CT: "—", Full: l.marks(&line.Full), Highest: "—", Grade: "—", GP: "—", Papers: []paperRow{}}
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
			row.Parts = lo.termParts(l, inputs, line.SubjectIDs, &row)
			if len(line.SubjectIDs) > 1 {
				for _, sid := range line.SubjectIDs {
					n := names[sid]
					paper := paperRow{Code: l.code(n.Code), Subject: l.pick(n.EN, n.BN)}
					if paper.Subject == "" {
						paper.Subject = row.Subject
					}
					paper.Parts = lo.termParts(l, inputs, []string{sid}, nil)
					row.Papers = append(row.Papers, paper)
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
			row.Parts = make([]sheetCell, len(lo.termIDs))
			for i, id := range lo.termIDs {
				row.Parts[i] = sheetCell{Obt: "—"}
				for _, t := range line.Terms {
					if t.TermID == id && t.Marks != "" {
						m := t.Marks
						row.Parts[i].Obt = l.marks(&m)
					}
				}
			}
			if len(line.SubjectIDs) > 1 {
				parts := []string{}
				for _, sid := range line.SubjectIDs {
					if n, ok := names[sid]; ok {
						parts = append(parts, l.pick(n.EN, n.BN))
					}
				}
				row.Note = strings.Join(parts, " + ")
			}
		case exams.ScopeAssessment:
			row.Parts = lo.componentParts(l, line.Components)
			if line.Status == "absent" {
				row.Note = l.t("absent")
			}
			if len(line.SubjectIDs) > 1 {
				for _, in := range examInputs.Lines {
					if !slices.Contains(line.SubjectIDs, in.SubjectID) {
						continue
					}
					paper := paperRow{Code: l.code(in.Code), Subject: l.pick(in.NameEN, in.NameBN), Parts: lo.componentParts(l, in.Components)}
					if paper.Subject == "" {
						paper.Subject = row.Subject
					}
					row.Papers = append(row.Papers, paper)
				}
			}
		}
		if line.Optional {
			optional = append(optional, row)
		} else {
			required = append(required, row)
		}
	}
	return required, optional
}

// componentParts lays a published line's components out over the layout's
// component columns (assessment scope). A component below its own pass marks
// is marked failed.
func (lo layout) componentParts(l lang, comps []exams.OutComponent) []sheetCell {
	parts := make([]sheetCell, len(lo.compKeys))
	for i, key := range lo.compKeys {
		parts[i] = sheetCell{Obt: "—"}
		for _, c := range comps {
			if !strings.EqualFold(c.Key, key) {
				continue
			}
			parts[i].Obt = l.marks(c.Marks)
			if c.Full != "" {
				full := c.Full
				parts[i].Full = l.marks(&full)
			}
			if c.Marks != nil && c.Pass != nil {
				got, err1 := strconv.ParseFloat(*c.Marks, 64)
				pass, err2 := strconv.ParseFloat(*c.Pass, 64)
				parts[i].Failed = err1 == nil && err2 == nil && got < pass
			}
		}
	}
	return parts
}

// termParts sums each component's obtained and full marks over the given
// papers of the term's exams. With row set, an absence is noted on it.
func (lo layout) termParts(l lang, inputs exams.TermInputs, subjectIDs []string, row *sheetRow) []sheetCell {
	parts := make([]sheetCell, len(lo.compKeys))
	for i, key := range lo.compKeys {
		sum, full, pass, any, anyFull, anyPass := 0.0, 0.0, 0.0, false, false, false
		for _, sid := range subjectIDs {
			for ak := range lo.examKeys {
				m, ok := inputs.Marks[ak][sid]
				if !ok {
					continue
				}
				for _, c := range m.Components {
					if c.Key != key {
						continue
					}
					if v, err := strconv.ParseFloat(c.Full, 64); err == nil {
						full, anyFull = full+v, true
					}
					if c.Pass != nil {
						if v, err := strconv.ParseFloat(*c.Pass, 64); err == nil {
							pass, anyPass = pass+v, true
						}
					}
					if c.Marks != nil {
						if v, err := strconv.ParseFloat(*c.Marks, 64); err == nil {
							sum, any = sum+v, true
						}
					}
				}
				// An ordinary paper keeps its mark without components.
				if strings.EqualFold(key, "TOTAL") && len(m.Components) == 0 {
					if v, err := strconv.ParseFloat(m.Full, 64); err == nil {
						full, anyFull = full+v, true
					}
					if m.Marks != nil {
						if v, err := strconv.ParseFloat(*m.Marks, 64); err == nil {
							sum, any = sum+v, true
						}
					}
				}
				if row != nil && m.Status == "absent" && i == 0 {
					row.Note = l.t("absent")
				}
			}
		}
		parts[i] = sheetCell{Obt: "—"}
		if any {
			s := markText(sum)
			parts[i].Obt = l.marks(&s)
			parts[i].Failed = anyPass && sum < pass
		}
		if anyFull {
			s := markText(full)
			parts[i].Full = l.marks(&s)
		}
	}
	return parts
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

// posText is a merit position in ordinal words, "3rd of 60" (in Bangla
// "৬০ জনের মধ্যে ৩য়") when the count is known.
func posText(l lang, p *int, of int) string {
	if p == nil || *p <= 0 {
		return "—"
	}
	if of > 0 {
		return fmt.Sprintf(l.t("pos_of"), l.ordinal(*p), l.int(of))
	}
	return l.ordinal(*p)
}

func rollText(l lang, roll *int) string {
	if roll == nil {
		return "—"
	}
	return l.int(*roll)
}
