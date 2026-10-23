package people

import (
	"context"
	"errors"
	"strings"
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
func TestImportCSVRejectsNUL(t *testing.T) {
	s := NewService(nil)
	body := []byte("admission_no,name_en,name_bn,date_of_birth,roll_no,status\nA-1,Nul\x00Name,,2014-01-20,1,active\n")
	if _, err := s.ImportCSV(context.Background(), Actor{ID: testUUID, Role: "admin"}, testUUID, testUUID, testUUID, body); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NUL CSV: got %v, want invalid", err)
	}
}

func TestStudentProfileValidation(t *testing.T) {
	text := func(s string) *string { return &s }
	good := StudentInput{AdmissionNo: "A-1", NameEN: "Amina", DateOfBirth: "2014-01-20", Status: "active", SessionID: testUUID, ClassID: testUUID, SectionID: testUUID,
		StudentCode: text(" FB-1 "), Gender: text("female"), BloodGroup: text("AB-"), FatherNameBN: text("করিম"), AdmissionDate: text("2026-01-05")}
	v, err := normalizeStudent(good)
	if err != nil || *v.StudentCode != "FB-1" {
		t.Fatalf("valid profile rejected or not trimmed: %v %q", err, *v.StudentCode)
	}
	legacy := StudentInput{AdmissionNo: "A-1", NameEN: "Amina", Status: "active", SessionID: testUUID, ClassID: testUUID, SectionID: testUUID}
	if v, err := normalizeStudent(legacy); err != nil || v.Gender != nil || v.AdmissionDate != nil {
		t.Fatalf("omitted details must stay omitted: %v %+v", err, v)
	}
	cases := map[string]func(*StudentInput){
		"gender":           func(v *StudentInput) { v.Gender = text("unknown") },
		"blood group":      func(v *StudentInput) { v.BloodGroup = text("C+") },
		"long parent name": func(v *StudentInput) { v.FatherNameEN = text(strings.Repeat("a", 161)) },
		"long address":     func(v *StudentInput) { v.PresentAddress = text(strings.Repeat("ঠ", 200)) },
		"bad date":         func(v *StudentInput) { v.AdmissionDate = text("2026-13-01") },
		"before birth":     func(v *StudentInput) { v.AdmissionDate = text("2013-12-31") },
		"years ahead":      func(v *StudentInput) { v.AdmissionDate = text("2099-01-01") },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			input := good
			input.StudentCode, input.Gender, input.BloodGroup, input.FatherNameBN, input.AdmissionDate = text("FB-1"), text("female"), text("AB-"), text("করিম"), text("2026-01-05")
			modify(&input)
			if _, err := normalizeStudent(input); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v, want invalid", err)
			}
		})
	}
	if !IsCardToken("AbCdEfGhIjKlMnOpQrSt-_") || IsCardToken("short") || IsCardToken("AbCdEfGhIjKlMnOpQrSt-'") {
		t.Fatal("card token shape check")
	}
}
