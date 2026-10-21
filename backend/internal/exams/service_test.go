package exams

import (
	"errors"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

func TestBangladeshGradesAndFourthSubject(t *testing.T) {
	bands := []Band{{"A+", 80, 100, 5}, {"A", 70, 79, 4}, {"A-", 60, 69, 3.5}, {"B", 50, 59, 3}, {"C", 40, 49, 2}, {"D", 33, 39, 1}, {"F", 0, 32, 0}}
	for _, tc := range []struct {
		marks float64
		grade string
		pass  bool
	}{{80, "A+", true}, {79.99, "A", true}, {70, "A", true}, {69.99, "A-", true}, {33, "D", true}, {32.99, "F", false}, {0, "F", false}} {
		g, err := GradeFor(tc.marks, 100, 33, bands)
		if err != nil || g.Name != tc.grade || g.Passed != tc.pass {
			t.Fatalf("marks %.2f: %+v %v", tc.marks, g, err)
		}
	}
	half, err := GradeFor(40, 50, 17, bands)
	if err != nil || half.Name != "A+" {
		t.Fatalf("scaled full marks: %+v %v", half, err)
	}
	gpa, passed, _, err := GPA([]ScoredSubject{{80, 100, 33, false}, {70, 100, 33, false}, {80, 100, 33, true}}, bands)
	if err != nil || !passed || gpa != 5 {
		t.Fatalf("fourth bonus cap: %.2f %v %v", gpa, passed, err)
	}
	gpa, passed, _, err = GPA([]ScoredSubject{{32, 100, 33, false}, {80, 100, 33, false}, {100, 100, 33, true}}, bands)
	if err != nil || passed || gpa != 0 {
		t.Fatalf("required failure: %.2f %v %v", gpa, passed, err)
	}
	gpa, passed, _, err = GPA([]ScoredSubject{{50, 100, 60, false}}, bands)
	if err != nil || passed || gpa != 0 {
		t.Fatalf("pass threshold: %.2f %v %v", gpa, passed, err)
	}
	if _, err := GradeFor(100.01, 100, 33, bands); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out of range: %v", err)
	}
	if _, _, _, err := GPA([]ScoredSubject{{80, 100, 33, false}, {80, 100, 33, true}, {80, 100, 33, true}}, bands); !errors.Is(err, ErrInvalid) {
		t.Fatalf("two fourth subjects: %v", err)
	}
}

// TestExactBandEdges covers the E2 fix: a mark exactly on a band edge gets
// that band (the float engine graded 16.83 of 51, exactly 33 %, as F).
func TestExactBandEdges(t *testing.T) {
	bands := []Band{{"A+", 80, 100, 5}, {"A", 70, 79, 4}, {"A-", 60, 69, 3.5}, {"B", 50, 59, 3}, {"C", 40, 49, 2}, {"D", 33, 39, 1}, {"F", 0, 32, 0}}
	for _, tc := range []struct {
		marks, full, pass float64
		grade             string
		passed            bool
	}{{16.83, 51, 16.83, "D", true}, {16.82, 51, 16.83, "F", false}, {10.2, 17, 5, "A-", true}, {18.4, 23, 7, "A+", true}, {18.39, 23, 7, "A", true}} {
		g, err := GradeFor(tc.marks, tc.full, tc.pass, bands)
		if err != nil || g.Name != tc.grade || g.Passed != tc.passed {
			t.Fatalf("%.2f/%.2f: %+v %v, want %s", tc.marks, tc.full, g, err, tc.grade)
		}
	}
	gpa, passed, grades, err := GPA([]ScoredSubject{{16.83, 51, 16.83, false}, {80, 100, 33, false}}, bands)
	if err != nil || !passed || gpa != 3 || grades[0].Name != "D" {
		t.Fatalf("GPA with an edge mark: %.2f %v %+v %v", gpa, passed, grades, err)
	}
	r, err := Score([]ScoredSubject{{0.1, 1, 0.33, false}, {0.2, 1, 0.33, false}}, bands)
	if err != nil || r.Total.String() != "0.30" || r.Full.String() != "2.00" {
		t.Fatalf("exact totals: %s/%s %v", r.Total, r.Full, err)
	}
}

// TestScoreLinesStatuses covers the publication rules for mark statuses.
func TestScoreLinesStatuses(t *testing.T) {
	bands := []Band{{"A+", 80, 100, 5}, {"A", 70, 79, 4}, {"A-", 60, 69, 3.5}, {"B", 50, 59, 3}, {"C", 40, 49, 2}, {"D", 33, 39, 1}, {"F", 0, 32, 0}}
	m := func(v float64) *float64 { return &v }
	present := func(v float64) MarkLine {
		return MarkLine{Marks: m(v), FullMarks: 100, PassMarks: 33, Status: StatusPresent}
	}
	// All present: identical to Score.
	s, err := ScoreLines([]MarkLine{present(80), present(70)}, bands)
	if err != nil || s.Result.GPA != 450 || !s.Result.Passed || s.Result.Total != 15000 {
		t.Fatalf("present: %+v %v", s, err)
	}
	// Absent fails like a 0 and stores F.
	s, _ = ScoreLines([]MarkLine{present(80), {FullMarks: 100, PassMarks: 33, Status: StatusAbsent}}, bands)
	if s.Result.Passed || s.Result.GPA != 0 || *s.Lines[1].Grade != "F" || *s.Lines[1].Passed || s.Result.Full != 20000 {
		t.Fatalf("absent: %+v", s)
	}
	// Exempt leaves the result and the denominator; no grade is stored.
	s, _ = ScoreLines([]MarkLine{present(80), {FullMarks: 100, PassMarks: 33, Status: StatusExempt}, present(70)}, bands)
	if !s.Result.Passed || s.Result.GPA != 450 || s.Lines[1].Grade != nil || s.Result.Full != 20000 || *s.Lines[2].Grade != "A" {
		t.Fatalf("exempt: %+v", s)
	}
	// Withheld holds the whole result back, even with marks.
	if s, _ := ScoreLines([]MarkLine{present(80), {Marks: m(50), FullMarks: 100, PassMarks: 33, Status: StatusWithheld}}, bands); !s.Withheld {
		t.Fatalf("withheld: %+v", s)
	}
	if _, err := ScoreLines([]MarkLine{present(80), {FullMarks: 100, PassMarks: 33, Status: StatusIncomplete}}, bands); !errors.Is(err, ErrMarksIncomplete) {
		t.Fatalf("incomplete: %v", err)
	}
	if _, err := ScoreLines([]MarkLine{{FullMarks: 100, PassMarks: 33, Status: StatusExempt}, {Marks: m(90), FullMarks: 100, PassMarks: 33, Fourth: true, Status: StatusPresent}}, bands); !errors.Is(err, ErrAllSubjectsExempt) {
		t.Fatalf("all required exempt: %v", err)
	}
	if _, err := ScoreLines([]MarkLine{{FullMarks: 100, PassMarks: 33, Status: StatusPresent}}, bands); !errors.Is(err, ErrMarksIncomplete) {
		t.Fatalf("present without marks: %v", err)
	}
}

