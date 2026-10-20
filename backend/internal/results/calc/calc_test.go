package calc

import (
	"errors"
	"math/big"
	"strconv"
	"testing"
)

// mk parses a two-decimal literal such as "16.83".
func mk(t testing.TB, s string) Marks {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MarksOf(v)
	if err != nil {
		t.Fatalf("%s: %v", s, err)
	}
	return m
}

var edges = []int{33, 40, 50, 60, 70, 80}

// bandStartingAt is the bd_default band whose lower edge is min.
func bandStartingAt(min int) Band {
	for _, b := range bdDefault().bands {
		if b.Min == min {
			return b
		}
	}
	panic("no band")
}

// bandBelow is the band just under the one starting at min.
func bandBelow(min int) Band {
	bands := bdDefault().bands
	for i, b := range bands {
		if b.Min == min {
			return bands[i+1]
		}
	}
	panic("no band")
}

// TestLegacyFloatDefectReproduced documents the defect E2 fixes: the old float
// engine grades 66 exact band edges (full marks 1–200) one band too low.
func TestLegacyFloatDefectReproduced(t *testing.T) {
	misgraded := 0
	for full := int64(1); full <= 200; full++ {
		for _, e := range edges {
			marks := float64(int64(e)*full) / 100 // exactly e % of full
			g, ok := legacyGradeFor(marks, float64(full), 0, bdDefaultLegacy)
			if !ok {
				t.Fatalf("legacy refused %.2f/%d", marks, full)
			}
			if g.Name != bandStartingAt(e).Grade {
				misgraded++
			}
		}
	}
	if misgraded != 66 {
		t.Fatalf("legacy engine misgraded %d exact edges, want the 66 recorded in the E1 audit", misgraded)
	}
	g, _ := legacyGradeFor(16.83, 51, 16.83, bdDefaultLegacy)
	if g.Name != "F" || g.Passed {
		t.Fatalf("legacy 16.83/51 = %+v, the audit recorded F and failed", g)
	}
}

// TestExactEdgesEverywhere checks every band edge for every full mark from
// 0.01 to 1000.00 (in steps of 0.01) where the edge is a two-decimal mark:
// one hundredth below is in the band below, the edge and one hundredth above
// are in the band.
func TestExactEdgesEverywhere(t *testing.T) {
	s := bdDefault()
	checked := 0
	for fullH := int64(1); fullH <= 100000; fullH++ {
		full := Marks(fullH)
		for _, e := range edges {
			if int64(e)*fullH%100 != 0 {
				continue // the edge is not a two-decimal mark for this full
			}
			at := Marks(int64(e) * fullH / 100)
			want := bandStartingAt(e)
			if b, err := s.Band(at, full); err != nil || b.Grade != want.Grade {
				t.Fatalf("%s/%s (exactly %d%%): %v %v, want %s", at, full, e, b.Grade, err, want.Grade)
			}
			// The neighbours are checked only while they stay inside the
			// neighbouring band (with a tiny full mark, 0.01 more can jump
			// two bands).
			if above := int64(at+1) * 100; at+1 <= full && above < int64(want.Max+1)*fullH {
				if b, _ := s.Band(at+1, full); b.Grade != want.Grade {
					t.Fatalf("%s/%s just above %d%%: %s, want %s", at+1, full, e, b.Grade, want.Grade)
				}
			}
			if below := int64(at-1) * 100; at > 0 && below >= int64(bandBelow(e).Min)*fullH {
				if b, _ := s.Band(at-1, full); b.Grade != bandBelow(e).Grade {
					t.Fatalf("%s/%s just below %d%%: %s, want %s", at-1, full, e, b.Grade, bandBelow(e).Grade)
				}
			}
			checked++
		}
	}
	if checked < 100000 {
		t.Fatalf("only %d edges checked", checked)
	}
}

