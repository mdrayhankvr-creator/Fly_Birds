package exams

import (
	"errors"
	"testing"
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
