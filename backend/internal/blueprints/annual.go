package blueprints

import (
	"math/big"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// Annual results combine a student's published term results subject by
// subject (calculation rules §3.6, level "subject_percent"):
//   - final_only: the final term's subject percentage;
//   - equal: the average of the term percentages;
//   - weighted: Σ weight × percentage / Σ weight.
//
// The annual mark is that share of the final term's full marks, rounded to
// two decimals (R5) and to whole marks if the school chose R3, then graded
// with the term pass mark. The arithmetic is exact (math/big.Rat).
//
// A term in which the subject is exempt, or which the subject does not have
// (an optional subject chosen later), leaves that subject's average and the
// other weights count proportionally. A withheld term withholds the annual
// result; a student without a published result in a term is incomplete.

// AnnualTerm is one term the annual result draws from, in sequence order.
type AnnualTerm struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence int    `json:"sequence"`
	Weight   int    `json:"weight"` // weighted method only
}

// AnnualRules is how a class's annual result is made.
type AnnualRules struct {
	Method        string       `json:"method"` // final_only, equal or weighted
	FinalMustPass bool         `json:"final_must_pass"`
	WholeMarks    bool         `json:"whole_marks"`
	GPARounding   string       `json:"gpa_rounding"`
	Ranking       string       `json:"ranking"`
	Terms         []AnnualTerm `json:"terms"` // the last is the final term
}

// AnnualLineInput is one subject line of a published term result.
type AnnualLineInput struct {
	Key         string   `json:"key"` // the combined key, or the subject id
	SubjectIDs  []string `json:"subject_ids"`
	CombinedKey string   `json:"combined_key,omitempty"`
	Optional    bool     `json:"optional"`
	Status      string   `json:"status"` // graded, exempt, incomplete or withheld
	Marks       string   `json:"marks"`
	Full        string   `json:"full"`
	Pass        string   `json:"pass"`
	Passed      bool     `json:"passed"`
}

// AnnualTermInput is a student's published result of one term.
type AnnualTermInput struct {
	TermID    string            `json:"term_id"`
	VersionID string            `json:"version_id"`
	Version   int               `json:"version"`
	Outcome   string            `json:"outcome"` // "" when the term has no published result
	Lines     []AnnualLineInput `json:"lines"`
}

// AnnualTermLine is one term's part in an annual subject line.
type AnnualTermLine struct {
	TermID string `json:"term_id"`
	Status string `json:"status"` // graded, exempt, withheld, incomplete or missing
	Marks  string `json:"marks"`
	Full   string `json:"full"`
	Weight int    `json:"weight"` // the weight it counted with; 0 when it left the line
}

// AnnualStudent is one student's input: one entry per rules term.
type AnnualStudent struct {
	ID        string
	SectionID string
	Terms     []AnnualTermInput
}

// AnnualResults computes the annual results of a class, then positions.
func AnnualResults(rules AnnualRules, students []AnnualStudent, scheme calc.Scheme) ([]StudentResult, error) {
	out := make([]StudentResult, 0, len(students))
	ranked := []calc.Ranked{}
	for _, st := range students {
		r, key, err := studentAnnual(rules, st, scheme)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
		if r.Outcome == "passed" || r.Outcome == "failed" {
			ranked = append(ranked, key)
		}
	}
	assignPositions(out, ranked, rules.Ranking)
	return out, nil
}

