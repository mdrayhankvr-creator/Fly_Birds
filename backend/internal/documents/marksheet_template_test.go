package documents

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
)

// An exam can be run the simple way: each subject marked out of its full
// marks with no component breakdown. That is what a school gets by creating
// an exam and adding subjects to it, with no blueprint, and it is the most
// common setup. Such a result has no component columns at all, which the
// marksheet is meant to print as totals only — and that is the case the
// template used to refuse, because the "Obtained marks" heading spanned
// `columns.len()` and Typst will not accept a span of zero. Every marksheet
// of such an exam failed to render, and the student saw "The document could
// not be prepared".
func plainAssessmentSet() exams.DocSet {
	inputs, _ := json.Marshal(exams.AssessmentInputs{Lines: []exams.InLine{
		{SubjectID: "s1", Code: "101", NameEN: "Bangla", NameBN: "বাংলা", Full: "100"},
		{SubjectID: "s2", Code: "109", NameEN: "Mathematics", NameBN: "গণিত", Full: "100"},
		{SubjectID: "s3", Code: "127", NameEN: "Science", NameBN: "বিজ্ঞান", Full: "100"},
	}})
	line := func(key, code, en, bn, marks, grade, point string) exams.OutLine {
		return exams.OutLine{Key: key, SubjectIDs: []string{key}, Code: code, NameEN: en, NameBN: bn,
			Status: "graded", Marks: text(marks), Full: "100", Grade: text(grade), Point: text(point)}
	}
	return exams.DocSet{
		ScopeRef:    exams.ScopeRef{Scope: exams.ScopeAssessment},
		ClassName:   "Class Six",
		SessionName: "2026",
		Title:       "Half Yearly Examination",
		Assessment:  &exams.AssessmentRules{Bands: []blueprints.BandRef{{Grade: "A+", Min: 80, Max: 100, GPA: 5}, {Grade: "A", Min: 70, Max: 79, GPA: 4}, {Grade: "F", Min: 0, Max: 32, GPA: 0}}},
		Results: []exams.DocResult{{
			Outcome: "passed",
			Inputs:  inputs,
			Outputs: exams.Outputs{Outcome: "passed", Subjects: []exams.OutLine{
				line("s1", "101", "Bangla", "বাংলা", "78.00", "A", "4.00"),
				line("s2", "109", "Mathematics", "গণিত", "85.00", "A+", "5.00"),
				line("s3", "127", "Science", "বিজ্ঞান", "71.00", "A", "4.00"),
			}},
		}},
	}
}

