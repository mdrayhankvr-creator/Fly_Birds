package schoolsetup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
)

var (
	ErrTeacherSetupForbidden = errors.New("teacher setup requires an administrator with staff and account grants")
	ErrTeacherSetupInvalid   = errors.New("invalid teacher setup request")
	ErrTeacherSetupConflict  = errors.New("teacher setup identifiers or school state conflict")
	ErrTeacherSetupStale     = errors.New("teacher setup preview is stale")
	ErrTeacherSetupKeyReused = errors.New("teacher setup key was used for another preview")
)

type ExistingTeacher struct {
	ID                  string `json:"id"`
	EmployeeCode        string `json:"employee_code"`
	NameEN              string `json:"name_en"`
	Active              bool   `json:"active"`
	LinkedAccount       bool   `json:"linked_account"`
	LinkedRole          string `json:"linked_role"`
	LinkedAccountActive bool   `json:"linked_account_active"`
}

type PlannedTeacher struct {
	EmployeeCode string `json:"employee_code"`
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	Role         string `json:"role"`
	LoginID      string `json:"login_id"`
	TestMarker   string `json:"test_marker"`
}

type TeacherPlan struct {
	Target      int               `json:"target"`
	Existing    int               `json:"existing"`
	ToAdd       int               `json:"to_add"`
	Teachers    []ExistingTeacher `json:"teachers"`
	Candidates  []PlannedTeacher  `json:"candidates"`
	Fingerprint string            `json:"fingerprint"`
	Incomplete  []string          `json:"incomplete"`
}

type TeacherApplyRequest struct {
	Fingerprint    string `json:"fingerprint"`
	IdempotencyKey string `json:"idempotency_key"`
	Confirm        bool   `json:"confirm"`
}

type TeacherCredential struct {
	NameEN            string    `json:"name_en"`
	NameBN            string    `json:"name_bn"`
	EmployeeCode      string    `json:"employee_code"`
	Role              string    `json:"role"`
	LoginID           string    `json:"login_id"`
	InitialPassword   string    `json:"initial_password"`
	PasswordExpiresAt time.Time `json:"password_expires_at"`
}

type TeacherApplyResult struct {
	Created     int                 `json:"created"`
	Replayed    bool                `json:"replayed"`
	Credentials []TeacherCredential `json:"credentials"`
}