func TestReportedBoundaryCases(t *testing.T) {
	s := bdDefault()
	for _, c := range []struct {
		marks, full, pass string
		grade             string
		point             Points
		passed            bool
	}{
		{"16.83", "51", "16.83", "D", 100, true},    // exactly 33 %: was F, failed
		{"16.82", "51", "16.83", "F", 0, false},     // just below
		{"16.84", "51", "16.83", "D", 100, true},    // just above
		{"10.20", "17", "5", "A-", 350, true},       // exactly 60 %: was B
		{"18.40", "23", "7", "A+", 500, true},       // exactly 80 %: was A
		{"32.80", "41", "14", "A+", 500, true},      // exactly 80 %: was A
		{"36.80", "46", "15", "A+", 500, true},      // exactly 80 %: was A
		{"34.30", "49", "17", "A", 400, true},       // exactly 70 %: was A-
		{"9.20", "23", "7", "C", 200, true},         // exactly 40 %: was D
		{"18.75", "37.50", "12.38", "B", 300, true}, // irregular full marks, exactly 50 %
	} {
		g, err := GradeSubject(mk(t, c.marks), mk(t, c.full), mk(t, c.pass), s)
		if err != nil || g.Grade != c.grade || g.Point != c.point || g.Passed != c.passed {
			t.Errorf("%s/%s: %+v %v, want %s %s %v", c.marks, c.full, g, err, c.grade, c.point, c.passed)
		}
	}
}

// TestCommonFullMarks checks just below, at and just above every edge for the
// full marks schools use (100, 50, 25), where old and new engines agree.
func TestCommonFullMarks(t *testing.T) {
	s := bdDefault()
	for _, full := range []int64{100, 50, 25} {
		for _, e := range edges {
			at := Marks(int64(e) * full) // e % of full, in hundredths
			for _, c := range []struct {
				m    Marks
				want Band
			}{{at - 1, bandBelow(e)}, {at, bandStartingAt(e)}, {at + 1, bandStartingAt(e)}} {
				g, err := GradeSubject(c.m, Marks(full*100), Marks(full*33), s)
				if err != nil || g.Grade != c.want.Grade {
					t.Errorf("%s/%d: %+v %v, want %s", c.m, full, g, err, c.want.Grade)
				}
				lg, _ := legacyGradeFor(c.m.Float(), float64(full), float64(full)*0.33, bdDefaultLegacy)
				if lg.Name != g.Grade || lg.Passed != g.Passed {
					t.Errorf("%s/%d: legacy %+v differs from exact %+v", c.m, full, lg, g)
				}
			}
		}
	}
	// The edges of the scheme itself: 0 and full marks.
	if g, _ := GradeSubject(0, 10000, 3300, s); g.Grade != "F" || g.Passed {
		t.Errorf("zero: %+v", g)
	}
	if g, _ := GradeSubject(10000, 10000, 3300, s); g.Grade != "A+" || !g.Passed {
		t.Errorf("full: %+v", g)
	}
}

func TestInvalidInput(t *testing.T) {
	s := bdDefault()
	for _, v := range []float64{-0.01, 1.005, 33.333} {
		if _, err := MarksOf(v); !errors.Is(err, ErrInvalid) {
			t.Errorf("MarksOf(%v) accepted", v)
		}
	}
	if _, err := GradeSubject(10001, 10000, 3300, s); !errors.Is(err, ErrInvalid) {
		t.Error("marks above full accepted")
	}
	if _, err := GradeSubject(5000, 10000, 10001, s); !errors.Is(err, ErrInvalid) {
		t.Error("pass above full accepted")
	}
	if _, err := GradeSubject(5000, 0, 0, s); !errors.Is(err, ErrInvalid) {
		t.Error("zero full marks accepted")
	}
	if _, err := NewScheme(nil); !errors.Is(err, ErrInvalid) {
		t.Error("empty scheme accepted")
	}
	gap, _ := NewScheme([]Band{{"A", 50, 100, 500}, {"F", 0, 40, 0}})
	if _, err := gap.Band(4500, 10000); !errors.Is(err, ErrInvalid) {
		t.Error("a percentage in a scheme gap was graded")
	}
	two := []Subject{{8000, 10000, 3300, false}, {8000, 10000, 3300, true}, {8000, 10000, 3300, true}}
	if _, err := Calculate(two, s, Policy{}); !errors.Is(err, ErrInvalid) {
		t.Error("two optional subjects accepted")
	}
	if _, err := Calculate([]Subject{{8000, 10000, 3300, true}}, s, Policy{}); !errors.Is(err, ErrInvalid) {
		t.Error("only an optional subject accepted")
	}
}

