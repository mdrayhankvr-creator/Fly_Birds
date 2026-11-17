package schoolsetup

import (
	"reflect"
	"strings"
	"testing"
)

func manifestTestReport() Report {
	return Report{
		SessionID: "11111111-1111-1111-1111-111111111111", Session: "2026",
		Sections: []Section{
			{ClassID: "22222222-2222-2222-2222-222222222222", Class: "SIX-06", ID: "33333333-3333-3333-3333-333333333333", Name: "A", Target: 15, Existing: 4, ToAdd: 11},
			{ClassID: "22222222-2222-2222-2222-222222222222", Class: "SIX-06", ID: "44444444-4444-4444-4444-444444444444", Name: "B", Target: 15, Existing: 0, ToAdd: 15},
		},
		StudentExisting: 4, StudentToAdd: 26, TeacherExisting: 1, TeacherToAdd: 14,
		Teachers: []Teacher{{EmployeeCode: "TEACHER-01"}},
		ProtectedAdmins: []AdminProtection{
			{Email: protectedAdminEmails[0], Present: true, Role: "admin", Active: true, ExcludedFromProposedChanges: true},
			{Email: protectedAdminEmails[1], Present: true, Role: "admin", Active: true, ExcludedFromProposedChanges: true},
		},
	}
}

func TestBuildManifestDeterministicTargetsAndSafety(t *testing.T) {
	r := manifestTestReport()
	first, err := buildManifestFromKeys(r, []string{"existing-4", "existing-1"}, []string{"TEACHER-01"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildManifestFromKeys(r, []string{"existing-1", "existing-4"}, []string{"TEACHER-01"})
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("generation changed with unchanged identifier set: %v", err)
	}
	if first.SchemaVersion != ManifestSchemaVersion || len(first.StudentCandidates) != 26 || len(first.TeacherCandidates) != 14 {
		t.Fatalf("unexpected counts: %d students, %d teachers", len(first.StudentCandidates), len(first.TeacherCandidates))
	}
	if first.StudentCandidates[0].AdmissionNo != studentIdentifier("2026", r.Sections[0].ID, 5) || first.StudentCandidates[10].AdmissionNo != studentIdentifier("2026", r.Sections[0].ID, 15) || first.StudentCandidates[11].AdmissionNo != studentIdentifier("2026", r.Sections[1].ID, 1) {
		t.Fatal("student ordering or 15-minus-existing arithmetic changed")
	}
	students, teachers := map[string]bool{}, map[string]bool{}
	for _, c := range first.StudentCandidates {
		if c.TestMarker != "TEST" || !strings.HasPrefix(c.AdmissionNo, "TEST-2026-") || students[c.AdmissionNo] {
			t.Fatalf("unsafe or duplicate student identifier: %+v", c)
		}
		students[c.AdmissionNo] = true
	}
	for _, c := range first.TeacherCandidates {
		if c.EmployeeCode == "TEACHER-01" || teachers[c.EmployeeCode] {
			t.Fatalf("existing teacher reused or duplicate code: %+v", c)
		}
		teachers[c.EmployeeCode] = true
	}
	if first.TeacherCandidates[0].EmployeeCode != "TEACHER-02" || first.TeacherCandidates[13].EmployeeCode != "TEACHER-15" {
		t.Fatal("teacher code sequence changed")
	}
	if len(first.AccountEmails) != 0 || len(first.StudentAdmissionNumbers) != 0 || len(first.TeacherEmployeeCodes) != 0 {
		t.Fatal("generated manifest contains account targets or legacy identifiers")
	}
}

func TestBuildManifestRefusesCollisionsAndUnsafeState(t *testing.T) {
	base := manifestTestReport()
	for _, tc := range []struct {
		name      string
		alter     func(*Report)
		admission []string
		employee  []string
	}{
		{"student collision", nil, []string{strings.ToLower(studentIdentifier("2026", base.Sections[0].ID, 5))}, []string{"TEACHER-01"}},
		{"employee collision", nil, nil, []string{"TEACHER-01", "TEACHER-02"}},
		{"missing sections", func(r *Report) { r.Sections = nil }, nil, []string{"TEACHER-01"}},
		{"excess enrollment", func(r *Report) { r.Sections[0].Existing = 16 }, nil, []string{"TEACHER-01"}},
		{"excess teachers", func(r *Report) { r.TeacherExisting = 16 }, nil, []string{"TEACHER-01"}},
		{"missing protected admin", func(r *Report) { r.ProtectedAdmins[0].Present = false }, nil, []string{"TEACHER-01"}},
		{"database duplicates", func(r *Report) {
			r.IdentifierConflicts = []IdentifierConflict{{Kind: "email", Value: "duplicate", Reason: "duplicate_in_database"}}
		}, nil, []string{"TEACHER-01"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			r.Sections = append([]Section(nil), base.Sections...)
			r.ProtectedAdmins = append([]AdminProtection(nil), base.ProtectedAdmins...)
			if tc.alter != nil {
				tc.alter(&r)
			}
			if _, err := buildManifestFromKeys(r, tc.admission, tc.employee); err == nil {
				t.Fatal("unsafe manifest state accepted")
			}
		})
	}
}