// Blueprint rules on one exam: a component below its pass mark fails the
// subject; papers combine into one subject (each-paper rule optional);
// absent can mean missing; GPA can be truncated.
func TestScoreLinesWithPolicy(t *testing.T) {
	bands := []Band{{"A+", 80, 100, 5}, {"A", 70, 79, 4}, {"A-", 60, 69, 3.5}, {"B", 50, 59, 3}, {"C", 40, 49, 2}, {"D", 33, 39, 1}, {"F", 0, 32, 0}}
	m := func(v float64) *float64 { return &v }
	pass := func(v float64) *float64 { return &v }
	// Example 3: CQ 16 below 17 fails Physics despite 66.
	phy := MarkLine{Marks: m(66), FullMarks: 100, PassMarks: 33, Status: StatusPresent, Components: []ComponentLine{{Key: "CQ", Marks: 16, Pass: pass(17)}, {Key: "MCQ", Marks: 25, Pass: pass(8)}, {Key: "PRAC", Marks: 25, Pass: pass(8)}}}
	eng := MarkLine{Marks: m(80), FullMarks: 100, PassMarks: 33, Status: StatusPresent}
	s, err := ScoreLinesWith([]MarkLine{phy, eng}, bands, ScorePolicy{})
	if err != nil || *s.Lines[0].Grade != "F" || s.Result.Passed {
		t.Fatalf("component failure: %+v %v", s, err)
	}
	// Example 4: Bangla 58 + 30 = 88 / 200 → C on the combined rule; F for
	// both papers when each must pass.
	b1 := MarkLine{Marks: m(58), FullMarks: 100, PassMarks: 33, Status: StatusPresent, Combined: "BAN"}
	b2 := MarkLine{Marks: m(30), FullMarks: 100, PassMarks: 33, Status: StatusPresent, Combined: "BAN"}
	s, err = ScoreLinesWith([]MarkLine{b1, b2, eng}, bands, ScorePolicy{})
	if err != nil || *s.Lines[0].Grade != "C" || *s.Lines[1].Grade != "C" || s.Result.GPA != 350 {
		t.Fatalf("combined: %+v %v", s.Lines, err)
	}
	s, _ = ScoreLinesWith([]MarkLine{b1, b2, eng}, bands, ScorePolicy{EachPaper: true})
	if *s.Lines[0].Grade != "F" || *s.Lines[1].Grade != "F" || s.Result.Passed {
		t.Fatalf("each paper: %+v", s.Lines)
	}
	// An exempt paper leaves the combined subject to the other paper (58 / 100,
	// B) and keeps no grade itself.
	ex := b2
	ex.Status, ex.Marks = StatusExempt, nil
	s, _ = ScoreLinesWith([]MarkLine{b1, ex, eng}, bands, ScorePolicy{EachPaper: true})
	if *s.Lines[0].Grade != "B" || s.Lines[1].Grade != nil {
		t.Fatalf("exempt paper: %+v", s.Lines)
	}
	// Absent means missing: the result waits.
	abs := MarkLine{FullMarks: 100, PassMarks: 33, Status: StatusAbsent}
	if _, err := ScoreLinesWith([]MarkLine{abs, eng}, bands, ScorePolicy{Absent: calc.AbsentIsMissing}); !errors.Is(err, ErrMarksIncomplete) {
		t.Fatalf("absent as missing: %v", err)
	}
	// Example 5's 4.4166… → 4.41 truncated.
	lines := []MarkLine{}
	for _, v := range []float64{85, 90, 75, 72, 65, 55} {
		lines = append(lines, MarkLine{Marks: m(v), FullMarks: 100, PassMarks: 33, Status: StatusPresent})
	}
	lines = append(lines, MarkLine{Marks: m(74), FullMarks: 100, PassMarks: 33, Status: StatusPresent, Fourth: true})
	if s, _ := ScoreLinesWith(lines, bands, ScorePolicy{GPA: calc.Truncate}); s.Result.GPA != 441 {
		t.Fatalf("truncate: %s", s.Result.GPA)
	}
	if s, _ := ScoreLines(lines, bands); s.Result.GPA != 442 {
		t.Fatalf("half up: %s", s.Result.GPA)
	}
}