func TestMarksFormatting(t *testing.T) {
	if got := mk(t, "16.83").String(); got != "16.83" {
		t.Errorf("String %q", got)
	}
	if got := Points(350).String(); got != "3.50" {
		t.Errorf("String %q", got)
	}
	if got := Marks(5).String(); got != "0.05" {
		t.Errorf("String %q", got)
	}
	if got := mk(t, "300.30").Float(); got != 300.3 {
		t.Errorf("Float %v", got)
	}
}

// Example 5 of the calculation rules: GPA with an optional subject, rounding
// policies and the 5.00 cap.
func TestGPAExamples(t *testing.T) {
	s := bdDefault()
	required := []Subject{ // A+, A+, A, A, A-, B → 24.5 points
		{8500, 10000, 3300, false}, {9000, 10000, 3300, false}, {7500, 10000, 3300, false},
		{7000, 10000, 3300, false}, {6500, 10000, 3300, false}, {5500, 10000, 3300, false},
	}
	with := func(opt Subject) []Subject { return append(append([]Subject(nil), required...), opt) }
	for _, c := range []struct {
		name     string
		subjects []Subject
		policy   Policy
		gpa      Points
	}{
		{"no optional", required, Policy{}, 408},
		{"optional A, half up", with(Subject{7500, 10000, 3300, true}), Policy{}, 442},
		{"optional A, truncated", with(Subject{7500, 10000, 3300, true}), Policy{GPA: Truncate}, 441},
		{"optional F does not fail", with(Subject{2000, 10000, 3300, true}), Policy{}, 408},
	} {
		r, err := Calculate(c.subjects, s, c.policy)
		if err != nil || !r.Passed || r.GPA != c.gpa {
			t.Errorf("%s: %+v %v, want %s", c.name, r, err, c.gpa)
		}
	}
	capped := []Subject{{9000, 10000, 3300, false}, {9000, 10000, 3300, false}, {9000, 10000, 3300, false}, {9000, 10000, 3300, false}, {9000, 10000, 3300, false}, {9000, 10000, 3300, true}}
	if r, _ := Calculate(capped, s, Policy{}); r.GPA != 500 {
		t.Errorf("cap: %s", r.GPA)
	}
	failed := []Subject{{3200, 10000, 3300, false}, {8000, 10000, 3300, false}, {10000, 10000, 3300, true}}
	if r, _ := Calculate(failed, s, Policy{}); r.Passed || r.GPA != 0 || len(r.Grades) != 3 {
		t.Errorf("failed required subject: %+v", r)
	}
	// Pass mark above the band floor: 50/100 with pass mark 60 fails.
	if r, _ := Calculate([]Subject{{5000, 10000, 6000, false}}, s, Policy{}); r.Passed {
		t.Errorf("pass mark: %+v", r)
	}
}

