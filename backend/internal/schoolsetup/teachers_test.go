package schoolsetup

import (
	"reflect"
	"testing"
)

func TestTeacherIdentifiers(t *testing.T) {
	old := []Teacher{{EmployeeCode: "TEACHER-01"}, {EmployeeCode: "TEACHER-02"}}
	codes := []string{"TEACHER-01", "TEACHER-02"}
	first, err := teacherIdentifiers(2, old, codes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := teacherIdentifiers(2, old, codes)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 13 || first[0].EmployeeCode != "TEACHER-03" || first[12].EmployeeCode != "TEACHER-15" || !reflect.DeepEqual(first, second) {
		t.Fatalf("unexpected deterministic 15-minus-existing plan: %+v", first)
	}
	if _, err := teacherIdentifiers(2, old, append(codes, "TEACHER-03")); err == nil {
		t.Fatal("accepted non-teacher employee-code collision")
	}
	if _, err := teacherIdentifiers(16, old, codes); err == nil {
		t.Fatal("accepted over-target staff")
	}
	if _, err := teacherIdentifiers(2, old, append(codes, "teacher-02")); err == nil {
		t.Fatal("accepted case-insensitive duplicate")
	}
	if result, err := teacherIdentifiers(15, old, codes); err != nil || len(result) != 0 {
		t.Fatalf("target already met: %v %+v", err, result)
	}
}

func TestSyntheticTeacherNames(t *testing.T) {
	seen := map[string]bool{}
	for slot := 1; slot <= TeacherTarget; slot++ {
		en, bn := syntheticTeacherName(slot)
		if len(en) < 10 || len(bn) < 10 || en[:5] != "TEST " || seen[en] {
			t.Fatalf("invalid TEST name at %d", slot)
		}
		seen[en] = true
	}
}