// TestPlainAssessmentHasNoComponentColumns pins the data contract the
// template has to cope with: no components means no columns and no parts.
func TestPlainAssessmentHasNoComponentColumns(t *testing.T) {
	set := plainAssessmentSet()
	lo := layoutOf(set)
	if cols := lo.columns("en"); len(cols) != 0 {
		t.Fatalf("columns: %v", cols)
	}
	if lo.showCT {
		t.Fatal("there are no class tests in this exam")
	}
	required, optional := lo.rows("en", set.Results[0], highest(set), nil)
	if len(required) != 3 || len(optional) != 0 {
		t.Fatalf("rows: %d required, %d optional", len(required), len(optional))
	}
	for _, r := range required {
		if len(r.Parts) != 0 {
			t.Fatalf("%s has parts with no components: %+v", r.Subject, r.Parts)
		}
	}
	// The marks still reach the sheet, in the subject's total column.
	if required[0].Total != "78" || required[1].Total != "85" || required[2].Highest != "71" {
		t.Fatalf("totals: %+v", required)
	}
	// What the renderer is handed must carry an empty list, not null: the
	// template calls .len() on it.
	body, err := json.Marshal(plainSheet(t, set))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"columns":[]`) || !strings.Contains(string(body), `"show_ct":false`) {
		t.Fatalf("sheet data: %s", firstBytes(string(body), 400))
	}
}

// plainSheet is a marksheet payload for the set, in the shape the service
// builds (sheetData), with one student.
func plainSheet(t *testing.T, set exams.DocSet) sheetData {
	t.Helper()
	l := lang("en")
	lo := layoutOf(set)
	required, optional := lo.rows(l, set.Results[0], highest(set), nil)
	if required == nil {
		required = []sheetRow{}
	}
	if optional == nil {
		optional = []sheetRow{}
	}
	cols := lo.columns(l)
	if cols == nil {
		cols = []string{}
	}
	return sheetData{
		Lang: string(l), Title: "Marksheet — " + set.ClassName, Heading: l.t("marksheet"), HeadingAlt: labelsEN["marksheet"],
		ExamName: set.Title, SessionName: set.SessionName, Labels: labelsFor(l), Columns: cols, ShowCT: lo.showCT,
		Grading: lo.grading(l),
		School: schoolData{Name: "Jalalabad High School", NameAlt: "জালালাবাদ উচ্চ বিদ্যালয়", Contact: "Sylhet",
			Accent: "#1d4ed8", Initials: "JH", Signatory: "Head Teacher", Border: true},
		Students: []sheetStudent{{
			Name: "Imran Rahman", NameAlt: "ইমরান রহমান", Father: "—", Mother: "—", DOB: "—",
			StudentID: "DEMO-012", Class: set.ClassName, Section: "B", Roll: "12", Session: set.SessionName,
			Exam: set.Title, Rows: required, Optional: optional,
			Total: "234", TotalMarks: "234.00", FullMarks: "300.00", GPA: "4.33", GPAWithout: "4.33",
			Letter: "A", Result: "Passed", FailedCount: "—", ClassPos: "3rd of 30", SectionPos: "1st of 15",
			Held: "—", PresentDays: "—", AbsentDays: "—", Published: "—",
			Signatures: []signature{{Title: "Class Teacher"}, {Title: "Head Teacher"}, {Title: "Guardian", Date: true}},
		}},
	}
}

func firstBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// testTypst is the real renderer when a Typst binary and the bundled fonts
// are to hand (TYPST_BIN, or typst on PATH, with DOCUMENT_FONTS_DIR or the
// repository's own font directory). Without one the test skips: this is the
// only check that compiles a template, so a fake renderer would prove
// nothing about it.
func testTypst(t *testing.T) *Typst {
	t.Helper()
	fonts := os.Getenv("DOCUMENT_FONTS_DIR")
	if fonts == "" {
		fonts = filepath.Join("..", "..", "assets", "fonts")
	}
	renderer, err := NewTypst(os.Getenv("TYPST_BIN"), fonts, 60*time.Second, 1)
	if err != nil {
		t.Skip("no typst binary or font directory; set TYPST_BIN and DOCUMENT_FONTS_DIR to compile the templates")
	}
	return renderer
}

// TestMarksheetTemplateRenders compiles the real template, which is the only
// way to catch a mistake in it. Both shapes matter: an exam with no
// components (one header row, no "Obtained marks" band) and one with them
// (two header rows, the band spanning the component columns).
func TestMarksheetTemplateRenders(t *testing.T) {
	renderer := testTypst(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	set := plainAssessmentSet()
	plain := plainSheet(t, set)
	if len(plain.Columns) != 0 {
		t.Fatalf("this case must have no component columns: %v", plain.Columns)
	}
	pdf, err := renderer.Render(ctx, Render{Template: "marksheet.typ", Data: plain})
	if err != nil {
		t.Fatalf("a marksheet of an exam with no components: %v", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") || Pages(pdf) != 1 {
		t.Fatalf("not a one-page PDF: %d bytes, %d page(s)", len(pdf), Pages(pdf))
	}

	// The same sheet with component columns still renders, so the fix did
	// not cost the two-row header.
	withComponents := plain
	withComponents.Columns = []string{"CQ", "MCQ"}
	withComponents.Students = append([]sheetStudent{}, plain.Students...)
	rows := append([]sheetRow{}, plain.Students[0].Rows...)
	for i := range rows {
		rows[i].Parts = []sheetCell{{Obt: "50", Full: "70"}, {Obt: "28", Full: "30"}}
	}
	withComponents.Students[0].Rows = rows
	pdf, err = renderer.Render(ctx, Render{Template: "marksheet.typ", Data: withComponents})
	if err != nil {
		t.Fatalf("a marksheet with component columns: %v", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF-") || Pages(pdf) != 1 {
		t.Fatalf("not a one-page PDF: %d bytes, %d page(s)", len(pdf), Pages(pdf))
	}

	// A class test column and no components is the other way to end up with
	// a single band column; it must render too.
	ct := plain
	ct.ShowCT = true
	ct.Students = append([]sheetStudent{}, plain.Students...)
	ctRows := append([]sheetRow{}, plain.Students[0].Rows...)
	for i := range ctRows {
		ctRows[i].CT = "18"
	}
	ct.Students[0].Rows = ctRows
	if _, err := renderer.Render(ctx, Render{Template: "marksheet.typ", Data: ct}); err != nil {
		t.Fatalf("a marksheet with a class-test column only: %v", err)
	}
}
