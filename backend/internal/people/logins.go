package people

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"gorm.io/gorm"
)

// Student logins (checkpoint 5). A student login signs in with a sign-in ID:
// the student's Student ID when the school uses one and nobody else has it,
// otherwise the admission number. An email is optional. The temporary
// password is shown once, on the response that created it, and is never
// stored in plain text, logged or audited.

var (
	ErrHasLogin         = errors.New("student already has a login")
	ErrNoLoginID        = errors.New("no usable sign-in ID for this student")
	ErrNoStudentsToLink = errors.New("every student in this section already has a login")
)

// StudentLogin is one created login, as printed on a slip.
type StudentLogin struct {
	StudentID   string `json:"student_id"`
	StudentName string `json:"student_name"`
	NameBN      string `json:"name_bn"`
	AdmissionNo string `json:"admission_no"`
	ClassName   string `json:"class_name"`
	SectionName string `json:"section_name"`
	RollNo      *int   `json:"roll_no"`
	UserID      string `json:"user_id"`
	LoginID     string `json:"login_id"`
	// Email is empty when the login has only a placeholder address.
	Email string `json:"email,omitempty"`
	// InitialPassword is returned once, in this response only.
	InitialPassword string `json:"initial_password"`
}

// newLogin is a login to create: the password is already hashed.
type newLogin struct {
	StudentID string
	Email     string
	Hash      string
	Password  string
}

// studentLoginIDTx chooses the sign-in ID for a student by the rule above.
// It returns "" when neither value can be used (for example an admission
// number with a space in it that also clashes with another login).
func studentLoginIDTx(tx *gorm.DB, studentID string) (string, error) {
	var chosen string
	err := tx.Raw(`SELECT CASE
  WHEN s.student_code ~ '^[^[:space:]@]{1,64}$'
   AND NOT EXISTS (SELECT 1 FROM students x WHERE x.id <> s.id AND (lower(x.student_code) = lower(s.student_code) OR lower(x.admission_no) = lower(s.student_code)))
   AND NOT EXISTS (SELECT 1 FROM users u WHERE lower(u.login_id) = lower(s.student_code)) THEN s.student_code
  WHEN s.admission_no ~ '^[^[:space:]@]{1,64}$'
   AND NOT EXISTS (SELECT 1 FROM users u WHERE lower(u.login_id) = lower(s.admission_no)) THEN s.admission_no
  ELSE '' END
FROM students s WHERE s.id = ?`, studentID).Scan(&chosen).Error
	return chosen, err
}

// createStudentLoginTx creates a login for a student who has none, links it
// and records it in the audit log (without the password). source names where
// it was made: "admission", "student_profile", "section" or "import".
func createStudentLoginTx(tx *gorm.DB, actorID string, in newLogin, source string) (StudentLogin, error) {
	var row struct {
		NameEN, NameBN, AdmissionNo, ClassName, SectionName, UserID string
		RollNo                                                      *int
	}
	err := tx.Raw(`SELECT s.name_en, s.name_bn, s.admission_no, c.name AS class_name, sec.name AS section_name, COALESCE(s.user_id::text,'') AS user_id, s.roll_no
FROM students s JOIN academic_classes c ON c.id = s.class_id JOIN academic_sections sec ON sec.id = s.section_id WHERE s.id = ? FOR UPDATE OF s`, in.StudentID).Scan(&row).Error
	if err != nil {
		return StudentLogin{}, err
	}
	if row.AdmissionNo == "" {
		return StudentLogin{}, ErrNotFound
	}
	if row.UserID != "" {
		return StudentLogin{}, ErrHasLogin
	}
	loginID, err := studentLoginIDTx(tx, in.StudentID)
	if err != nil {
		return StudentLogin{}, err
	}
	if loginID == "" {
		return StudentLogin{}, ErrNoLoginID
	}
	email := in.Email
	if email == "" {
		if email, err = auth.PlaceholderEmail(); err != nil {
			return StudentLogin{}, err
		}
	}
	var userID string
	// The student sets their own password at the first sign-in.
	if err := tx.Raw("INSERT INTO users (email,display_name,role,is_active,password_hash,must_change_password,login_id) VALUES (?,?,'student',true,?,true,?) RETURNING id", email, row.NameEN, in.Hash, loginID).Scan(&userID).Error; err != nil {
		return StudentLogin{}, loginError(err)
	}
	if err := tx.Exec("UPDATE students SET user_id = ?, updated_at = now() WHERE id = ?", userID, in.StudentID).Error; err != nil {
		return StudentLogin{}, err
	}
	if err := audit.Record(tx, &actorID, "user.created", "user", userID, map[string]any{"role": "student", "source": source, "student_id": in.StudentID, "has_email": in.Email != ""}); err != nil {
		return StudentLogin{}, err
	}
	out := StudentLogin{StudentID: in.StudentID, StudentName: row.NameEN, NameBN: row.NameBN, AdmissionNo: row.AdmissionNo, ClassName: row.ClassName, SectionName: row.SectionName, RollNo: row.RollNo, UserID: userID, LoginID: loginID, InitialPassword: in.Password}
	if in.Email != "" {
		out.Email = in.Email
	}
	return out, nil
}