// Examples 1 and 2: CT + Final and best two of three CTs, with rounding
// policy R3 changing the grade in example 1.
func TestTermMarkExamples(t *testing.T) {
	s := bdDefault()
	ct := func(m int64) Score { return Score{Marks: Marks(m * 100), Full: 2000} }
	term := func(m int64) Score { return Score{Marks: Marks(m * 100), Full: 10000} }
	ex1 := []Contribution{
		{Sources: []Score{ct(17), ct(14)}, Combine: Average, Worth: 2000},
		{Sources: []Score{term(68)}, Combine: Single, Worth: 8000},
	}
	m, full, blocked, err := TermMark(ex1, TermPolicy{})
	if err != nil || m != 6990 || full != 10000 || blocked != Present {
		t.Fatalf("example 1: %s/%s %v %v", m, full, blocked, err)
	}
	if g, _ := GradeSubject(m, full, 3300, s); g.Grade != "A-" || g.Point != 350 {
		t.Errorf("example 1 grade: %+v", g)
	}
	whole, _, _, _ := TermMark(ex1, TermPolicy{WholeMarks: true})
	if g, _ := GradeSubject(whole, full, 3300, s); whole != 7000 || g.Grade != "A" {
		t.Errorf("example 1 whole marks: %s %+v", whole, g)
	}
	ex2 := []Contribution{
		{Sources: []Score{ct(12), ct(18), ct(15)}, Combine: BestN, N: 2, Worth: 2000},
		{Sources: []Score{term(70)}, Combine: Single, Worth: 8000},
	}
	if m, _, _, _ := TermMark(ex2, TermPolicy{}); m != 7250 {
		t.Errorf("example 2: %s, want 72.50", m)
	}
	ex2[0].Combine = Average
	if m, _, _, _ := TermMark(ex2, TermPolicy{}); m != 7100 {
		t.Errorf("example 2 plain average: %s, want 71.00", m)
	}
	// Sum of uneven CTs: (15 + 20) / (20 + 25) × 20 = 15.555… → 15.56.
	r, _ := Contribute(Contribution{Sources: []Score{{1500, 2000, Present}, {2000, 2500, Present}}, Combine: Sum, Worth: 2000}, AbsentScoresZero)
	if RoundMarks(r.Value, HalfUp) != 1556 || RoundMarks(r.Value, Truncate) != 1555 {
		t.Errorf("sum contribution: %s", r.Value.FloatString(4))
	}
	// Latest: the last CT that is not exempt.
	r, _ = Contribute(Contribution{Sources: []Score{ct(12), ct(18), {Status: Exempt, Full: 2000}}, Combine: Latest, Worth: 2000}, AbsentScoresZero)
	if r.Value.Cmp(big.NewRat(18, 1)) != 0 {
		t.Errorf("latest: %s", r.Value.FloatString(2))
	}
	// R1 rounds each contribution before summing: 15.555… + 54.4 → 15.56 + 54.40.
	cs := []Contribution{{Sources: []Score{{1500, 2000, Present}, {2000, 2500, Present}}, Combine: Sum, Worth: 2000}, {Sources: []Score{term(68)}, Combine: Single, Worth: 8000}}
	if a, _, _, _ := TermMark(cs, TermPolicy{}); a != 6996 {
		t.Errorf("R1 off: %s", a)
	}
	if a, _, _, _ := TermMark(cs, TermPolicy{RoundContributions: true, Rounding: Truncate}); a != 6995 {
		t.Errorf("R1 truncated: %s", a)
	}
}

