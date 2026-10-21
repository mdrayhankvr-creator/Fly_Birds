package blueprints

import (
	"reflect"
	"sort"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
)

// bdBands is the bd_default grading scheme.
var bdBands = []calc.Band{
	{Grade: "A+", Min: 80, Max: 100, Point: 500}, {Grade: "A", Min: 70, Max: 79, Point: 400},
	{Grade: "A-", Min: 60, Max: 69, Point: 350}, {Grade: "B", Min: 50, Max: 59, Point: 300},
	{Grade: "C", Min: 40, Max: 49, Point: 200}, {Grade: "D", Min: 33, Max: 39, Point: 100},
	{Grade: "F", Min: 0, Max: 32, Point: 0},
}

func scheme(t *testing.T) calc.Scheme {
	t.Helper()
	s, err := calc.NewScheme(bdBands)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func f(v float64) *float64 { return &v }

var terms = map[string]Term{"T1": {Name: "First Term", Sequence: 1, Kind: "term"}, "T2": {Name: "Half-Yearly", Sequence: 2, Kind: "term"}, "T3": {Name: "Annual", Sequence: 3, Kind: "annual"}}

func total(full float64) []Component { return []Component{{Key: "TOTAL", Full: full}} }

// schoolA: CT 1 and CT 2 (20 each) averaged and worth 20, the term exam (100)
// worth 80, in three terms; annual weighted 25 / 25 / 50.
func schoolA(subjects ...string) Definition {
	d := Definition{Annual: Annual{Method: "weighted", Weights: map[string]int{"T1": 25, "T2": 25, "T3": 50}}}
	for _, tm := range []string{"T1", "T2", "T3"} {
		d.Assessments = append(d.Assessments,
			Assessment{Key: "CT1_" + tm, Name: "CT 1", TermID: tm, Kind: "class_test", Visibility: "hidden"},
			Assessment{Key: "CT2_" + tm, Name: "CT 2", TermID: tm, Kind: "class_test", Visibility: "hidden"},
			Assessment{Key: "EX_" + tm, Name: "Term exam", TermID: tm, Kind: "term_exam", Visibility: "with_assessment_publication"})
	}
	for _, id := range subjects {
		s := SubjectRule{SubjectID: id, Role: "required"}
		for _, tm := range []string{"T1", "T2", "T3"} {
			s.Sheets = append(s.Sheets, Sheet{Assessment: "CT1_" + tm, Pass: 7, Components: total(20)}, Sheet{Assessment: "CT2_" + tm, Pass: 7, Components: total(20)}, Sheet{Assessment: "EX_" + tm, Pass: 33, Components: total(100)})
			s.Terms = append(s.Terms, TermRule{TermID: tm, Full: 100, Pass: 33, Contributions: []Contribution{
				{From: []string{"CT1_" + tm, "CT2_" + tm}, Combine: "average", Worth: 20},
				{From: []string{"EX_" + tm}, Combine: "single", Worth: 80},
			}})
		}
		d.Subjects = append(d.Subjects, s)
	}
	d.Defaults()
	return d
}

// schoolB: three CTs, the best two averaged and worth 20, term exam 80.
func schoolB(subjects ...string) Definition {
	d := Definition{Assessments: []Assessment{
		{Key: "CT1", Name: "CT 1", TermID: "T1", Kind: "class_test", Visibility: "hidden"},
		{Key: "CT2", Name: "CT 2", TermID: "T1", Kind: "class_test", Visibility: "hidden"},
		{Key: "CT3", Name: "CT 3", TermID: "T1", Kind: "class_test", Visibility: "hidden"},
		{Key: "TERM1", Name: "Term exam", TermID: "T1", Kind: "term_exam", Visibility: "with_assessment_publication"},
	}}
	for _, id := range subjects {
		d.Subjects = append(d.Subjects, SubjectRule{SubjectID: id, Role: "required",
			Sheets: []Sheet{{Assessment: "CT1", Pass: 7, Components: total(20)}, {Assessment: "CT2", Pass: 7, Components: total(20)}, {Assessment: "CT3", Pass: 7, Components: total(20)}, {Assessment: "TERM1", Pass: 33, Components: total(100)}},
			Terms: []TermRule{{TermID: "T1", Full: 100, Pass: 33, Contributions: []Contribution{
				{From: []string{"CT1", "CT2", "CT3"}, Combine: "best", Best: 2, Worth: 20},
				{From: []string{"TERM1"}, Combine: "single", Worth: 80},
			}}}})
	}
	d.Defaults()
	return d
}

// schoolC: final only, one assessment per term (today's model).
func schoolC(subjects ...string) Definition {
	d := Definition{Assessments: []Assessment{{Key: "TERM1", Name: "Half-Yearly", TermID: "T1", Kind: "term_exam", Visibility: "with_assessment_publication"}}}
	for _, id := range subjects {
		d.Subjects = append(d.Subjects, finalOnly(id, "required", total(100)))
	}
	d.Defaults()
	return d
}

func finalOnly(id, role string, components []Component) SubjectRule {
	full := 0.0
	for _, c := range components {
		full += c.Full
	}
	return SubjectRule{SubjectID: id, Role: role, Sheets: []Sheet{{Assessment: "TERM1", Pass: 33, Components: components}},
		Terms: []TermRule{{TermID: "T1", Full: full, Pass: 33, Contributions: []Contribution{{From: []string{"TERM1"}, Combine: "single", Worth: full}}}}}
}

// schoolD: SSC-style components, Bangla papers combined, an optional subject.
func schoolD() Definition {
	d := schoolC()
	science := []Component{{Key: "CQ", Full: 50, Pass: f(17)}, {Key: "MCQ", Full: 25, Pass: f(8)}, {Key: "PRAC", Full: 25, Pass: f(8)}}
	d.Subjects = []SubjectRule{
		finalOnly("PHY", "required", science),
		finalOnly("ENG", "required", []Component{{Key: "CQ", Full: 70, Pass: f(23)}, {Key: "MCQ", Full: 30, Pass: f(10)}}),
		finalOnly("BAN1", "required", total(100)),
		finalOnly("BAN2", "required", total(100)),
		finalOnly("HMATH", "optional", science),
		finalOnly("AGRI", "optional", science),
		{SubjectID: "PE", Role: "non_graded"},
	}
	d.Subjects[2].CombinedKey, d.Subjects[3].CombinedKey = "BAN", "BAN"
	return d
}

func classContext(ids ...string) Context {
	c := Context{Terms: terms, Subjects: map[string]ClassSubject{}, Bands: bdBands}
	for _, id := range ids {
		c.Subjects[id] = ClassSubject{Name: id, Fourth: id == "HMATH" || id == "AGRI"}
	}
	return c
}

func codes(issues []Issue) []string {
	out := []string{}
	for _, i := range issues {
		out = append(out, i.Code)
	}
	sort.Strings(out)
	return out
}

func TestSchoolsAToDAreValid(t *testing.T) {
	for name, c := range map[string]struct {
		d   Definition
		ctx Context
	}{
		"A": {schoolA("MATH", "ENG"), classContext("MATH", "ENG")},
		"B": {schoolB("MATH"), classContext("MATH")},
		"C": {schoolC("MATH", "ENG"), classContext("MATH", "ENG")},
		"D": {schoolD(), classContext("PHY", "ENG", "BAN1", "BAN2", "HMATH", "AGRI", "PE")},
	} {
		if issues := Validate(c.d, c.ctx); len(issues) > 0 {
			t.Errorf("school %s: %+v", name, issues)
		}
	}
}

func TestValidationNamesEveryProblem(t *testing.T) {
	mutate := func(fn func(*Definition)) Definition {
		d := schoolB("MATH")
		fn(&d)
		return d
	}
	for name, c := range map[string]struct {
		d    Definition
		ctx  Context
		want []string
	}{
		"worth does not add up": {mutate(func(d *Definition) { d.Subjects[0].Terms[0].Contributions[1].Worth = 70 }), classContext("MATH"), []string{"worth_sum"}},
		"best N too large":      {mutate(func(d *Definition) { d.Subjects[0].Terms[0].Contributions[0].Best = 4 }), classContext("MATH"), []string{"best_invalid"}},
		"source from another term": {mutate(func(d *Definition) {
			d.Assessments = append(d.Assessments, Assessment{Key: "X", Name: "X", TermID: "T2", Kind: "class_test", Visibility: "hidden"})
			d.Subjects[0].Sheets = append(d.Subjects[0].Sheets, Sheet{Assessment: "X", Pass: 1, Components: total(10)})
			d.Subjects[0].Terms[0].Contributions[0].From = append(d.Subjects[0].Terms[0].Contributions[0].From, "X")
		}), classContext("MATH"), []string{"source_other_term", "term_rule_missing"}},
		"component pass above full": {mutate(func(d *Definition) {
			d.Subjects[0].Sheets[3].Components = []Component{{Key: "CQ", Full: 60, Pass: f(70)}, {Key: "MCQ", Full: 40}}
		}), classContext("MATH"), []string{"component_pass_invalid"}},
		"three decimals":        {mutate(func(d *Definition) { d.Subjects[0].Sheets[0].Components[0].Full = 20.005 }), classContext("MATH"), []string{"marks_invalid"}},
		"unused sheet":          {mutate(func(d *Definition) { d.Subjects[0].Terms[0].Contributions[0].From = []string{"CT1", "CT2"} }), classContext("MATH"), []string{"sheet_unused"}},
		"duplicate key":         {mutate(func(d *Definition) { d.Assessments[1].Key = "CT1" }), classContext("MATH"), []string{"assessment_key_duplicate", "sheet_assessment_unknown", "source_unknown"}},
		"unknown term":          {mutate(func(d *Definition) { d.Assessments[0].TermID = "T9" }), classContext("MATH"), []string{"assessment_term_unknown", "source_other_term", "term_rule_missing"}},
		"subject missing":       {schoolB("MATH"), classContext("MATH", "ENG"), []string{"coverage_missing"}},
		"not a class subject":   {schoolB("MATH", "ART"), classContext("MATH"), []string{"subject_unknown"}},
		"optional required":     {mutate(func(d *Definition) { d.Subjects[0].Role = "optional" }), classContext("MATH"), []string{"optional_not_fourth"}},
		"single combined paper": {mutate(func(d *Definition) { d.Subjects[0].CombinedKey = "BAN" }), classContext("MATH"), []string{"combined_single"}},
		"annual weights":        {mutate(func(d *Definition) { d.Annual = Annual{Method: "weighted", Weights: map[string]int{"T1": 90}} }), classContext("MATH"), []string{"annual_weights_sum"}},
		"policy":                {mutate(func(d *Definition) { d.Policy.Ranking = "random" }), classContext("MATH"), []string{"policy_invalid"}},
		"no assessments":        {Definition{PaperRule: "combined", Annual: Annual{Method: "final_only"}, Policy: Policy{GPARounding: "half_up", Absent: "zero", Ranking: "competition"}}, Context{Terms: terms, Bands: bdBands}, []string{"no_assessments"}},
		"grading gap":           {schoolB("MATH"), Context{Terms: terms, Subjects: map[string]ClassSubject{"MATH": {Name: "MATH"}}, Bands: bdBands[:6]}, []string{"bands_invalid"}},
	} {
		got := codes(Validate(c.d, c.ctx))
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

// The message names the subject, the term and both sums, as the
// specification asks ("Physics · Term 1: … = 90, expected 100").
func TestWorthSumMessageNamesSubjectAndTerm(t *testing.T) {
	d := schoolD()
	d.Subjects[0].Terms[0].Contributions[0].Worth = 90
	issues := Validate(d, classContext("PHY", "ENG", "BAN1", "BAN2", "HMATH", "AGRI", "PE"))
	if len(issues) != 1 || issues[0].Code != "worth_sum" || issues[0].Path != "subjects.0.terms.0.contributions" ||
		!reflect.DeepEqual(issues[0].Params, map[string]string{"subject": "PHY", "term": "First Term", "sum": "90.00", "full": "100.00"}) ||
		issues[0].Message != "PHY · First Term: contributions add up to 90.00, expected 100.00" {
		t.Fatalf("%+v", issues)
	}
}

func TestCanonicalHashIsStable(t *testing.T) {
	a, b := schoolA("MATH"), schoolA("MATH")
	ha, err := a.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := b.SHA256()
	if ha != hb || len(ha) != 64 {
		t.Fatalf("%s %s", ha, hb)
	}
	b.Policy.WholeMarks = true
	if hc, _ := b.SHA256(); hc == ha {
		t.Fatal("a policy change must change the hash")
	}
}

// marks builds one student's sheet marks: assessment → subject → mark.
type sheetMarks map[string]map[string]Mark

func present(v, full float64) Mark { return Mark{Status: "present", Marks: f(v), Full: full} }

func student(id string, m sheetMarks) Student {
	return Student{ID: id, SectionID: "A", Marks: m}
}

func one(t *testing.T, d Definition, st Student) StudentResult {
	t.Helper()
	rs, err := TermResults(d, "T1", []Student{st}, scheme(t))
	if err != nil {
		t.Fatal(err)
	}
	return rs[0]
}

// Example 1: CTs 17/20 and 14/20 average to 77.5 % → 15.50; the term exam
// 68/100 × 80 → 54.40; the subject mark 69.90 is A- (A with whole marks).
func TestExampleOneCTPlusFinal(t *testing.T) {
	d := schoolA("MATH")
	st := student("s", sheetMarks{"CT1_T1": {"MATH": present(17, 20)}, "CT2_T1": {"MATH": present(14, 20)}, "EX_T1": {"MATH": present(68, 100)}})
	r := one(t, d, st)
	s := r.Subjects[0]
	if s.Marks != "69.90" || s.Full != "100.00" || s.Grade != "A-" || s.Point != "3.50" || s.Contributions[0].Value != "15.50" || s.Contributions[1].Value != "54.40" || r.Outcome != "passed" || r.GPA != "3.50" {
		t.Fatalf("%+v", r)
	}
	d.Policy.WholeMarks = true
	if s := one(t, d, st).Subjects[0]; s.Marks != "70.00" || s.Grade != "A" {
		t.Fatalf("whole marks: %+v", s)
	}
}

// Example 2: best two of 12, 18, 15 (each / 20) → 82.5 % → 16.50; term exam
// 70 → 56.00; 72.50 → A.
func TestExampleTwoBestTwo(t *testing.T) {
	r := one(t, schoolB("MATH"), student("s", sheetMarks{"CT1": {"MATH": present(12, 20)}, "CT2": {"MATH": present(18, 20)}, "CT3": {"MATH": present(15, 20)}, "TERM1": {"MATH": present(70, 100)}}))
	if s := r.Subjects[0]; s.Marks != "72.50" || s.Grade != "A" || s.Contributions[0].Value != "16.50" || s.Contributions[1].Value != "56.00" {
		t.Fatalf("%+v", s)
	}
}

// Example 3: Physics CQ 16/50 (pass 17) fails the subject despite 66/100.
func TestExampleThreeComponentFailure(t *testing.T) {
	d := schoolC()
	d.Subjects = []SubjectRule{
		finalOnly("PHY", "required", []Component{{Key: "CQ", Full: 50, Pass: f(17)}, {Key: "MCQ", Full: 25, Pass: f(8)}, {Key: "PRAC", Full: 25, Pass: f(8)}}),
		finalOnly("ENG", "required", total(100)),
	}
	phy := present(66, 100)
	phy.Components = []ComponentMark{{Key: "CQ", Marks: 16, Full: 50, Pass: f(17)}, {Key: "MCQ", Marks: 25, Full: 25, Pass: f(8)}, {Key: "PRAC", Marks: 25, Full: 25, Pass: f(8)}}
	r := one(t, d, student("s", sheetMarks{"TERM1": {"PHY": phy, "ENG": present(90, 100)}}))
	s := r.Subjects[0]
	if s.Marks != "66.00" || s.Grade != "F" || s.Passed || len(s.Reasons) != 1 || s.Reasons[0].Component != "CQ" || r.Outcome != "failed" || r.GPA != "0.00" {
		t.Fatalf("%+v", r)
	}
}

// Example 4: Bangla 1st 58 + 2nd 30 = 88 / 200 → C passes on the combined
// rule and fails when each paper must pass.
func TestExampleFourCombinedPapers(t *testing.T) {
	d := schoolC()
	d.Subjects = []SubjectRule{finalOnly("BAN1", "required", total(100)), finalOnly("BAN2", "required", total(100))}
	d.Subjects[0].CombinedKey, d.Subjects[1].CombinedKey = "BAN", "BAN"
	st := student("s", sheetMarks{"TERM1": {"BAN1": present(58, 100), "BAN2": present(30, 100)}})
	r := one(t, d, st)
	if len(r.Subjects) != 1 || r.Subjects[0].Marks != "88.00" || r.Subjects[0].Full != "200.00" || r.Subjects[0].Grade != "C" || r.GPA != "2.00" || r.Outcome != "passed" {
		t.Fatalf("combined: %+v", r)
	}
	d.PaperRule = "each_paper"
	if r := one(t, d, st); r.Subjects[0].Grade != "F" || r.Outcome != "failed" || r.Subjects[0].Reasons[0].Code != "paper_below_pass" {
		t.Fatalf("each paper: %+v", r)
	}
}

// Example 5: six required subjects A+, A+, A, A, A-, B (24.5 points) and an
// optional A: (24.5 + 2) / 6 = 4.4166… → 4.42 half up, 4.41 truncated; an
// optional F adds nothing and does not fail.
func TestExampleFiveOptionalSubject(t *testing.T) {
	d := schoolC()
	marks := map[string]float64{"S1": 85, "S2": 90, "S3": 75, "S4": 72, "S5": 65, "S6": 55}
	m := map[string]Mark{}
	for id, v := range marks {
		d.Subjects = append(d.Subjects, finalOnly(id, "required", total(100)))
		m[id] = present(v, 100)
	}
	d.Subjects = append(d.Subjects, finalOnly("OPT", "optional", total(100)))
	m["OPT"] = present(74, 100)
	st := student("s", sheetMarks{"TERM1": m})
	if r := one(t, d, st); r.GPA != "4.42" || r.Outcome != "passed" {
		t.Fatalf("half up: %+v", r)
	}
	d.Policy.GPARounding = "truncate"
	if r := one(t, d, st); r.GPA != "4.41" {
		t.Fatalf("truncate: %+v", r)
	}
	m["OPT"] = present(10, 100)
	if r := one(t, d, st); r.GPA != "4.08" || r.Outcome != "passed" {
		t.Fatalf("optional F: %+v", r)
	}
}

// Example 8: CT 2 exempt leaves CT 1 (17/20 → 85 %) worth 17.00; an exempt
// term exam shrinks the full marks and scales the pass mark; a missing mark
// makes the result incomplete and unranked.
func TestExampleEightExemptAndMissing(t *testing.T) {
	d := schoolA("MATH")
	r := one(t, d, student("s", sheetMarks{"CT1_T1": {"MATH": present(17, 20)}, "CT2_T1": {"MATH": {Status: "exempt", Full: 20}}, "EX_T1": {"MATH": present(50, 100)}}))
	if c := r.Subjects[0].Contributions[0]; c.Value != "17.00" {
		t.Fatalf("exempt CT: %+v", c)
	}
	r = one(t, d, student("s", sheetMarks{"CT1_T1": {"MATH": present(10, 20)}, "CT2_T1": {"MATH": present(10, 20)}, "EX_T1": {"MATH": {Status: "exempt", Full: 100}}}))
	if s := r.Subjects[0]; s.Marks != "10.00" || s.Full != "20.00" || !s.Passed {
		t.Fatalf("exempt term exam: %+v", s)
	}
	r = one(t, d, student("s", sheetMarks{"CT1_T1": {"MATH": present(10, 20)}, "EX_T1": {"MATH": present(50, 100)}}))
	if r.Outcome != "incomplete" || r.ClassPosition != 0 || r.Subjects[0].Status != "incomplete" {
		t.Fatalf("missing: %+v", r)
	}
}

func TestAbsencePolicies(t *testing.T) {
	d := schoolC("MATH", "ENG")
	st := student("s", sheetMarks{"TERM1": {"MATH": {Status: "absent", Full: 100}, "ENG": present(80, 100)}})
	if r := one(t, d, st); r.Outcome != "failed" || r.Subjects[0].Marks != "0.00" || r.Subjects[0].Grade != "F" {
		t.Fatalf("absent scores zero: %+v", r)
	}
	d.Policy.Absent = "missing"
	if r := one(t, d, st); r.Outcome != "incomplete" {
		t.Fatalf("absent is missing: %+v", r)
	}
}

// Students who did not choose an optional subject do not take it.
func TestOptionalChoice(t *testing.T) {
	d := schoolD()
	m := map[string]Mark{"PHY": present(60, 100), "ENG": present(70, 100), "BAN1": present(60, 100), "BAN2": present(60, 100), "HMATH": present(90, 100)}
	for _, k := range []string{"PHY", "HMATH"} {
		x := m[k]
		x.Components = []ComponentMark{{Key: "CQ", Marks: x.Marks2(0.5), Full: 50, Pass: f(17)}, {Key: "MCQ", Marks: x.Marks2(0.25), Full: 25, Pass: f(8)}, {Key: "PRAC", Marks: x.Marks2(0.25), Full: 25, Pass: f(8)}}
		m[k] = x
	}
	e := m["ENG"]
	e.Components = []ComponentMark{{Key: "CQ", Marks: 49, Full: 70, Pass: f(23)}, {Key: "MCQ", Marks: 21, Full: 30, Pass: f(10)}}
	m["ENG"] = e
	st := student("s", sheetMarks{"TERM1": m})
	st.Takes = func(id string) bool { return id != "AGRI" }
	r := one(t, d, st)
	if len(r.Subjects) != 4 || r.Outcome != "passed" {
		t.Fatalf("%+v", r)
	}
	opt := r.Subjects[3]
	if !opt.Optional || opt.SubjectIDs[0] != "HMATH" || opt.Grade != "A+" {
		t.Fatalf("optional: %+v", opt)
	}
	// PHY A- 3.5, ENG A 4, BAN 60 % A- 3.5 → 11 / 3 = 3.666…; HMATH A+ adds 3 → 14 / 3 = 4.67.
	if r.GPA != "4.67" {
		t.Fatalf("gpa %s", r.GPA)
	}
}

func (m Mark) Marks2(share float64) float64 { return *m.Marks * share }

// Competition and dense ranking, by section and class; a tie shares a place.
func TestTermPositions(t *testing.T) {
	d := schoolC("MATH")
	mk := func(id, section string, v float64) Student {
		return Student{ID: id, SectionID: section, Marks: sheetMarks{"TERM1": {"MATH": present(v, 100)}}}
	}
	students := []Student{mk("a", "A", 90), mk("b", "B", 90), mk("c", "A", 70), mk("d", "B", 20)}
	rs, err := TermResults(d, "T1", students, scheme(t))
	if err != nil {
		t.Fatal(err)
	}
	pos := map[string][2]int{}
	for _, r := range rs {
		pos[r.StudentID] = [2]int{r.ClassPosition, r.SectionPosition}
	}
	if !reflect.DeepEqual(pos, map[string][2]int{"a": {1, 1}, "b": {1, 1}, "c": {3, 2}, "d": {4, 2}}) {
		t.Fatalf("competition: %v", pos)
	}
	d.Policy.Ranking = "dense"
	rs, _ = TermResults(d, "T1", students, scheme(t))
	for _, r := range rs {
		if r.StudentID == "c" && r.ClassPosition != 2 {
			t.Fatalf("dense: %+v", r)
		}
	}
}