func loginError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		switch pg.ConstraintName {
		case "users_email_key":
			return ErrLoginTaken
		case "users_login_id_key":
			return ErrNoLoginID
		}
	}
	return err
}

// prepareLogin makes a temporary password for a login and hashes it.
func prepareLogin(studentID, rawEmail string) (newLogin, error) {
	in := newLogin{StudentID: studentID}
	if strings.TrimSpace(rawEmail) != "" {
		email, err := auth.NormalizeEmail(rawEmail)
		if err != nil || auth.IsPlaceholderEmail(email) {
			return newLogin{}, ErrInvalid
		}
		in.Email = email
	}
	var err error
	if in.Password, err = auth.TemporaryPassword(); err != nil {
		return newLogin{}, err
	}
	if in.Hash, err = auth.HashTemporaryPassword(in.Password); err != nil {
		return newLogin{}, err
	}
	return in, nil
}

// CreateStudentLogin creates and links a login for one student, from the
// student's profile, and returns its slip.
func (s *Service) CreateStudentLogin(ctx context.Context, a Actor, studentID, email string) (StudentLogin, error) {
	if !validID(a.ID) || !validID(studentID) {
		return StudentLogin{}, ErrInvalid
	}
	in, err := prepareLogin(studentID, email)
	if err != nil {
		return StudentLogin{}, err
	}
	repo, ok := s.repo.(loginRepository)
	if !ok {
		return StudentLogin{}, ErrNotFound
	}
	return repo.CreateStudentLogin(ctx, a, in)
}

// CreateSectionLogins gives a login to every active student in a section who
// has none, in one transaction, and returns all the slips.
func (s *Service) CreateSectionLogins(ctx context.Context, a Actor, sectionID string) ([]StudentLogin, error) {
	if !validID(a.ID) || !validID(sectionID) {
		return nil, ErrInvalid
	}
	repo, ok := s.repo.(loginRepository)
	if !ok {
		return nil, ErrNotFound
	}
	ids, err := repo.StudentsWithoutLogin(ctx, a, sectionID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, ErrNoStudentsToLink
	}
	logins := make([]newLogin, 0, len(ids))
	for _, id := range ids {
		in, err := prepareLogin(id, "")
		if err != nil {
			return nil, err
		}
		logins = append(logins, in)
	}
	return repo.CreateSectionLogins(ctx, a, sectionID, logins)
}

type loginRepository interface {
	CreateStudentLogin(ctx context.Context, a Actor, in newLogin) (StudentLogin, error)
	StudentsWithoutLogin(ctx context.Context, a Actor, sectionID string) ([]string, error)
	CreateSectionLogins(ctx context.Context, a Actor, sectionID string, in []newLogin) ([]StudentLogin, error)
}

func requireLoginWriter(tx *gorm.DB, a Actor) error {
	if err := requireAdmin(tx, a, "students:manage"); err != nil {
		return err
	}
	return requireAdmin(tx, a, "users:create")
}

func (r *GormRepository) CreateStudentLogin(ctx context.Context, a Actor, in newLogin) (StudentLogin, error) {
	var out StudentLogin
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireLoginWriter(tx, a); err != nil {
			return err
		}
		if err := setActor(tx, a); err != nil {
			return err
		}
		var err error
		out, err = createStudentLoginTx(tx, a.ID, in, "student_profile")
		return err
	})
	return out, err
}

func (r *GormRepository) StudentsWithoutLogin(ctx context.Context, a Actor, sectionID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireLoginWriter(tx, a); err != nil {
			return err
		}
		var sections int64
		if err := tx.Table("academic_sections").Where("id = ?", sectionID).Count(&sections).Error; err != nil {
			return err
		}
		if sections == 0 {
			return ErrNotFound
		}
		return tx.Table("students").Where("section_id = ? AND user_id IS NULL AND status = 'active'", sectionID).Order("roll_no NULLS LAST, name_en, id").Pluck("id", &ids).Error
	})
	return ids, err
}

func (r *GormRepository) CreateSectionLogins(ctx context.Context, a Actor, sectionID string, in []newLogin) ([]StudentLogin, error) {
	out := make([]StudentLogin, 0, len(in))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireLoginWriter(tx, a); err != nil {
			return err
		}
		if err := setActor(tx, a); err != nil {
			return err
		}
		for _, one := range in {
			login, err := createStudentLoginTx(tx, a.ID, one, "section")
			if errors.Is(err, ErrHasLogin) {
				continue // linked meanwhile; nothing to do for this student
			}
			if err != nil {
				return err
			}
			out = append(out, login)
		}
		return audit.Record(tx, &a.ID, "students.logins_created", "section", sectionID, map[string]any{"count": len(out)})
	})
	return out, err
}
