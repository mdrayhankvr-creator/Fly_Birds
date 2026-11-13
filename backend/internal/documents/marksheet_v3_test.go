package documents

import (
	"encoding/json"
	"testing"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

func text(s string) *string { return &s }

// An exam's own marksheet takes its component columns from the published
// lines (the blueprint's components): the usual order (CQ, MCQ, practical),
// a component below its own pass marks marked failed, and combined subjects
// one row per paper from the inputs.
func TestAssessmentLayoutColumnsFromComponents(t *testing.T) {
	inputs, err := json.Marshal(exams.AssessmentInputs{Lines: []exams.InLine{
		{SubjectID: "s1", Code: "101", NameEN: "Bangla 1st", Full: "100", Components: []exams.OutComponent{
			{Key: "CQ", Marks: text("40.00"), Full: "70", Pass: text("23")},
			{Key: "MCQ", Marks: text("20.00"), Full: "30", Pass: text("10")},
		}},
		{SubjectID: "s2", Code: "102", NameEN: "Bangla 2nd", Full: "100", Components: []exams.OutComponent{
			{Key: "CQ", Marks: text("30.00"), Full: "70", Pass: text("23")},
			{Key: "MCQ", Marks: text("8.00"), Full: "30", Pass: text("10")},
		}},
		{SubjectID: "s3", Code: "127", NameEN: "Science", Full: "100", Components: []exams.OutComponent{
			{Key: "PRAC", Marks: text("20.00"), Full: "25", Pass: text("8")},
			{Key: "CQ", Marks: text("50.00"), Full: "75", Pass: text("25")},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	set := exams.DocSet{
		ScopeRef:   exams.ScopeRef{Scope: exams.ScopeAssessment},
		Assessment: &exams.AssessmentRules{Bands: []blueprints.BandRef{{Grade: "A+", Min: 80, Max: 100, GPA: 5}, {Grade: "F", Min: 0, Max: 32, GPA: 0}}},
		Results: []exams.DocResult{{
			Outcome: "passed",
			Inputs:  inputs,
			Outputs: exams.Outputs{Outcome: "passed", Subjects: []exams.OutLine{
				{Key: "combined", SubjectIDs: []string{"s1", "s2"}, Code: "101", NameEN: "Bangla", Status: "graded", Marks: text("98.00"), Full: "200",
					Grade: text("C"), Point: text("2.00"), Components: []exams.OutComponent{
						{Key: "CQ", Marks: text("70.00"), Full: "140", Pass: text("46")},
						{Key: "MCQ", Marks: text("28.00"), Full: "60", Pass: text("20")},
					}},
				{Key: "science", SubjectIDs: []string{"s3"}, Code: "127", NameEN: "Science", Status: "graded", Marks: text("70.00"), Full: "100",
					Grade: text("A-"), Point: text("3.50"), Components: []exams.OutComponent{
						{Key: "PRAC", Marks: text("20.00"), Full: "25", Pass: text("8")},
						{Key: "CQ", Marks: text("50.00"), Full: "75", Pass: text("25")},
					}},
			}},
		}},
	}
	lo := layoutOf(set)
	cols := lo.columns("en")
	if len(cols) != 3 || cols[0] != "CQ" || cols[1] != "MCQ" || cols[2] != "Prac." {
		t.Fatalf("columns: %v", cols)
	}
	required, optional := lo.rows("en", set.Results[0], highest(set), nil)
	if len(required) != 2 || len(optional) != 0 {
		t.Fatalf("rows: %d required, %d optional", len(required), len(optional))
	}
	bangla := required[0]
	if len(bangla.Parts) != 3 || bangla.Parts[0].Obt != "70" || bangla.Parts[0].Full != "140" || bangla.Parts[2].Obt != "—" {
		t.Fatalf("combined parts: %+v", bangla.Parts)
	}
	if len(bangla.Papers) != 2 || bangla.Papers[1].Subject != "Bangla 2nd" {
		t.Fatalf("papers: %+v", bangla.Papers)
	}
	// The second paper's MCQ (8 of 30, pass 10) failed; the first paper's did not.
	if !bangla.Papers[1].Parts[1].Failed || bangla.Papers[0].Parts[1].Failed {
		t.Fatalf("failed components: %+v %+v", bangla.Papers[0].Parts, bangla.Papers[1].Parts)
	}
	science := required[1]
	if science.Parts[0].Obt != "50" || science.Parts[1].Obt != "—" || science.Parts[2].Obt != "20" || science.Parts[2].Failed {
		t.Fatalf("science parts: %+v", science.Parts)
	}
	if science.Highest != "70" || bangla.Full != "200" {
		t.Fatalf("highest %q full %q", science.Highest, bangla.Full)
	}
}

// Merit positions print as ordinals: "3rd of 60", in Bangla "৬০ জনের মধ্যে ৩য়".
func TestOrdinalPositions(t *testing.T) {
	en, bn := lang("en"), lang("bn")
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 103: "103rd"} {
		if got := en.ordinal(n); got != want {
			t.Fatalf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
	for n, want := range map[int]string{1: "১ম", 3: "৩য়", 6: "৬ষ্ঠ", 10: "১০ম", 11: "১১তম"} {
		if got := bn.ordinal(n); got != want {
			t.Fatalf("bn ordinal(%d) = %q, want %q", n, got, want)
		}
	}
	three := 3
	if got := posText(en, &three, 60); got != "3rd of 60" {
		t.Fatalf("posText en: %q", got)
	}
	if got := posText(bn, &three, 60); got != "৬০ জনের মধ্যে ৩য়" {
		t.Fatalf("posText bn: %q", got)
	}
}
