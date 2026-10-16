package people

import (
	"errors"
	"testing"
)

const testUUID = "123e4567-e89b-12d3-a456-426614174000"

func TestStudentAdmissionValidation(t *testing.T) {
	roll := 12
	good := StudentInput{AdmissionNo: "A-2026-1", NameEN: "Amina", NameBN: "আমিনা", DateOfBirth: "2014-01-20", Status: "active", SessionID: testUUID, ClassID: testUUID, SectionID: testUUID, RollNo: &roll}
	if _, err := normalizeStudent(good); err != nil {
		t.Fatalf("valid admission rejected: %v", err)
	}
	cases := map[string]func(*StudentInput){"empty admission": func(v *StudentInput) { v.AdmissionNo = "" }, "missing name": func(v *StudentInput) { v.NameEN = "" }, "bad section": func(v *StudentInput) { v.SectionID = "not-a-uuid" }, "invalid status": func(v *StudentInput) { v.Status = "deleted" }, "zero roll": func(v *StudentInput) { n := 0; v.RollNo = &n }, "future birth date": func(v *StudentInput) { v.DateOfBirth = "2999-01-01" }}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			input := good
			modify(&input)
			if _, err := normalizeStudent(input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}
}
func TestGuardianAndQueryValidation(t *testing.T) {
	if _, err := normalizeGuardian(GuardianInput{NameEN: "Rahim", Phone: "01700000000"}); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeGuardian(GuardianInput{NameEN: " "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank guardian name: %v", err)
	}
	if _, err := normalizeGuardian(GuardianInput{NameEN: "Rahim", Email: "invalid-address"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid guardian email: %v", err)
	}
	if validQuery(Query{Page: 1, PageSize: 101}) || validQuery(Query{Page: 0, PageSize: 10}) || validQuery(Query{Page: 1, PageSize: 10, SectionID: "bad"}) {
		t.Fatal("accepted invalid pagination or filter")
	}
}
