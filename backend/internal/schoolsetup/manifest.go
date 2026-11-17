package schoolsetup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

const ManifestSchemaVersion = 2

// StudentCandidate is an identifier and a label, never an admission record.
// No name, gender, birth date, guardian, contact detail or login is generated.
type StudentCandidate struct {
	SessionID   string `json:"session_id"`
	Session     string `json:"session"`
	ClassID     string `json:"class_id"`
	Class       string `json:"class"`
	SectionID   string `json:"section_id"`
	Section     string `json:"section"`
	AdmissionNo string `json:"admission_no"`
	Label       string `json:"label"`
	TestMarker  string `json:"test_marker"`
}

// TeacherCandidate reserves only an employee code. Staff roster fields must
// come from the school in a later milestone.
type TeacherCandidate struct {
	EmployeeCode string `json:"employee_code"`
}

var teacherRosterRequired = []string{
	"teacher name", "designation", "joining date", "subject assignments",
	"qualifications and availability", "account-link decision",
}

func candidateAdmissions(candidates []StudentCandidate) []string {
	values := make([]string, 0, len(candidates))
	for _, c := range candidates {
		values = append(values, c.AdmissionNo)
	}
	return values
}

func candidateEmployeeCodes(candidates []TeacherCandidate) []string {
	values := make([]string, 0, len(candidates))
	for _, c := range candidates {
		values = append(values, c.EmployeeCode)
	}
	return values
}

// GenerateManifest reads the active session once, in the same transaction as
// collision checking. The caller must explicitly name the expected session.
func GenerateManifest(ctx context.Context, db *sql.DB, expectedSession string) (Manifest, Report, error) {
	if expectedSession == "" {
		return Manifest{}, Report{}, fmt.Errorf("expected session is required")
	}
	tx, err := beginReadOnly(ctx, db)
	if err != nil {
		return Manifest{}, Report{}, err
	}
	defer tx.Rollback()
	r, err := previewTx(ctx, tx, nil)
	if err != nil {
		return Manifest{}, Report{}, err
	}
	if r.Session != expectedSession {
		return Manifest{}, Report{}, fmt.Errorf("active session %q differs from requested session %q", r.Session, expectedSession)
	}
	m, err := buildManifestTx(ctx, tx, r)
	if err != nil {
		return Manifest{}, Report{}, err
	}
	if err := tx.Commit(); err != nil {
		return Manifest{}, Report{}, err
	}
	r.ManifestChecked = true
	r.ManifestStudents, r.ManifestTeachers = len(m.StudentCandidates), len(m.TeacherCandidates)
	for i, warning := range r.Warnings {
		if warning == "candidate identifier conflicts not checked; provide --manifest with proposed identifiers" {
			r.Warnings = append(r.Warnings[:i], r.Warnings[i+1:]...)
			break
		}
	}
	return m, r, nil
}

func validateGeneratedManifest(ctx context.Context, tx *sql.Tx, given *Manifest, r Report) error {
	if given.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema version")
	}
	expected, err := buildManifestTx(ctx, tx, r)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(*given, expected) {
		return fmt.Errorf("manifest differs from current session, sections, counts or identifiers; review and generate a new file")
	}
	return nil
}

func buildManifestTx(ctx context.Context, tx *sql.Tx, r Report) (Manifest, error) {
	admissions, err := identifierKeys(ctx, tx, "SELECT lower(admission_no) FROM students ORDER BY lower(admission_no)")
	if err != nil {
		return Manifest{}, err
	}
	employees, err := identifierKeys(ctx, tx, "SELECT upper(employee_code) FROM employees ORDER BY upper(employee_code)")
	if err != nil {
		return Manifest{}, err
	}
	return buildManifestFromKeys(r, admissions, employees)
}