// Example 3: a component failure despite a high total; example 4: combined
// papers under both pass rules.
func TestComponentAndPaperExamples(t *testing.T) {
	s := bdDefault()
	physics := []Component{
		{Key: "CQ", Marks: 1600, Full: 5000, Pass: 1700, HasPass: true},
		{Key: "MCQ", Marks: 2500, Full: 2500, Pass: 800, HasPass: true},
		{Key: "PRAC", Marks: 2500, Full: 2500, Pass: 800, HasPass: true},
	}
	line, err := GradeComponents(physics, 3300, s, AbsentScoresZero)
	if err != nil || line.Marks != 6600 || line.Grade.Grade != "F" || line.Grade.Passed || len(line.Reasons) != 1 || line.Reasons[0] != "CQ below pass mark" {
		t.Fatalf("example 3: %+v %v", line, err)
	}
	if b, _ := s.Band(6600, 10000); b.Grade != "A-" {
		t.Errorf("66 alone is %s, the example says A-", b.Grade)
	}
	physics[0].Marks = 1700
	if line, _ := GradeComponents(physics, 3300, s, AbsentScoresZero); !line.Grade.Passed || line.Grade.Grade != "A-" {
		t.Errorf("CQ at its pass mark: %+v", line)
	}
	physics[1].Status = Absent
	if line, _ := GradeComponents(physics, 3300, s, AbsentScoresZero); line.Grade.Passed || line.Reasons[0] != "MCQ absent" || line.Marks != 4200 {
		t.Errorf("absent component: %+v", line)
	}
	if line, _ := GradeComponents(physics, 3300, s, AbsentIsMissing); line.Blocked != Incomplete {
		t.Errorf("absent component, missing policy: %+v", line)
	}
	bangla := []Paper{{"1st paper", 5800, 10000, 3300}, {"2nd paper", 3000, 10000, 3300}}
	combined, _ := CombinePapers(bangla, CombinedPass, s)
	if combined.Marks != 8800 || combined.Full != 20000 || combined.Grade.Grade != "C" || !combined.Grade.Passed {
		t.Errorf("example 4 combined: %+v", combined)
	}
	each, _ := CombinePapers(bangla, EachPaperPass, s)
	if each.Grade.Passed || each.Grade.Grade != "F" || each.Reasons[0] != "2nd paper below pass mark" {
		t.Errorf("example 4 each paper: %+v", each)
	}
}

// Example 7: annual results.
func TestAnnualExamples(t *testing.T) {
	s := bdDefault()
	terms := []Score{{7200, 10000, Present}, {6600, 10000, Present}, {8450, 10000, Present}}
	for _, c := range []struct {
		method  AnnualMethod
		weights []int
		want    Marks
		grade   string
	}{
		{FinalOnly, nil, 8450, "A+"},
		{Equal, nil, 7417, "A"},
		{Weighted, []int{25, 25, 50}, 7675, "A"},
	} {
		m, err := AnnualMark(terms, c.method, c.weights, 10000, HalfUp, false)
		g, _ := GradeSubject(m, 10000, 3300, s)
		if err != nil || m != c.want || g.Grade != c.grade {
			t.Errorf("method %d: %s %s %v, want %s %s", c.method, m, g.Grade, err, c.want, c.grade)
		}
	}
	close := []Score{{7900, 10000, Present}, {7900, 10000, Present}, {8050, 10000, Present}}
	m, _ := AnnualMark(close, Weighted, []int{30, 30, 40}, 10000, HalfUp, false)
	w, _ := AnnualMark(close, Weighted, []int{30, 30, 40}, 10000, HalfUp, true)
	if m != 7960 || w != 8000 {
		t.Errorf("rounding sensitivity: %s, whole %s", m, w)
	}
	english := []Score{{8500, 10000, Present}, {8000, 10000, Present}, {2500, 10000, Present}}
	if m, _ := AnnualMark(english, Equal, nil, 10000, HalfUp, false); m != 6333 {
		t.Errorf("final must pass example: %s", m)
	}
	if _, err := AnnualMark(terms, Weighted, []int{30, 30, 30}, 10000, HalfUp, false); !errors.Is(err, ErrInvalid) {
		t.Error("weights not summing to 100 accepted")
	}
}

