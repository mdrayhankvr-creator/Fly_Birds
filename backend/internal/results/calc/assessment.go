package calc

import (
	"math/big"
	"sort"
)

// This file holds the building blocks of blueprint results (CT + Final,
// best-N, components, combined papers, annual results and mark statuses).
// They are tested against the worked examples of the calculation rules but
// are not used by production code until the blueprint data model exists
// (milestones E5 and E6).

// Status is the status of one mark.
type Status int

const (
	Present Status = iota
	Absent
	Exempt
	Incomplete
	Withheld
)

// AbsentPolicy is what an absent mark means.
type AbsentPolicy int

const (
	// AbsentScoresZero counts an absence as 0, which fails (the default,
	// like today's 0 but labelled).
	AbsentScoresZero AbsentPolicy = iota
	// AbsentIsMissing makes the result incomplete until resolved.
	AbsentIsMissing
)

// Outcome is the overall state of a result.
type Outcome int

const (
	Passed Outcome = iota
	Failed
	OutcomeIncomplete
	OutcomeWithheld
)

// Score is one assessment's marks for a subject.
type Score struct {
	Marks  Marks
	Full   Marks
	Status Status
}

// Combine is how a contribution combines its source assessments.
type Combine int

const (
	Single  Combine = iota // exactly one source
	Sum                    // worth × Σmarks / Σfull
	Average                // worth × average of percentages
	BestN                  // the N best percentages, averaged
	Latest                 // the last source that is not exempt
)

// Contribution is one part of a term subject mark, e.g. "CTs averaged, worth
// 20" or "term exam, worth 80".
type Contribution struct {
	Sources []Score // in assessment order
	Combine Combine
	N       int   // for BestN
	Worth   Marks // the marks the contribution is scaled to
}

// ContributionResult is the exact value of a contribution.
type ContributionResult struct {
	Value   *big.Rat // in marks; nil when exempt or blocked
	Exempt  bool     // every source exempt: the contribution leaves the full marks
	Blocked Status   // Incomplete or Withheld when a source blocks the result
}

// Contribute evaluates a contribution exactly. Exempt sources leave the set
// (averages and best-N use the rest); absent sources score 0 or block,
// depending on the policy; incomplete and withheld sources block.
func Contribute(c Contribution, absent AbsentPolicy) (ContributionResult, error) {
	if c.Worth <= 0 || c.Combine == BestN && c.N < 1 || c.Combine == Single && len(c.Sources) != 1 {
		return ContributionResult{}, ErrInvalid
	}
	var used []Score
	blocked := Present
	for _, s := range c.Sources {
		if s.Full <= 0 || s.Marks < 0 || s.Marks > s.Full {
			return ContributionResult{}, ErrInvalid
		}
		switch s.Status {
		case Exempt:
			continue
		case Withheld:
			blocked = Withheld
		case Incomplete:
			if blocked != Withheld {
				blocked = Incomplete
			}
		case Absent:
			if absent == AbsentIsMissing {
				if blocked != Withheld {
					blocked = Incomplete
				}
				continue
			}
			s.Marks = 0
		}
		used = append(used, s)
	}
	if blocked != Present {
		return ContributionResult{Blocked: blocked}, nil
	}
	if len(used) == 0 {
		return ContributionResult{Exempt: true}, nil
	}
	ratio := func(s Score) *big.Rat { return big.NewRat(int64(s.Marks), int64(s.Full)) }
	share := new(big.Rat)
	switch c.Combine {
	case Single, Latest:
		share = ratio(used[len(used)-1])
	case Sum:
		var m, f int64
		for _, s := range used {
			m, f = m+int64(s.Marks), f+int64(s.Full)
		}
		share = big.NewRat(m, f)
	case Average:
		share = averageRatio(used, ratio)
	case BestN:
		sorted := append([]Score(nil), used...)
		sort.SliceStable(sorted, func(i, j int) bool { return ratio(sorted[i]).Cmp(ratio(sorted[j])) > 0 })
		if len(sorted) > c.N {
			sorted = sorted[:c.N]
		}
		share = averageRatio(sorted, ratio)
	default:
		return ContributionResult{}, ErrInvalid
	}
	return ContributionResult{Value: new(big.Rat).Mul(share, c.Worth.Rat())}, nil
}

func averageRatio(scores []Score, ratio func(Score) *big.Rat) *big.Rat {
	sum := new(big.Rat)
	for _, s := range scores {
		sum.Add(sum, ratio(s))
	}
	return sum.Quo(sum, big.NewRat(int64(len(scores)), 1))
}

