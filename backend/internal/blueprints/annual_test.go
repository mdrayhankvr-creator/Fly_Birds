package blueprints

import (
	"testing"
)

// annualOf builds one student whose single subject (Mathematics, on 100,
// pass 33) has the given term marks; "" leaves the term without the subject.
func annualOf(id string, marks ...string) AnnualStudent {
	st := AnnualStudent{ID: id, SectionID: "A"}
	for i, m := range marks {
		t := AnnualTermInput{TermID: termID(i), Outcome: "passed", Lines: []AnnualLineInput{}}
		if m != "" {
			passed := m >= "33.00" && len(m) >= 5
			t.Lines = append(t.Lines, AnnualLineInput{Key: "MATH", SubjectIDs: []string{"MATH"}, Status: "graded", Marks: m, Full: "100.00", Pass: "33.00", Passed: passed})
		}
		st.Terms = append(st.Terms, t)
	}
	return st
}

func termID(i int) string { return []string{"T1", "T2", "T3"}[i] }

func rulesOf(method string, weights ...int) AnnualRules {
	r := AnnualRules{Method: method, GPARounding: "half_up", Ranking: "competition"}
	for i := 0; i < 3; i++ {
		w := 0
		if len(weights) == 3 {
			w = weights[i]
		}
		r.Terms = append(r.Terms, AnnualTerm{ID: termID(i), Name: termID(i), Sequence: i + 1, Weight: w})
	}
	return r
}

// Example 7 of the calculation rules: First Term 72.00, Half-Yearly 66.00,
// Annual 84.50.
func TestAnnualExample7(t *testing.T) {
	s := scheme(t)
	cases := []struct {
		name         string
		rules        AnnualRules
		marks, grade string
	}{
		{"final only", rulesOf("final_only"), "84.50", "A+"},
		{"equal", rulesOf("equal"), "74.17", "A"},
		{"weighted 25/25/50", rulesOf("weighted", 25, 25, 50), "76.75", "A"},
	}
	for _, c := range cases {
		out, err := AnnualResults(c.rules, []AnnualStudent{annualOf("s", "72.00", "66.00", "84.50")}, s)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		line := out[0].Subjects[0]
		if line.Marks != c.marks || line.Grade != c.grade || out[0].Outcome != "passed" {
			t.Fatalf("%s: got %s %s %s", c.name, line.Marks, line.Grade, out[0].Outcome)
		}
	}
}

// Rounding sensitivity: weighted 30/30/40 of 79, 79 and 80.50 is 79.60 (A);
// with whole marks it is 80 (A+).
func TestAnnualRoundingSensitivity(t *testing.T) {
	s := scheme(t)
	r := rulesOf("weighted", 30, 30, 40)
	out, err := AnnualResults(r, []AnnualStudent{annualOf("s", "79.00", "79.00", "80.50")}, s)
	if err != nil || out[0].Subjects[0].Marks != "79.60" || out[0].Subjects[0].Grade != "A" {
		t.Fatalf("no rounding: %+v %v", out, err)
	}
	r.WholeMarks = true
	out, err = AnnualResults(r, []AnnualStudent{annualOf("s", "79.00", "79.00", "80.50")}, s)
	if err != nil || out[0].Subjects[0].Marks != "80.00" || out[0].Subjects[0].Grade != "A+" {
		t.Fatalf("whole marks: %+v %v", out, err)
	}
}

// final_must_pass: English 85, 80 and 25 (failed) averages 63.33 (A-); with
// the rule the subject fails for the year.
func TestAnnualFinalMustPass(t *testing.T) {
	s := scheme(t)
	st := annualOf("s", "85.00", "80.00", "25.00")
	st.Terms[2].Lines[0].Passed = false
	r := rulesOf("equal")
	out, err := AnnualResults(r, []AnnualStudent{st}, s)
	if err != nil || out[0].Subjects[0].Marks != "63.33" || out[0].Subjects[0].Grade != "A-" || out[0].Outcome != "passed" {
		t.Fatalf("off: %+v %v", out, err)
	}
	r.FinalMustPass = true
	out, err = AnnualResults(r, []AnnualStudent{st}, s)
	if err != nil || out[0].Subjects[0].Grade != "F" || out[0].Outcome != "failed" || out[0].GPA != "0.00" || out[0].Subjects[0].Reasons[0].Code != "final_term_failed" {
		t.Fatalf("on: %+v %v", out, err)
	}
}

// A term without the subject leaves its average; a withheld or missing term
// result withholds or blocks the annual result; positions rank the class.
func TestAnnualStatusesAndPositions(t *testing.T) {
	s := scheme(t)
	r := rulesOf("equal")
	out, err := AnnualResults(r, []AnnualStudent{
		annualOf("a", "", "70.00", "80.00"), // joined the subject in term 2: (70+80)/2 = 75
		annualOf("b", "90.00", "90.00", "90.00"),
		annualOf("c", "60.00", "60.00", "60.00"),
	}, s)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Subjects[0].Marks != "75.00" || out[0].Subjects[0].Terms[0].Status != "missing" || out[0].Subjects[0].Terms[0].Weight != 0 {
		t.Fatalf("missing term: %+v", out[0].Subjects[0])
	}
	if out[1].ClassPosition != 1 || out[0].ClassPosition != 2 || out[2].ClassPosition != 3 {
		t.Fatalf("positions: %d %d %d", out[0].ClassPosition, out[1].ClassPosition, out[2].ClassPosition)
	}
	withheld := annualOf("w", "70.00", "70.00", "70.00")
	withheld.Terms[1].Outcome = "withheld"
	missing := annualOf("m", "70.00", "70.00", "70.00")
	missing.Terms[0].Outcome = ""
	out, err = AnnualResults(r, []AnnualStudent{withheld, missing}, s)
	if err != nil || out[0].Outcome != "withheld" || out[1].Outcome != "incomplete" || out[0].ClassPosition != 0 {
		t.Fatalf("blocked: %+v %v", out, err)
	}
}

func TestParseMarksIsExact(t *testing.T) {
	for in, want := range map[string]int64{"16.83": 1683, "100.00": 10000, "0": 0, "7.5": 750} {
		got, err := parseMarks(in)
		if err != nil || int64(got) != want {
			t.Fatalf("%s: %d %v", in, got, err)
		}
	}
	for _, bad := range []string{"1.234", "-1", "x", ""} {
		if _, err := parseMarks(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