func buildManifestFromKeys(r Report, admissions, employees []string) (Manifest, error) {
	if len(r.Sections) == 0 {
		return Manifest{}, fmt.Errorf("active session has no active sections")
	}
	for _, section := range r.Sections {
		if section.Existing > StudentsPerSection {
			return Manifest{}, fmt.Errorf("%s %s has %d students, above target %d", section.Class, section.Name, section.Existing, StudentsPerSection)
		}
	}
	if r.TeacherExisting > TeacherTarget {
		return Manifest{}, fmt.Errorf("teacher profiles exceed target")
	}
	if len(r.ProtectedAdmins) != len(protectedAdminEmails) {
		return Manifest{}, fmt.Errorf("protected administrator check is incomplete")
	}
	if len(r.IdentifierConflicts) != 0 {
		return Manifest{}, fmt.Errorf("existing identifier conflicts require review before manifest generation")
	}
	for _, admin := range r.ProtectedAdmins {
		if !admin.Present || admin.Role != "admin" || !admin.Active {
			return Manifest{}, fmt.Errorf("protected administrator state requires review")
		}
	}
	admissions = append([]string(nil), admissions...)
	employees = append([]string(nil), employees...)
	sort.Strings(admissions)
	sort.Strings(employees)
	digestData := struct {
		SessionID  string
		Sections   []Section
		Teachers   []string
		Admissions []string
		Employees  []string
	}{SessionID: r.SessionID, Sections: r.Sections, Admissions: admissions, Employees: employees}
	for _, teacher := range r.Teachers {
		digestData.Teachers = append(digestData.Teachers, strings.ToUpper(teacher.EmployeeCode))
	}
	data, err := json.Marshal(digestData)
	if err != nil {
		return Manifest{}, err
	}
	hash := sha256.Sum256(data)
	m := Manifest{SchemaVersion: ManifestSchemaVersion, SessionID: r.SessionID, Session: r.Session, SnapshotSHA256: hex.EncodeToString(hash[:]), TeacherRosterRequired: append([]string(nil), teacherRosterRequired...)}
	m.StudentCandidates, err = buildStudentCandidates(r, admissions)
	if err != nil {
		return Manifest{}, err
	}
	usedEmployees := make(map[string]bool, len(employees))
	for _, value := range employees {
		usedEmployees[value] = true
	}
	existingTeachers := make(map[string]bool, len(r.Teachers))
	for _, teacher := range r.Teachers {
		existingTeachers[strings.ToUpper(teacher.EmployeeCode)] = true
	}
	for slot := 1; len(m.TeacherCandidates) < r.TeacherToAdd; slot++ {
		if slot > TeacherTarget {
			return Manifest{}, fmt.Errorf("teacher identifier range cannot meet target without collision")
		}
		code := fmt.Sprintf("TEACHER-%02d", slot)
		if !staff.ValidEmployeeCode(code) {
			return Manifest{}, fmt.Errorf("generated employee code does not meet staff rules")
		}
		if existingTeachers[code] {
			continue
		} // preserve existing teacher profile
		if usedEmployees[code] {
			return Manifest{}, fmt.Errorf("candidate employee code %s collides with an existing employee", code)
		}
		usedEmployees[code] = true
		m.TeacherCandidates = append(m.TeacherCandidates, TeacherCandidate{EmployeeCode: code})
	}
	return m, nil
}

// buildStudentCandidates is shared with the student-only apply milestone.
// Teacher targets and protected-account presence are separate setup concerns;
// they must not block a safe student shortfall preview.
func buildStudentCandidates(r Report, admissions []string) ([]StudentCandidate, error) {
	if len(r.Sections) == 0 || len(r.IdentifierConflicts) != 0 {
		return nil, fmt.Errorf("active sections or existing identifiers require review")
	}
	usedAdmissions := make(map[string]bool, len(admissions)+r.StudentToAdd)
	for _, value := range admissions {
		usedAdmissions[value] = true
	}
	candidates := make([]StudentCandidate, 0, r.StudentToAdd)
	for _, section := range r.Sections {
		if section.Existing > StudentsPerSection {
			return nil, fmt.Errorf("%s %s exceeds %d students", section.Class, section.Name, StudentsPerSection)
		}
		for slot := 1; slot <= section.ToAdd; slot++ {
			candidateNumber := section.Existing + slot
			id := studentIdentifier(r.Session, section.ID, candidateNumber)
			if !people.ValidAdmissionNumber(id) {
				return nil, fmt.Errorf("generated admission number does not meet admission rules")
			}
			key := strings.ToLower(id)
			if usedAdmissions[key] {
				return nil, fmt.Errorf("candidate admission number %s collides with an existing or proposed student", id)
			}
			usedAdmissions[key] = true
			candidates = append(candidates, StudentCandidate{
				SessionID: r.SessionID, Session: r.Session, ClassID: section.ClassID, Class: section.Class,
				SectionID: section.ID, Section: section.Name, AdmissionNo: id,
				Label: fmt.Sprintf("TEST ONLY - %s %s candidate %02d", section.Class, section.Name, candidateNumber), TestMarker: "TEST",
			})
		}
	}
	return candidates, nil
}

func identifierKeys(ctx context.Context, tx *sql.Tx, query string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func studentIdentifier(session, sectionID string, slot int) string {
	return fmt.Sprintf("TEST-%s-%s-%02d", strings.ToUpper(session), strings.ToUpper(strings.ReplaceAll(sectionID, "-", "")), slot)
}