// TermPolicy holds the rounding points that apply to a term subject mark.
type TermPolicy struct {
	RoundContributions bool     // R1: round each contribution to two decimals
	Rounding           Rounding // used by R1 and R2
	WholeMarks         bool     // R3: round the subject mark to whole marks
	Absent             AbsentPolicy
}

// TermMark sums the contributions of one subject, then applies R2 (two
// decimals) and, when chosen, R3 (whole marks). Full marks are the worth of
// the contributions that are not exempt.
func TermMark(cs []Contribution, p TermPolicy) (marks, full Marks, blocked Status, err error) {
	total := new(big.Rat)
	for _, c := range cs {
		r, err := Contribute(c, p.Absent)
		if err != nil {
			return 0, 0, Present, err
		}
		if r.Blocked > blocked {
			blocked = r.Blocked
		}
		if r.Exempt || r.Blocked != Present {
			continue
		}
		v := r.Value
		if p.RoundContributions {
			v = RoundMarks(v, p.Rounding).Rat()
		}
		total.Add(total, v)
		full += c.Worth
	}
	if blocked != Present {
		return 0, 0, blocked, nil
	}
	marks = RoundMarks(total, p.Rounding)
	if p.WholeMarks {
		marks = WholeMarks(marks, p.Rounding)
	}
	return marks, full, Present, nil
}

// Component is a separately marked part of a subject (CQ, MCQ, practical).
type Component struct {
	Key     string
	Marks   Marks
	Full    Marks
	Pass    Marks
	HasPass bool
	Status  Status
}

// SubjectLine is a graded subject with the reasons it failed, if any.
type SubjectLine struct {
	Marks   Marks
	Full    Marks
	Grade   SubjectGrade
	Reasons []string // e.g. "CQ below pass mark", "CQ absent"
	Blocked Status
}

// GradeComponents sums a subject's components and grades it. The subject
// fails, whatever its total, when any component with a pass mark is below it
// or a component is absent (under AbsentScoresZero).
func GradeComponents(cs []Component, subjectPass Marks, s Scheme, absent AbsentPolicy) (SubjectLine, error) {
	var line SubjectLine
	for _, c := range cs {
		if c.Full <= 0 || c.Marks < 0 || c.Marks > c.Full || c.HasPass && c.Pass > c.Full {
			return SubjectLine{}, ErrInvalid
		}
		switch c.Status {
		case Exempt:
			continue
		case Withheld:
			line.Blocked = Withheld
			continue
		case Incomplete:
			line.Blocked = max(line.Blocked, Incomplete)
			continue
		case Absent:
			if absent == AbsentIsMissing {
				line.Blocked = max(line.Blocked, Incomplete)
				continue
			}
			c.Marks = 0
			line.Reasons = append(line.Reasons, c.Key+" absent")
		default:
			if c.HasPass && c.Marks < c.Pass {
				line.Reasons = append(line.Reasons, c.Key+" below pass mark")
			}
		}
		line.Marks += c.Marks
		line.Full += c.Full
	}
	if line.Blocked != Present {
		return SubjectLine{Blocked: line.Blocked}, nil
	}
	if line.Full == 0 {
		return SubjectLine{}, ErrInvalid
	}
	g, err := GradeSubject(line.Marks, line.Full, min(subjectPass, line.Full), s)
	if err != nil {
		return SubjectLine{}, err
	}
	if len(line.Reasons) > 0 {
		g = s.failGrade()
	}
	line.Grade = g
	return line, nil
}

// failGrade is the scheme's failing grade (the lowest band with point 0).
func (s Scheme) failGrade() SubjectGrade {
	for i := len(s.bands) - 1; i >= 0; i-- {
		if s.bands[i].Point == 0 {
			return SubjectGrade{Grade: s.bands[i].Grade}
		}
	}
	return SubjectGrade{Grade: "F"}
}

// PaperRule is how combined papers (Bangla 1st + 2nd) pass.
type PaperRule int

const (
	// CombinedPass passes when the combined marks reach the papers' combined
	// pass marks.
	CombinedPass PaperRule = iota
	// EachPaperPass also requires every paper to reach its own pass mark.
	EachPaperPass
)

// Paper is one paper of a combined subject.
type Paper struct {
	Key   string
	Marks Marks
	Full  Marks
	Pass  Marks
}

