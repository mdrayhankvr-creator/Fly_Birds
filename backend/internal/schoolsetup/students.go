package schoolsetup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/people"
)

var (
	ErrStudentSetupForbidden = errors.New("student setup requires an administrator with student and user creation grants")
	ErrStudentSetupStale     = errors.New("student setup preview is stale; preview again")
	ErrStudentSetupKeyReused = errors.New("student setup key was used for another preview")
	ErrStudentSetupInvalid   = errors.New("invalid student setup request")
	ErrStudentSetupConflict  = errors.New("student setup identifiers or school state conflict")
	studentSetupUUID         = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	studentSetupHash         = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type StudentSetupConflictError struct{ Conflicts []IdentifierConflict }

func (e *StudentSetupConflictError) Error() string { return ErrStudentSetupConflict.Error() }
func (e *StudentSetupConflictError) Unwrap() error { return ErrStudentSetupConflict }

// PlannedStudent contains a clearly marked synthetic bilingual name, gender
// and identifiers. Other personal, guardian and subject fields stay incomplete.
type PlannedStudent struct {
	StudentCandidate
	NameEN  string `json:"name_en"`
	NameBN  string `json:"name_bn"`
	Gender  string `json:"gender"`
	RollNo  int    `json:"roll_no"`
	LoginID string `json:"login_id"`
}

type StudentPlan struct {
	SessionID   string           `json:"session_id"`
	Session     string           `json:"session"`
	Existing    int              `json:"existing"`
	ToAdd       int              `json:"to_add"`
	Sections    []Section        `json:"sections"`
	Students    []PlannedStudent `json:"students"`
	Fingerprint string           `json:"fingerprint"`
	Incomplete  []string         `json:"incomplete"`
}

type StudentApplyRequest struct {
	SessionID      string `json:"session_id"`
	Fingerprint    string `json:"fingerprint"`
	IdempotencyKey string `json:"idempotency_key"`
	Confirm        bool   `json:"confirm"`
}

type StudentApplyResult struct {
	Admitted int                   `json:"admitted"`
	Replayed bool                  `json:"replayed"`
	Logins   []people.StudentLogin `json:"logins"`
}

type studentQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireStudentSetupAdmin(ctx context.Context, q studentQueryer, actor string) error {
	if !studentSetupUUID.MatchString(actor) {
		return ErrStudentSetupForbidden
	}
	var count int
	err := q.QueryRowContext(ctx, `SELECT count(DISTINCT rp.permission_id) FROM users u
JOIN role_permissions rp ON rp.role_id=u.role
WHERE u.id=$1 AND u.is_active AND u.role='admin'
AND rp.permission_id IN ('students:manage','users:create')`, actor).Scan(&count)
	if err != nil {
		return err
	}
	if count != 2 {
		return ErrStudentSetupForbidden
	}
	return nil
}

// PreviewStudents reads a consistent, read-only snapshot. It reuses the
// school-setup identifier manifest's target arithmetic and collision checks.
func PreviewStudents(ctx context.Context, db *sql.DB, actor, expectedSession string) (StudentPlan, error) {
	tx, err := beginReadOnly(ctx, db)
	if err != nil {
		return StudentPlan{}, err
	}
	defer tx.Rollback()
	if err := requireStudentSetupAdmin(ctx, tx, actor); err != nil {
		return StudentPlan{}, err
	}
	plan, err := studentPlanTx(ctx, tx, expectedSession)
	if err != nil {
		return StudentPlan{}, err
	}
	return plan, tx.Commit()
}

func studentPlanTx(ctx context.Context, tx *sql.Tx, expectedSession string) (StudentPlan, error) {
	if expectedSession == "" {
		return StudentPlan{}, ErrStudentSetupInvalid
	}
	report, err := previewTx(ctx, tx, nil)
	if err != nil {
		return StudentPlan{}, err
	}
	if report.SessionID != expectedSession {
		return StudentPlan{}, ErrStudentSetupStale
	}
	if len(report.IdentifierConflicts) != 0 {
		return StudentPlan{}, &StudentSetupConflictError{Conflicts: report.IdentifierConflicts}
	}
	var conflicts []IdentifierConflict
	if len(report.Sections) == 0 {
		conflicts = append(conflicts, IdentifierConflict{Kind: "section", Value: report.Session, Reason: "no_active_sections"})
	}
	for _, section := range report.Sections {
		if section.Existing > StudentsPerSection {
			conflicts = append(conflicts, IdentifierConflict{Kind: "section", Value: section.Class + " / " + section.Name, Reason: "above_target"})
		}
	}
	if len(conflicts) > 0 {
		return StudentPlan{}, &StudentSetupConflictError{Conflicts: conflicts}
	}
	admissions, err := identifierKeys(ctx, tx, "SELECT lower(admission_no) FROM students ORDER BY lower(admission_no)")
	if err != nil {
		return StudentPlan{}, err
	}
	usedAdmissions := map[string]bool{}
	for _, number := range admissions {
		usedAdmissions[number] = true
	}
	for _, section := range report.Sections {
		for slot := 1; slot <= section.ToAdd; slot++ {
			id := studentIdentifier(report.Session, section.ID, section.Existing+slot)
			if usedAdmissions[strings.ToLower(id)] {
				conflicts = append(conflicts, IdentifierConflict{Kind: "admission_number", Value: id, Reason: "already_exists"})
			}
		}
	}
	if len(conflicts) > 0 {
		return StudentPlan{}, &StudentSetupConflictError{Conflicts: conflicts}
	}
	candidates, err := buildStudentCandidates(report, admissions)
	if err != nil {
		return StudentPlan{}, fmt.Errorf("%w: %v", ErrStudentSetupConflict, err)
	}
	plan := StudentPlan{
		SessionID: report.SessionID, Session: report.Session, Existing: report.StudentExisting,
		ToAdd: report.StudentToAdd, Sections: report.Sections, Students: make([]PlannedStudent, 0, len(candidates)),
		Incomplete: []string{"verified student names and dates of birth", "guardian and contact details", "group and optional subject choices"},
	}
	if plan.ToAdd > people.MaxBulkRows {
		return StudentPlan{}, ErrStudentSetupInvalid
	}
	usedRolls := map[string]map[int]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT section_id::text,roll_no FROM student_enrolments WHERE roll_no IS NOT NULL`)
	if err != nil {
		return StudentPlan{}, err
	}
	for rows.Next() {
		var section string
		var roll int
		if err := rows.Scan(&section, &roll); err != nil {
			rows.Close()
			return StudentPlan{}, err
		}
		if usedRolls[section] == nil {
			usedRolls[section] = map[int]bool{}
		}
		usedRolls[section][roll] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return StudentPlan{}, err
	}
	rows.Close()
	loginIDs, err := identifierKeys(ctx, tx, "SELECT lower(login_id) FROM users WHERE login_id IS NOT NULL ORDER BY lower(login_id)")
	if err != nil {
		return StudentPlan{}, err
	}
	usedIDs := map[string]bool{}
	for _, id := range loginIDs {
		usedIDs[id] = true
	}
	for index, candidate := range candidates {
		if usedIDs[strings.ToLower(candidate.AdmissionNo)] {
			return StudentPlan{}, &StudentSetupConflictError{Conflicts: []IdentifierConflict{{Kind: "login_id", Value: candidate.AdmissionNo, Reason: "already_exists"}}}
		}
		if candidate.TestMarker != "TEST" || !people.ValidAdmissionNumber(candidate.AdmissionNo) || !auth.ValidLoginID(candidate.AdmissionNo) {
			return StudentPlan{}, ErrStudentSetupInvalid
		}
		usedIDs[strings.ToLower(candidate.AdmissionNo)] = true
		if usedRolls[candidate.SectionID] == nil {
			usedRolls[candidate.SectionID] = map[int]bool{}
		}
		roll := 1
		for usedRolls[candidate.SectionID][roll] && roll <= 999999 {
			roll++
		}
		if roll > 999999 {
			return StudentPlan{}, ErrStudentSetupInvalid
		}
		usedRolls[candidate.SectionID][roll] = true
		nameEN, nameBN, gender := syntheticStudentName(index)
		plan.Students = append(plan.Students, PlannedStudent{StudentCandidate: candidate,
			NameEN: nameEN, NameBN: nameBN, Gender: gender, RollNo: roll, LoginID: candidate.AdmissionNo})
	}
	// Include the student manifest's identifiers and school state, assigned
	// rolls and login-ID namespace. Teacher setup has no bearing on this apply.
	sort.Strings(loginIDs)
	fingerprintInput, err := json.Marshal(struct {
		SessionID       string
		Sections        []Section
		Admissions      []string
		ProtectedAdmins []AdminProtection
		Students        []PlannedStudent
		LoginIDs        []string
	}{report.SessionID, report.Sections, admissions, report.ProtectedAdmins, plan.Students, loginIDs})
	if err != nil {
		return StudentPlan{}, err
	}
	digest := sha256.Sum256(fingerprintInput)
	plan.Fingerprint = hex.EncodeToString(digest[:])
	return plan, nil
}

type preparedStudentLogin struct{ password, hash, email string }

func prepareStudentSetupLogins(count int) ([]preparedStudentLogin, error) {
	prepared := make([]preparedStudentLogin, count)
	seen := map[string]bool{}
	for i := range prepared {
		for {
			password, err := auth.TemporaryPassword()
			if err != nil {
				return nil, err
			}
			if seen[password] {
				continue
			}
			seen[password] = true
			email, err := auth.PlaceholderEmail()
			if err != nil {
				return nil, err
			}
			hash, err := auth.HashTemporaryPassword(password)
			if err != nil {
				return nil, err
			}
			prepared[i] = preparedStudentLogin{password, hash, email}
			break
		}
	}
	return prepared, nil
}

// ApplyStudents performs one confirmed apply. Table locks serialize all
// relevant writes while the manifest, counts, rolls and login IDs are
// rechecked. A retry returns only the count, never the old credentials.
func ApplyStudents(ctx context.Context, db *sql.DB, actor string, in StudentApplyRequest) (StudentApplyResult, error) {
	if !in.Confirm || !studentSetupUUID.MatchString(in.SessionID) || !studentSetupUUID.MatchString(in.IdempotencyKey) || !studentSetupHash.MatchString(in.Fingerprint) {
		return StudentApplyResult{}, ErrStudentSetupInvalid
	}
	if err := requireStudentSetupAdmin(ctx, db, actor); err != nil {
		return StudentApplyResult{}, err
	}
	lookup := func() (StudentApplyResult, bool, error) {
		var fingerprint, session string
		var count int
		err := db.QueryRowContext(ctx, `SELECT fingerprint,session_id::text,admitted FROM student_setup_runs WHERE idempotency_key=$1`, in.IdempotencyKey).Scan(&fingerprint, &session, &count)
		if errors.Is(err, sql.ErrNoRows) {
			return StudentApplyResult{}, false, nil
		}
		if err != nil {
			return StudentApplyResult{}, false, err
		}
		if fingerprint != in.Fingerprint || session != in.SessionID {
			return StudentApplyResult{}, true, ErrStudentSetupKeyReused
		}
		return StudentApplyResult{Admitted: count, Replayed: true, Logins: []people.StudentLogin{}}, true, nil
	}
	if previous, found, err := lookup(); found || err != nil {
		return previous, err
	}
	before, err := PreviewStudents(ctx, db, actor, in.SessionID)
	if err != nil || before.Fingerprint != in.Fingerprint {
		if err != nil {
			return StudentApplyResult{}, err
		}
		return StudentApplyResult{}, ErrStudentSetupStale
	}
	if before.ToAdd == 0 {
		return StudentApplyResult{}, ErrStudentSetupInvalid
	}
	prepared, err := prepareStudentSetupLogins(before.ToAdd)
	if err != nil {
		return StudentApplyResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return StudentApplyResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return StudentApplyResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `LOCK TABLE users, students, student_enrolments, academic_sessions, academic_classes, academic_sections, employees, student_setup_runs IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return StudentApplyResult{}, err
	}
	if err := requireStudentSetupAdmin(ctx, tx, actor); err != nil {
		return StudentApplyResult{}, err
	}
	var priorFingerprint, priorSession string
	var priorCount int
	err = tx.QueryRowContext(ctx, `SELECT fingerprint,session_id::text,admitted FROM student_setup_runs WHERE idempotency_key=$1`, in.IdempotencyKey).Scan(&priorFingerprint, &priorSession, &priorCount)
	if err == nil {
		if priorFingerprint != in.Fingerprint || priorSession != in.SessionID {
			return StudentApplyResult{}, ErrStudentSetupKeyReused
		}
		return StudentApplyResult{Admitted: priorCount, Replayed: true, Logins: []people.StudentLogin{}}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return StudentApplyResult{}, err
	}
	current, err := studentPlanTx(ctx, tx, in.SessionID)
	if err != nil || current.Fingerprint != in.Fingerprint || current.ToAdd != len(prepared) {
		return StudentApplyResult{}, ErrStudentSetupStale
	}
	out := StudentApplyResult{Admitted: current.ToAdd, Logins: make([]people.StudentLogin, 0, current.ToAdd)}
	for i, candidate := range current.Students {
		one := prepared[i]
		var studentID, userID string
		if err := tx.QueryRowContext(ctx, `INSERT INTO students (admission_no,name_en,name_bn,gender,status,session_id,class_id,section_id,roll_no)
			VALUES ($1,$2,$3,$4,'active',$5,$6,$7,$8) RETURNING id`, candidate.AdmissionNo, candidate.NameEN,
			candidate.NameBN, candidate.Gender, candidate.SessionID, candidate.ClassID, candidate.SectionID, candidate.RollNo).Scan(&studentID); err != nil {
			return StudentApplyResult{}, err
		}
		expires := time.Now().UTC().Add(auth.TemporaryPasswordLifetime)
		if err := tx.QueryRowContext(ctx, `INSERT INTO users (email,display_name,role,is_active,password_hash,must_change_password,temporary_password_expires_at,login_id)
			VALUES ($1,$2,'student',true,$3,true,$4,$5) RETURNING id`, one.email, candidate.NameEN,
			one.hash, expires, candidate.LoginID).Scan(&userID); err != nil {
			return StudentApplyResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE students SET user_id=$1 WHERE id=$2`, userID, studentID); err != nil {
			return StudentApplyResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (actor_user_id,action,target_type,target_id,outcome,metadata)
			VALUES ($1,'student.admitted','student',$2,'success',jsonb_build_object('source','synthetic_setup','login_created',true,'test_marker','TEST'))`, actor, studentID); err != nil {
			return StudentApplyResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (actor_user_id,action,target_type,target_id,outcome,metadata)
			VALUES ($1,'user.created','user',$2,'success',jsonb_build_object('role','student','source','synthetic_setup','student_id',$3::text))`, actor, userID, studentID); err != nil {
			return StudentApplyResult{}, err
		}
		roll := candidate.RollNo
		out.Logins = append(out.Logins, people.StudentLogin{StudentID: studentID, StudentName: candidate.NameEN, NameBN: candidate.NameBN,
			AdmissionNo: candidate.AdmissionNo, ClassName: candidate.Class, SectionName: candidate.Section,
			RollNo: &roll, UserID: userID, LoginID: candidate.LoginID, InitialPassword: one.password, PasswordExpiresAt: expires})
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO student_setup_runs (idempotency_key,fingerprint,session_id,admitted,created_by) VALUES ($1,$2,$3,$4,$5)`,
		in.IdempotencyKey, in.Fingerprint, in.SessionID, out.Admitted, actor); err != nil {
		return StudentApplyResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (actor_user_id,action,target_type,target_id,outcome,metadata)
		VALUES ($1,'students.synthetic_setup','session',$2,'success',jsonb_build_object('admitted',$3::integer,'fingerprint',$4::text))`, actor, in.SessionID, out.Admitted, in.Fingerprint); err != nil {
		return StudentApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return StudentApplyResult{}, err
	}
	return out, nil
}