func requireTeacherSetupAdmin(ctx context.Context, q studentQueryer, actor string) error {
	if !studentSetupUUID.MatchString(actor) {
		return ErrTeacherSetupForbidden
	}
	var grants, modules int
	if err := q.QueryRowContext(ctx, `SELECT count(DISTINCT rp.permission_id) FROM users u
JOIN role_permissions rp ON rp.role_id=u.role WHERE u.id=$1 AND u.is_active AND u.role='admin'
AND rp.permission_id IN ('staff:manage','users:create','permissions:manage')`, actor).Scan(&grants); err != nil {
		return err
	}
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM school_modules WHERE school_id=1 AND enabled
AND module_key IN ('staff','permissions')`).Scan(&modules); err != nil {
		return err
	}
	if grants != 3 || modules != 2 {
		return ErrTeacherSetupForbidden
	}
	return nil
}

// PreviewTeachers reads a consistent PostgreSQL snapshot and never writes.
func PreviewTeachers(ctx context.Context, db *sql.DB, actor string) (TeacherPlan, error) {
	tx, err := beginReadOnly(ctx, db)
	if err != nil {
		return TeacherPlan{}, err
	}
	defer tx.Rollback()
	if err := requireTeacherSetupAdmin(ctx, tx, actor); err != nil {
		return TeacherPlan{}, err
	}
	plan, err := teacherPlanTx(ctx, tx)
	if err != nil {
		return TeacherPlan{}, err
	}
	return plan, tx.Commit()
}

type teacherState struct {
	ID, Code, Name, UserID, Role, Updated string
	Active, LinkedActive                  bool
}
type protectedState struct {
	ID, Email, Role, Updated string
	Active                   bool
}

func teacherPlanTx(ctx context.Context, tx *sql.Tx) (TeacherPlan, error) {
	plan := TeacherPlan{Target: TeacherTarget, Teachers: []ExistingTeacher{}, Candidates: []PlannedTeacher{},
		Incomplete: []string{"verified staff identities and joining dates", "qualifications and approved subjects", "section assignments, rooms and availability"}}
	rows, err := tx.QueryContext(ctx, `SELECT e.id::text,e.employee_code,e.name_en,e.is_active,
COALESCE(e.user_id::text,''),COALESCE(u.role,''),COALESCE(u.is_active,false),e.updated_at::text
FROM employees e LEFT JOIN users u ON u.id=e.user_id WHERE e.kind='teacher' ORDER BY e.employee_code,e.id`)
	if err != nil {
		return TeacherPlan{}, err
	}
	var states []teacherState
	for rows.Next() {
		var s teacherState
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Active, &s.UserID, &s.Role, &s.LinkedActive, &s.Updated); err != nil {
			rows.Close()
			return TeacherPlan{}, err
		}
		states = append(states, s)
		plan.Teachers = append(plan.Teachers, ExistingTeacher{s.ID, s.Code, s.Name, s.Active, s.UserID != "", s.Role, s.LinkedActive})
		if s.Active {
			plan.Existing++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TeacherPlan{}, err
	}
	rows.Close()
	if plan.Existing > TeacherTarget {
		return TeacherPlan{}, ErrTeacherSetupConflict
	}
	plan.ToAdd = TeacherTarget - plan.Existing
	codes, err := identifierKeys(ctx, tx, "SELECT upper(employee_code) FROM employees ORDER BY upper(employee_code)")
	if err != nil {
		return TeacherPlan{}, err
	}
	emails, err := identifierKeys(ctx, tx, "SELECT lower(email) FROM users ORDER BY lower(email)")
	if err != nil {
		return TeacherPlan{}, err
	}
	usedEmails := make(map[string]bool, len(emails))
	for _, email := range emails {
		if usedEmails[email] {
			return TeacherPlan{}, ErrTeacherSetupConflict
		}
		usedEmails[email] = true
	}
	protectedRows, err := tx.QueryContext(ctx, `SELECT id::text,email,role,is_active,updated_at::text FROM users
WHERE lower(email) IN ('mdrayhankvr@gmail.com','2023331031@student.sust.edu') ORDER BY lower(email),id`)
	if err != nil {
		return TeacherPlan{}, err
	}
	var protected []protectedState
	for protectedRows.Next() {
		var p protectedState
		if err := protectedRows.Scan(&p.ID, &p.Email, &p.Role, &p.Active, &p.Updated); err != nil {
			protectedRows.Close()
			return TeacherPlan{}, err
		}
		protected = append(protected, p)
	}
	if err := protectedRows.Err(); err != nil {
		protectedRows.Close()
		return TeacherPlan{}, err
	}
	protectedRows.Close()
	if len(protected) != len(protectedAdminEmails) {
		return TeacherPlan{}, ErrTeacherSetupConflict
	}
	for _, p := range protected {
		if p.Role != "admin" || !p.Active {
			return TeacherPlan{}, ErrTeacherSetupConflict
		}
	}
	old := make([]Teacher, 0, len(states))
	for _, s := range states {
		old = append(old, Teacher{EmployeeCode: s.Code})
	}
	ids, err := teacherIdentifiers(plan.Existing, old, codes)
	if err != nil {
		return TeacherPlan{}, ErrTeacherSetupConflict
	}
	for _, id := range ids {
		slot, err := strconv.Atoi(strings.TrimPrefix(id.EmployeeCode, "TEACHER-"))
		if err != nil || slot < 1 || slot > len(syntheticTeacherNames) || !staff.ValidEmployeeCode(id.EmployeeCode) {
			return TeacherPlan{}, ErrTeacherSetupConflict
		}
		nameEN, nameBN := syntheticTeacherName(slot)
		login := "test." + strings.ToLower(id.EmployeeCode) + "@flybirds.test"
		if _, err := auth.NormalizeEmail(login); err != nil || usedEmails[login] {
			return TeacherPlan{}, ErrTeacherSetupConflict
		}
		for _, admin := range protectedAdminEmails {
			if strings.EqualFold(login, admin) {
				return TeacherPlan{}, ErrTeacherSetupConflict
			}
		}
		usedEmails[login] = true
		plan.Candidates = append(plan.Candidates, PlannedTeacher{id.EmployeeCode, nameEN, nameBN, "teacher", login, "TEST"})
	}
	if len(plan.Candidates) != plan.ToAdd {
		return TeacherPlan{}, ErrTeacherSetupConflict
	}
	sort.Strings(codes)
	sort.Strings(emails)
	data, err := json.Marshal(struct {
		Teachers      []teacherState
		Codes, Emails []string
		Protected     []protectedState
		Candidates    []PlannedTeacher
	}{states, codes, emails, protected, plan.Candidates})
	if err != nil {
		return TeacherPlan{}, err
	}
	digest := sha256.Sum256(data)
	plan.Fingerprint = hex.EncodeToString(digest[:])
	return plan, nil
}

type preparedTeacherPassword struct{ plain, hash string }

func prepareTeacherPasswords(count int) ([]preparedTeacherPassword, error) {
	out := make([]preparedTeacherPassword, count)
	seen := map[string]bool{}
	for i := range out {
		for {
			password, err := auth.TemporaryPassword()
			if err != nil {
				return nil, err
			}
			if seen[password] {
				continue
			}
			seen[password] = true
			hash, err := auth.HashTemporaryPassword(password)
			if err != nil {
				return nil, err
			}
			out[i] = preparedTeacherPassword{password, hash}
			break
		}
	}
	return out, nil
}

func teacherSetupDBError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23514" || pg.Code == "23502") {
		return ErrTeacherSetupConflict
	}
	return err
}

// ApplyTeachers rechecks the fingerprint after locking accounts and staff.
// One transaction creates every profile and account, or none. Retries never
// replay initial passwords, which are absent from persistent state.
func ApplyTeachers(ctx context.Context, db *sql.DB, actor string, in TeacherApplyRequest) (TeacherApplyResult, error) {
	if !in.Confirm || !studentSetupUUID.MatchString(in.IdempotencyKey) || !studentSetupHash.MatchString(in.Fingerprint) {
		return TeacherApplyResult{}, ErrTeacherSetupInvalid
	}
	if err := requireTeacherSetupAdmin(ctx, db, actor); err != nil {
		return TeacherApplyResult{}, err
	}
	lookup := func(q studentQueryer) (TeacherApplyResult, bool, error) {
		var fingerprint string
		var count int
		err := q.QueryRowContext(ctx, `SELECT fingerprint,created FROM synthetic_teacher_setup_runs WHERE idempotency_key=$1`, in.IdempotencyKey).Scan(&fingerprint, &count)
		if errors.Is(err, sql.ErrNoRows) {
			return TeacherApplyResult{}, false, nil
		}
		if err != nil {
			return TeacherApplyResult{}, false, err
		}
		if fingerprint != in.Fingerprint {
			return TeacherApplyResult{}, true, ErrTeacherSetupKeyReused
		}
		return TeacherApplyResult{Created: count, Replayed: true, Credentials: []TeacherCredential{}}, true, nil
	}
	if previous, found, err := lookup(db); found || err != nil {
		return previous, err
	}
	before, err := PreviewTeachers(ctx, db, actor)
	if err != nil {
		return TeacherApplyResult{}, err
	}
	if before.Fingerprint != in.Fingerprint {
		return TeacherApplyResult{}, ErrTeacherSetupStale
	}
	if before.ToAdd == 0 {
		return TeacherApplyResult{}, ErrTeacherSetupInvalid
	}
	prepared, err := prepareTeacherPasswords(before.ToAdd)
	if err != nil {
		return TeacherApplyResult{}, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return TeacherApplyResult{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return TeacherApplyResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `LOCK TABLE employees, users, role_permissions, school_modules, synthetic_teacher_setup_runs IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return TeacherApplyResult{}, err
	}
	if err = requireTeacherSetupAdmin(ctx, tx, actor); err != nil {
		return TeacherApplyResult{}, err
	}
	if previous, found, err := lookup(tx); found || err != nil {
		return previous, err
	}
	current, err := teacherPlanTx(ctx, tx)
	if err != nil {
		return TeacherApplyResult{}, err
	}
	if current.Fingerprint != in.Fingerprint || current.ToAdd != len(prepared) {
		return TeacherApplyResult{}, ErrTeacherSetupStale
	}
	out := TeacherApplyResult{Created: current.ToAdd, Credentials: make([]TeacherCredential, 0, current.ToAdd)}
	for i, candidate := range current.Candidates {
		one := prepared[i]
		expires := time.Now().UTC().Add(auth.TemporaryPasswordLifetime)
		var userID, employeeID string
		if err := tx.QueryRowContext(ctx, `INSERT INTO users (email,display_name,role,is_active,password_hash,must_change_password,temporary_password_expires_at)
VALUES ($1,$2,'teacher',true,$3,true,$4) RETURNING id::text`, candidate.LoginID, candidate.NameEN, one.hash, expires).Scan(&userID); err != nil {
			return TeacherApplyResult{}, teacherSetupDBError(err)
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,joined_on,email,qualifications,synthetic_test)
VALUES ($1,$2,'teacher',$3,$4,'TEST Teacher',NULL,'','[]'::jsonb,true) RETURNING id::text`, candidate.EmployeeCode, userID, candidate.NameEN, candidate.NameBN).Scan(&employeeID); err != nil {
			return TeacherApplyResult{}, teacherSetupDBError(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (actor_user_id,action,target_type,target_id,outcome,metadata)
VALUES ($1,'user.created','user',$2,'success',jsonb_build_object('role','teacher','source','synthetic_teacher_setup','test_marker','TEST'))`, actor, userID); err != nil {
			return TeacherApplyResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (actor_user_id,action,target_type,target_id,outcome,metadata)
VALUES ($1,'staff.synthetic_teacher_created','employee',$2,'success',jsonb_build_object('employee_code',$3::text,'test_marker','TEST','account_linked',true))`, actor, employeeID, candidate.EmployeeCode); err != nil {
			return TeacherApplyResult{}, err
		}
		out.Credentials = append(out.Credentials, TeacherCredential{candidate.NameEN, candidate.NameBN, candidate.EmployeeCode, "teacher", candidate.LoginID, one.plain, expires})
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO synthetic_teacher_setup_runs (idempotency_key,fingerprint,created,created_by) VALUES ($1,$2,$3,$4)`, in.IdempotencyKey, in.Fingerprint, out.Created, actor); err != nil {
		return TeacherApplyResult{}, teacherSetupDBError(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events (actor_user_id,action,target_type,target_id,outcome,metadata)
VALUES ($1,'staff.synthetic_teacher_setup','teacher_setup',$2,'success',jsonb_build_object('created',$3::integer,'fingerprint',$4::text))`, actor, in.IdempotencyKey, out.Created, in.Fingerprint); err != nil {
		return TeacherApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TeacherApplyResult{}, teacherSetupDBError(err)
	}
	return out, nil
}