// CombinePapers grades papers as one subject on their combined marks.
func CombinePapers(papers []Paper, rule PaperRule, s Scheme) (SubjectLine, error) {
	if len(papers) < 2 {
		return SubjectLine{}, ErrInvalid
	}
	var line SubjectLine
	var pass Marks
	for _, p := range papers {
		if p.Full <= 0 || p.Marks < 0 || p.Marks > p.Full || p.Pass > p.Full {
			return SubjectLine{}, ErrInvalid
		}
		line.Marks += p.Marks
		line.Full += p.Full
		pass += p.Pass
		if rule == EachPaperPass && p.Marks < p.Pass {
			line.Reasons = append(line.Reasons, p.Key+" below pass mark")
		}
	}
	g, err := GradeSubject(line.Marks, line.Full, pass, s)
	if err != nil {
		return SubjectLine{}, err
	}
	if len(line.Reasons) > 0 {
		g = s.failGrade()
	}
	line.Grade = g
	return line, nil
}

// AnnualMethod combines term results into the annual result.
type AnnualMethod int

const (
	FinalOnly AnnualMethod = iota
	Equal
	Weighted
)

// AnnualMark combines one subject's term marks (in term order) into an
// annual mark on full, at subject-percentage level: final-only takes the last
// term, equal averages the percentages, weighted uses weights summing to 100.
// The result is rounded to two decimals (R5), then to whole marks if chosen.
func AnnualMark(terms []Score, method AnnualMethod, weights []int, full Marks, rounding Rounding, whole bool) (Marks, error) {
	if len(terms) == 0 || full <= 0 {
		return 0, ErrInvalid
	}
	for _, t := range terms {
		if t.Full <= 0 || t.Marks < 0 || t.Marks > t.Full {
			return 0, ErrInvalid
		}
	}
	pct := func(t Score) *big.Rat { return big.NewRat(int64(t.Marks), int64(t.Full)) }
	share := new(big.Rat)
	switch method {
	case FinalOnly:
		share = pct(terms[len(terms)-1])
	case Equal:
		share = averageRatio(terms, pct)
	case Weighted:
		if len(weights) != len(terms) {
			return 0, ErrInvalid
		}
		total := 0
		for i, w := range weights {
			if w < 0 {
				return 0, ErrInvalid
			}
			total += w
			share.Add(share, new(big.Rat).Mul(big.NewRat(int64(w), 100), pct(terms[i])))
		}
		if total != 100 {
			return 0, ErrInvalid
		}
	default:
		return 0, ErrInvalid
	}
	m := RoundMarks(new(big.Rat).Mul(share, full.Rat()), rounding)
	if whole {
		m = WholeMarks(m, rounding)
	}
	return m, nil
}

// Line is one subject of a result with its mark status (blueprint results).
type Line struct {
	Marks    Marks
	Full     Marks
	Pass     Marks
	Optional bool
	Status   Status
	Failed   bool // already failed by a component or paper rule
}

// StatusResult is a result computed with mark statuses.
type StatusResult struct {
	Outcome Outcome
	Result  Result
}

// CalculateWithStatuses applies the production rules to subjects that carry
// statuses: withheld or incomplete subjects block the result; exempt subjects
// leave it (and the GPA denominator); absent subjects score 0 and fail under
// AbsentScoresZero; a subject already failed by a component rule fails.
func CalculateWithStatuses(lines []Line, s Scheme, p Policy, absent AbsentPolicy) (StatusResult, error) {
	subjects := make([]Subject, 0, len(lines))
	forced := make([]bool, 0, len(lines))
	blocked := Present
	for _, l := range lines {
		switch l.Status {
		case Exempt:
			continue
		case Withheld:
			blocked = Withheld
			continue
		case Incomplete:
			blocked = max(blocked, Incomplete)
			continue
		case Absent:
			if absent == AbsentIsMissing {
				blocked = max(blocked, Incomplete)
				continue
			}
			l.Marks, l.Failed = 0, true
		}
		subjects = append(subjects, Subject{Marks: l.Marks, Full: l.Full, Pass: l.Pass, Optional: l.Optional})
		forced = append(forced, l.Failed)
	}
	switch blocked {
	case Withheld:
		return StatusResult{Outcome: OutcomeWithheld}, nil
	case Incomplete:
		return StatusResult{Outcome: OutcomeIncomplete}, nil
	}
	grades := make([]SubjectGrade, 0, len(subjects))
	for i, sub := range subjects {
		g, err := GradeSubject(sub.Marks, sub.Full, sub.Pass, s)
		if err != nil {
			return StatusResult{}, err
		}
		if forced[i] {
			g = s.failGrade() // failed by a component, paper or absence
		}
		grades = append(grades, g)
	}
	res, err := aggregate(subjects, grades, p)
	if err != nil {
		return StatusResult{}, err
	}
	outcome := Failed
	if res.Passed {
		outcome = Passed
	}
	return StatusResult{Outcome: outcome, Result: res}, nil
}