// assignPositions sets class positions over every ranked result and section
// positions within each section. Unranked results keep 0.
func assignPositions(out []StudentResult, ranked []calc.Ranked, ranking string) {
	mode := calc.Competition
	if ranking == "dense" {
		mode = calc.Dense
	}
	sectionOf := map[string]string{}
	for _, r := range out {
		sectionOf[r.StudentID] = r.SectionID
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
}

func weightOf(rules AnnualRules, i int) int64 {
	switch rules.Method {
	case "final_only":
		if i == len(rules.Terms)-1 {
			return 1
		}
		return 0
	case "weighted":
		return int64(rules.Terms[i].Weight)
	default:
		return 1
	}
}

func lineOf(t AnnualTermInput, key string) *AnnualLineInput {
	for i := range t.Lines {
		if t.Lines[i].Key == key {
			return &t.Lines[i]
		}
	}
	return nil
}

func studentAnnual(rules AnnualRules, st AnnualStudent, scheme calc.Scheme) (StudentResult, calc.Ranked, error) {
	none := calc.Ranked{}
	res := StudentResult{StudentID: st.ID, SectionID: st.SectionID, Subjects: []SubjectResult{}}
	n := len(rules.Terms)
	if n == 0 || len(st.Terms) != n {
		return res, none, ErrInvalidInput
	}
	for _, t := range st.Terms {
		if t.Outcome == "" || t.Outcome == "incomplete" {
			res.Outcome = "incomplete"
			return res, none, nil
		}
	}
	for _, t := range st.Terms {
		if t.Outcome == "withheld" {
			res.Outcome = "withheld"
			return res, none, nil
		}
	}
	final := st.Terms[n-1]
	in := []calc.Line{}
	graded := []int{}
	optionalLine := map[int]bool{}
	for _, fl := range final.Lines {
		sr := SubjectResult{SubjectIDs: fl.SubjectIDs, CombinedKey: fl.CombinedKey, Optional: fl.Optional,
			Reasons: []Reason{}, Contributions: []ContributionLine{}, Terms: []AnnualTermLine{}}
		num, den := new(big.Rat), int64(0)
		status := calc.Present
		var full, pass calc.Marks // from the latest graded term
		for i, term := range rules.Terms {
			tl := AnnualTermLine{TermID: term.ID, Status: "missing"}
			line := lineOf(st.Terms[i], fl.Key)
			w := weightOf(rules, i)
			if line != nil {
				tl.Status = line.Status
				switch line.Status {
				case "graded":
					m, err1 := parseMarks(line.Marks)
					f, err2 := parseMarks(line.Full)
					p, err3 := parseMarks(line.Pass)
					if err1 != nil || err2 != nil || err3 != nil || f <= 0 || m > f || p > f {
						return res, none, ErrInvalidInput
					}
					tl.Marks, tl.Full = line.Marks, line.Full
					full, pass = f, p
					if w > 0 {
						num.Add(num, new(big.Rat).Mul(big.NewRat(w, 1), big.NewRat(int64(m), int64(f))))
						den += w
						tl.Weight = int(w)
					}
				case "withheld":
					status = calc.Withheld
				case "incomplete":
					if status != calc.Withheld {
						status = calc.Incomplete
					}
				}
			}
			sr.Terms = append(sr.Terms, tl)
		}
		switch {
		case status != calc.Present:
			sr.Status = "withheld"
			if status == calc.Incomplete {
				sr.Status = "incomplete"
			}
			in = append(in, calc.Line{Status: status})
		case den == 0 || full == 0:
			sr.Status = "exempt" // no term counts for this subject
		default:
			share := new(big.Rat).Quo(num, big.NewRat(den, 1))
			marks := calc.RoundMarks(new(big.Rat).Mul(share, full.Rat()), calc.HalfUp)
			if rules.WholeMarks {
				marks = calc.WholeMarks(marks, calc.HalfUp)
			}
			failed := false
			if rules.FinalMustPass && fl.Status == "graded" && !fl.Passed {
				failed = true
				first := ""
				if len(fl.SubjectIDs) > 0 {
					first = fl.SubjectIDs[0]
				}
				sr.Reasons = append(sr.Reasons, Reason{Code: "final_term_failed", SubjectID: first})
			}
			sr.Status = "graded"
			sr.Marks, sr.Full, sr.Pass = marks.String(), full.String(), pass.String()
			in = append(in, calc.Line{Marks: marks, Full: full, Pass: pass, Optional: fl.Optional, Failed: failed})
			graded = append(graded, len(res.Subjects))
			optionalLine[len(res.Subjects)] = fl.Optional
		}
		res.Subjects = append(res.Subjects, sr)
	}
	required := false
	for _, i := range graded {
		required = required || !optionalLine[i]
	}
	if !required {
		res.Outcome = "incomplete"
		for _, sr := range res.Subjects {
			if sr.Status == "withheld" {
				res.Outcome = "withheld"
			}
		}
		return res, none, nil
	}
	gpaRounding := calc.HalfUp
	if rules.GPARounding == "truncate" {
		gpaRounding = calc.Truncate
	}
	r, err := calc.CalculateWithStatuses(in, scheme, calc.Policy{GPA: gpaRounding}, calc.AbsentScoresZero)
	if err != nil {
		return res, none, ErrInvalidInput
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

// parseMarks reads a two-decimal mark ("68.50").
func parseMarks(s string) (calc.Marks, error) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, ErrInvalidInput
	}
	h := new(big.Rat).Mul(r, big.NewRat(100, 1))
	if !h.IsInt() || h.Sign() < 0 {
		return 0, ErrInvalidInput
	}
	return calc.Marks(h.Num().Int64()), nil
}
