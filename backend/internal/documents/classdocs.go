package documents

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

// The data the class template reads (templates/class.typ).

type tabSubject struct {
	Key  string `json:"-"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type tabCell struct {
	Marks  string `json:"marks"`
	Grade  string `json:"grade"`
	Failed bool   `json:"failed"`
}

type tabRow struct {
	Roll     string    `json:"roll"`
	Name     string    `json:"name"`
	Section  string    `json:"section"`
	Cells    []tabCell `json:"cells"`
	Total    string    `json:"total"`
	GPA      string    `json:"gpa"`
	Letter   string    `json:"letter"`
	Position string    `json:"position"`
	Result   string    `json:"result"`
	Failed   bool      `json:"failed"`
}

type meritRow struct {
	Position string `json:"position"`
	Roll     string `json:"roll"`
	Name     string `json:"name"`
	NameAlt  string `json:"name_alt"`
	Section  string `json:"section"`
	Total    string `json:"total"`
	GPA      string `json:"gpa"`
	Letter   string `json:"letter"`
}

type failedRow struct {
	Roll     string `json:"roll"`
	Name     string `json:"name"`
	Section  string `json:"section"`
	Subjects string `json:"subjects"`
}

type summaryTile struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Strong bool   `json:"strong"`
}

type summaryRow struct {
	Cells []string `json:"cells"`
}

type gradeShare struct {
	Grade string  `json:"grade"`
	Count string  `json:"count"`
	Share string  `json:"share"`
	Ratio float64 `json:"ratio"`
}

type subjectGrades struct {
	Name   string   `json:"name"`
	Counts []string `json:"counts"`
}

type summaryData struct {
	Tiles         []summaryTile   `json:"tiles"`
	SectionHead   []string        `json:"section_head"`
	Sections      []summaryRow    `json:"sections"`
	Grades        []gradeShare    `json:"grades"`
	SubjectGrades []string        `json:"subject_grades"`
	Subjects      []subjectGrades `json:"subjects"`
}

type classData struct {
	Lang        string            `json:"lang"`
	Title       string            `json:"title"`
	Heading     string            `json:"heading"`
	Subtitle    string            `json:"subtitle"`
	Footer      string            `json:"footer"`
	Kind        string            `json:"kind"`
	Paper       string            `json:"paper"`
	Provisional bool              `json:"provisional"`
	Labels      map[string]string `json:"labels"`
	School      schoolData        `json:"school"`
	Tabulation  struct {
		Subjects []tabSubject `json:"subjects"`
		Rows     []tabRow     `json:"rows"`
		// Totals is the strip under the sheet: how many appeared, passed and
		// failed, how many reached the top grade, and the pass rate.
		Totals []summaryTile `json:"totals"`
	} `json:"tabulation"`
	Merit   []meritRow  `json:"merit"`
	Failed  []failedRow `json:"failed"`
	Summary summaryData `json:"summary"`
}

// byRoll orders results by section, then roll, then name.
func byRoll(rs []exams.DocResult) []exams.DocResult {
	out := append([]exams.DocResult{}, rs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Student, out[j].Student
		if a.SectionName != b.SectionName {
			return a.SectionName < b.SectionName
		}
		ra, rb := 1<<30, 1<<30
		if a.RollNo != nil {
			ra = *a.RollNo
		}
		if b.RollNo != nil {
			rb = *b.RollNo
		}
		if ra != rb {
			return ra < rb
		}
		return a.NameEN < b.NameEN
	})
	return out
}

// subjectsOf lists the set's subject lines in first-seen order.
func subjectsOf(l lang, rs []exams.DocResult) []tabSubject {
	seen := map[string]bool{}
	out := []tabSubject{}
	for _, r := range rs {
		for _, line := range r.Outputs.Subjects {
			if seen[line.Key] {
				continue
			}
			seen[line.Key] = true
			name := l.pick(line.NameEN, line.NameBN)
			if line.Optional {
				name += " *"
			}
			out = append(out, tabSubject{Key: line.Key, Code: l.code(line.Code), Name: name})
		}
	}
	return out
}

func (s *Service) classData(l lang, kind string, set exams.DocSet, results []exams.DocResult, section string, school schoolData) classData {
	lo := layoutOf(set)
	d := classData{Lang: string(l), Kind: kind, Paper: "a4", Provisional: set.Provisional, Labels: labelsFor(l), School: school}
	d.Heading = l.t(kind)
	place := l.t("class") + ": " + set.ClassName
	if section != "" {
		place += " · " + l.t("section") + ": " + section
	} else {
		place += " · " + l.t("all_sections")
	}
	d.Subtitle = place + " · " + examName(l, set)
	d.Title = d.Heading + " — " + set.ClassName
	if set.Provisional {
		d.Footer = l.t("footer_provisional")
	} else {
		d.Footer = fmt.Sprintf("%s: %s %s · %s", l.t("published"), l.t("version"), l.int(set.Revision), l.date(set.PublishedAt))
	}
	ordered := byRoll(results)
	switch kind {
	case "tabulation":
		subjects := subjectsOf(l, ordered)
		passed, failed, top := 0, 0, 0
		topGrade := ""
		if bands := sortedBands(lo); len(bands) > 0 {
			topGrade = bands[0]
		}
		if len(subjects) > 10 {
			d.Paper = "a3"
		}
		d.Tabulation.Subjects = subjects
		for _, r := range ordered {
			row := tabRow{Roll: rollText(l, r.Student.RollNo), Name: l.pick(r.Student.NameEN, r.Student.NameBN), Result: outcomeText(l, r.Outcome), Failed: r.Outcome == "failed", Total: "—", GPA: "—", Letter: "—", Position: "—"}
			if section == "" {
				row.Section = r.Student.SectionName
			}
			lines := map[string]exams.OutLine{}
			for _, line := range r.Outputs.Subjects {
				lines[line.Key] = line
			}
			for _, sub := range subjects {
				line, ok := lines[sub.Key]
				cell := tabCell{}
				if ok && r.Outcome != "withheld" {
					if line.Status == "graded" {
						cell.Marks = l.marks(line.Marks)
						if line.Grade != nil {
							cell.Grade = *line.Grade
						}
						cell.Failed = line.Passed != nil && !*line.Passed
					} else {
						cell.Marks = l.t(line.Status)
					}
				}
				row.Cells = append(row.Cells, cell)
			}
			if r.Outcome == "passed" || r.Outcome == "failed" {
				row.Total = l.marks(r.Outputs.Total)
				row.GPA = l.points(r.Outputs.GPA)
				row.Letter = lo.letter(r.Outputs)
				pos := r.Outputs.ClassPosition
				if section != "" {
					pos = r.Outputs.SectionPosition
				}
				row.Position = posText(l, pos, 0)
			}
			d.Tabulation.Rows = append(d.Tabulation.Rows, row)
			switch r.Outcome {
			case "passed":
				passed++
			case "failed":
				failed++
			}
			if topGrade != "" && (r.Outcome == "passed" || r.Outcome == "failed") && lo.letter(r.Outputs) == topGrade {
				top++
			}
		}
		// A withheld or absent student is on the sheet but did not appear.
		appeared := passed + failed
		d.Tabulation.Totals = []summaryTile{
			{Label: l.t("tab_appeared"), Value: l.int(appeared)},
			{Label: l.t("passed"), Value: l.int(passed)},
			{Label: l.t("failed"), Value: l.int(failed)},
		}
		if topGrade != "" {
			d.Tabulation.Totals = append(d.Tabulation.Totals, summaryTile{Label: topGrade, Value: l.int(top)})
		}
		d.Tabulation.Totals = append(d.Tabulation.Totals, summaryTile{Label: l.t("pass_rate"), Value: percent(l, passed, appeared), Strong: true})
	case "merit_list":
		ranked := append([]exams.DocResult{}, ordered...)
		pos := func(r exams.DocResult) int {
			p := r.Outputs.ClassPosition
			if section != "" {
				p = r.Outputs.SectionPosition
			}
			if p == nil || *p <= 0 {
				return 1 << 30
			}
			return *p
		}
		sort.SliceStable(ranked, func(i, j int) bool { return pos(ranked[i]) < pos(ranked[j]) })
		for _, r := range ranked {
			if r.Outcome != "passed" {
				continue
			}
			p := pos(r)
			row := meritRow{Position: "—", Roll: rollText(l, r.Student.RollNo), Name: l.pick(r.Student.NameEN, r.Student.NameBN), NameAlt: l.other(r.Student.NameEN, r.Student.NameBN),
				Section: r.Student.SectionName, Total: l.marks(r.Outputs.Total), GPA: l.points(r.Outputs.GPA), Letter: lo.letter(r.Outputs)}
			if p < 1<<30 {
				row.Position = l.int(p)
			}
			d.Merit = append(d.Merit, row)
		}
	case "failed_list":
		for _, r := range ordered {
			if r.Outcome == "withheld" {
				continue
			}
			names := []string{}
			for _, line := range r.Outputs.Subjects {
				if line.Passed != nil && !*line.Passed {
					names = append(names, fmt.Sprintf("%s (%s)", l.pick(line.NameEN, line.NameBN), l.marks(line.Marks)))
				}
			}
			if len(names) == 0 {
				continue
			}
			d.Failed = append(d.Failed, failedRow{Roll: rollText(l, r.Student.RollNo), Name: l.pick(r.Student.NameEN, r.Student.NameBN), Section: r.Student.SectionName, Subjects: strings.Join(names, ", ")})
		}
	case "summary":
		d.Summary = summaryOf(l, lo, ordered)
	}
	return d
}

func percent(l lang, n, of int) string {
	if of == 0 {
		return "—"
	}
	return l.num(strconv.FormatFloat(float64(n)*100/float64(of), 'f', 1, 64)) + "%"
}

func summaryOf(l lang, lo layout, rs []exams.DocResult) summaryData {
	s := summaryData{}
	passed, failed, withheld := 0, 0, 0
	type counts struct{ students, passed, failed, withheld int }
	sections := map[string]*counts{}
	order := []string{}
	for _, r := range rs {
		name := r.Student.SectionName
		c, ok := sections[name]
		if !ok {
			c = &counts{}
			sections[name] = c
			order = append(order, name)
		}
		c.students++
		switch r.Outcome {
		case "passed":
			passed++
			c.passed++
		case "failed":
			failed++
			c.failed++
		case "withheld":
			withheld++
			c.withheld++
		}
	}
	sort.Strings(order)
	s.Tiles = []summaryTile{
		{Label: l.t("students"), Value: l.int(len(rs))},
		{Label: l.t("passed"), Value: l.int(passed)},
		{Label: l.t("failed"), Value: l.int(failed)},
		{Label: l.t("withheld"), Value: l.int(withheld)},
		{Label: l.t("pass_rate"), Value: percent(l, passed, passed+failed), Strong: true},
	}
	s.SectionHead = []string{l.t("section"), l.t("students"), l.t("passed"), l.t("failed"), l.t("withheld"), l.t("pass_rate")}
	for _, name := range order {
		c := sections[name]
		s.Sections = append(s.Sections, summaryRow{Cells: []string{name, l.int(c.students), l.int(c.passed), l.int(c.failed), l.int(c.withheld), percent(l, c.passed, c.passed+c.failed)}})
	}
	// Overall grade distribution: each student's letter by GPA.
	grades := []string{}
	for _, b := range sortedBands(lo) {
		grades = append(grades, b)
	}
	byGrade := map[string]int{}
	graded := 0
	for _, r := range rs {
		if r.Outcome != "passed" && r.Outcome != "failed" {
			continue
		}
		graded++
		byGrade[lo.letter(r.Outputs)]++
	}
	for _, g := range grades {
		n := byGrade[g]
		ratio := 0.0
		if graded > 0 {
			ratio = float64(n) / float64(graded)
		}
		s.Grades = append(s.Grades, gradeShare{Grade: g, Count: l.int(n), Share: percent(l, n, graded), Ratio: ratio})
	}
	// Grades by subject.
	s.SubjectGrades = grades
	for _, sub := range subjectsOf(l, rs) {
		counts := map[string]int{}
		for _, r := range rs {
			for _, line := range r.Outputs.Subjects {
				if line.Key == sub.Key && line.Grade != nil && r.Outcome != "withheld" {
					counts[*line.Grade]++
				}
			}
		}
		row := subjectGrades{Name: sub.Name}
		for _, g := range grades {
			row.Counts = append(row.Counts, l.int(counts[g]))
		}
		s.Subjects = append(s.Subjects, row)
	}
	return s
}

func sortedBands(lo layout) []string {
	bands := append(lo.bands[:0:0], lo.bands...)
	sort.Slice(bands, func(i, j int) bool { return bands[i].Min > bands[j].Min })
	out := []string{}
	for _, b := range bands {
		out = append(out, b.Grade)
	}
	return out
}

// examName names what the results are of: the term, or the annual result.
func examName(l lang, set exams.DocSet) string {
	if set.Scope == exams.ScopeAnnual {
		return l.t("annual") + " " + l.num(set.SessionName)
	}
	return set.Title + " · " + l.num(set.SessionName)
}

// labelsFor is every label a template reads.
func labelsFor(l lang) map[string]string {
	out := map[string]string{}
	for k := range labelsEN {
		out[k] = l.t(k)
	}
	out["scan"] = map[lang]string{"en": "Scan to verify", "bn": "যাচাই করতে স্ক্যান করুন"}[l]
	return out
}