// Example 8: absent, exempt, incomplete and withheld.
func TestStatusExamples(t *testing.T) {
	s := bdDefault()
	five := func() []Line { // points 5 + 4 + 4 + 4 + 3 = 20
		return []Line{{9000, 10000, 3300, false, Present, false}, {7500, 10000, 3300, false, Present, false}, {7500, 10000, 3300, false, Present, false}, {7500, 10000, 3300, false, Present, false}, {5500, 10000, 3300, false, Present, false}}
	}
	pe := append(five(), Line{Full: 10000, Pass: 3300, Status: Exempt})
	r, err := CalculateWithStatuses(pe, s, Policy{}, AbsentScoresZero)
	if err != nil || r.Outcome != Passed || r.Result.GPA != 400 {
		t.Errorf("exempt subject leaves the denominator: %+v %v", r, err)
	}
	english := append(five(), Line{Full: 10000, Pass: 3300, Status: Absent})
	r, _ = CalculateWithStatuses(english, s, Policy{}, AbsentScoresZero)
	if r.Outcome != Failed || r.Result.GPA != 0 || r.Result.Grades[5].Grade != "F" {
		t.Errorf("absent fails: %+v", r)
	}
	r, _ = CalculateWithStatuses(english, s, Policy{}, AbsentIsMissing)
	if r.Outcome != OutcomeIncomplete {
		t.Errorf("absent as missing: %+v", r)
	}
	held := append(five(), Line{Full: 10000, Pass: 3300, Status: Withheld}, Line{Full: 10000, Pass: 3300, Status: Incomplete})
	if r, _ := CalculateWithStatuses(held, s, Policy{}, AbsentScoresZero); r.Outcome != OutcomeWithheld {
		t.Errorf("withheld wins over incomplete: %+v", r)
	}
	forced := append(five(), Line{Marks: 6600, Full: 10000, Pass: 3300, Failed: true})
	if r, _ := CalculateWithStatuses(forced, s, Policy{}, AbsentScoresZero); r.Outcome != Failed || r.Result.Grades[5].Grade != "F" {
		t.Errorf("component failure fails the result: %+v", r)
	}
	// A component-failed optional subject gives no bonus and does not fail.
	opt := append(five(), Line{Marks: 9000, Full: 10000, Pass: 3300, Optional: true, Failed: true})
	if r, _ := CalculateWithStatuses(opt, s, Policy{}, AbsentScoresZero); r.Outcome != Passed || r.Result.GPA != 400 {
		t.Errorf("failed optional subject: %+v", r)
	}
	// CT 2 exempt: the CT contribution uses CT 1 only.
	c, _ := Contribute(Contribution{Sources: []Score{{1700, 2000, Present}, {0, 2000, Exempt}}, Combine: Average, Worth: 2000}, AbsentScoresZero)
	if RoundMarks(c.Value, HalfUp) != 1700 {
		t.Errorf("exempt CT: %s", c.Value.FloatString(2))
	}
	// Every source exempt: the contribution leaves the full marks.
	m, full, _, _ := TermMark([]Contribution{
		{Sources: []Score{{0, 2000, Exempt}}, Combine: Average, Worth: 2000},
		{Sources: []Score{{6800, 10000, Present}}, Combine: Single, Worth: 8000},
	}, TermPolicy{})
	if m != 5440 || full != 8000 {
		t.Errorf("exempt contribution: %s/%s", m, full)
	}
	// Absent CT scores 0 by default.
	c, _ = Contribute(Contribution{Sources: []Score{{1700, 2000, Present}, {0, 2000, Absent}}, Combine: Average, Worth: 2000}, AbsentScoresZero)
	if RoundMarks(c.Value, HalfUp) != 850 {
		t.Errorf("absent CT: %s", c.Value.FloatString(2))
	}
}

// Example 9: merit ties.
func TestRankExample(t *testing.T) {
	entries := []Ranked{
		{"Arif", 0, 61000, false},
		{"Sumi", 483, 93000, true},
		{"Tanim", 500, 90500, true},
		{"Nila", 500, 91200, true},
		{"Rafi", 500, 91200, true},
	}
	want := map[RankMode]map[string]int{
		Competition: {"Rafi": 1, "Nila": 1, "Tanim": 3, "Sumi": 4, "Arif": 5},
		Dense:       {"Rafi": 1, "Nila": 1, "Tanim": 2, "Sumi": 3, "Arif": 4},
	}
	for mode, positions := range want {
		got := Rank(entries, mode)
		for k, v := range positions {
			if got[k] != v {
				t.Errorf("mode %d: %s at %d, want %d", mode, k, got[k], v)
			}
		}
	}
}
