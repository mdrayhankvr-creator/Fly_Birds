package staff

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/users"
	"gorm.io/gorm"
)

type CreateWithLoginInput struct {
	EmployeeCode        string           `json:"employee_code"`
	ConfirmEmployeeCode string           `json:"confirm_employee_code"`
	Role                string           `json:"role"`
	Email               string           `json:"email"`
	NameEN              string           `json:"name_en"`
	NameBN              string           `json:"name_bn"`
	Designation         string           `json:"designation"`
	JoinedOn            string           `json:"joined_on"`
	Qualifications      []string         `json:"qualifications"`
	Assignment          *AssignmentInput `json:"assignment,omitempty"`
}

// The temporary password exists only in this creation response. It is never
// persisted as plaintext and cannot be fetched later.
type CreatedWithLogin struct {
	Employee        Employee  `json:"employee"`
	Email           string    `json:"email"`
	Role            string    `json:"role"`
	InitialPassword string    `json:"initial_password"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type createWithLoginRepository interface {
	CreateWithLogin(context.Context, string, CreateWithLoginInput, []byte, string) (CreatedWithLogin, error)
}

func (s *Service) CreateWithLogin(ctx context.Context, actor string, in CreateWithLoginInput, photoType string, photo []byte) (CreatedWithLogin, error) {
	in.EmployeeCode = strings.ToUpper(strings.TrimSpace(in.EmployeeCode))
	in.ConfirmEmployeeCode = strings.ToUpper(strings.TrimSpace(in.ConfirmEmployeeCode))
	in.NameEN = strings.TrimSpace(in.NameEN)
	in.NameBN = strings.TrimSpace(in.NameBN)
	in.Designation = strings.TrimSpace(in.Designation)
	mail, err := auth.NormalizeEmail(in.Email)
	if err != nil {
		return CreatedWithLogin{}, ErrInvalid
	}
	in.Email = mail
	// These owner accounts must never be provisioned through a staff workflow,
	// even in a database where an account with the address is absent.
	if in.Email == "mdrayhankvr@gmail.com" || in.Email == "2023331031@student.sust.edu" {
		return CreatedWithLogin{}, ErrConflict
	}
	if !validID(actor) || !ValidEmployeeCode(in.EmployeeCode) || in.ConfirmEmployeeCode != in.EmployeeCode ||
		(in.Role != "teacher" && in.Role != "head_teacher") || !validText(in.NameEN, 160, true) ||
		!validText(in.NameBN, 160, false) || !validText(in.Designation, 120, true) || !validDate(in.JoinedOn) {
		return CreatedWithLogin{}, ErrInvalid
	}
	if in.Assignment != nil && (!validID(in.Assignment.SessionID) || !validID(in.Assignment.ClassID) || !validID(in.Assignment.SubjectID)) {
		return CreatedWithLogin{}, ErrInvalid
	}
	in.Qualifications, err = cleanQualifications(in.Qualifications)
	if err != nil {
		return CreatedWithLogin{}, err
	}
	var content []byte
	var version string
	if len(photo) > 0 || photoType != "" {
		content, version, err = ProcessPhoto(photoType, photo)
		if err != nil {
			return CreatedWithLogin{}, err
		}
	}
	repo, ok := s.repo.(createWithLoginRepository)
	if !ok {
		return CreatedWithLogin{}, ErrNotFound
	}
	return repo.CreateWithLogin(ctx, actor, in, content, version)
}

func (r *GormRepository) CreateWithLogin(ctx context.Context, actor string, in CreateWithLoginInput, photo []byte, version string) (CreatedWithLogin, error) {
	result := CreatedWithLogin{Email: in.Email, Role: in.Role}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, permission := range []string{"staff:manage", "users:create", "permissions:manage"} {
			if err := requireAdministrator(tx, actor, permission); err != nil {
				return err
			}
		}
		password, err := auth.TemporaryPassword()
		if err != nil {
			return err
		}
		hash, err := auth.HashTemporaryPassword(password)
		if err != nil {
			return err
		}
		account, expires, err := users.CreateInTx(tx, actor, users.User{Email: in.Email, DisplayName: in.NameEN, Role: in.Role, IsActive: true}, hash)
		if errors.Is(err, users.ErrDuplicate) {
			return ErrDuplicate
		}
		if errors.Is(err, users.ErrForbidden) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		qualifications, err := json.Marshal(in.Qualifications)
		if err != nil {
			return err
		}
		employee := Employee{EmployeeCode: in.EmployeeCode, UserID: &account.ID, Kind: "teacher", NameEN: in.NameEN, NameBN: in.NameBN, Designation: in.Designation, JoinedOn: in.JoinedOn, IsActive: true, PhotoVersion: version}
		if err := tx.Raw(`INSERT INTO employees (employee_code,user_id,kind,name_en,name_bn,designation,joined_on,email,qualifications)
			VALUES (?,?,'teacher',?,?,?,?,?,?::jsonb) RETURNING id`, in.EmployeeCode, account.ID, in.NameEN, in.NameBN, in.Designation, in.JoinedOn, in.Email, string(qualifications)).Scan(&employee.ID).Error; err != nil {
			return err
		}
		if len(photo) > 0 {
			if err := tx.Exec("INSERT INTO employee_photos (employee_id,content,version,updated_by) VALUES (?,?,?,?)", employee.ID, photo, version, actor).Error; err != nil {
				return err
			}
		}
		if in.Assignment != nil {
			var count int64
			a := in.Assignment
			if err := tx.Table("academic_sessions AS s").Joins("JOIN academic_classes AS c ON c.id=?", a.ClassID).Joins("JOIN academic_subjects AS u ON u.id=?", a.SubjectID).Joins("JOIN academic_class_subjects AS m ON m.class_id=c.id AND m.subject_id=u.id").Where("s.id=? AND c.is_active=true AND u.is_active=true", a.SessionID).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrConflict
			}
			if err := tx.Exec("INSERT INTO teacher_assignments (employee_id,session_id,class_id,subject_id) VALUES (?,?,?,?)", employee.ID, a.SessionID, a.ClassID, a.SubjectID).Error; err != nil {
				return err
			}
		}
		if err := audit.Record(tx, &actor, "staff.created_with_login", "employee", employee.ID, map[string]any{"role": in.Role, "has_photo": len(photo) > 0, "has_assignment": in.Assignment != nil}); err != nil {
			return err
		}
		result.Employee, result.InitialPassword, result.ExpiresAt = employee, password, expires
		return nil
	})
	if err != nil {
		return CreatedWithLogin{}, dbError(err)
	}
	return result, nil
}
