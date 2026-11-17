package staff

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AccountLinkInput confirms both records by stable identifiers. Display names
// are deliberately not used to decide whether an account belongs to staff.
type AccountLinkInput struct {
	UserID              string `json:"user_id"`
	ConfirmEmail        string `json:"confirm_email"`
	ConfirmEmployeeCode string `json:"confirm_employee_code"`
}

type accountLinkRepository interface {
	LinkAccount(context.Context, string, string, AccountLinkInput) (Employee, error)
}

func (s *Service) LinkAccount(ctx context.Context, actor, employeeID string, input AccountLinkInput) (Employee, error) {
	input.ConfirmEmail = strings.ToLower(strings.TrimSpace(input.ConfirmEmail))
	input.ConfirmEmployeeCode = strings.ToUpper(strings.TrimSpace(input.ConfirmEmployeeCode))
	if !validID(actor) || !validID(employeeID) || !validID(input.UserID) || input.ConfirmEmail == "" || len(input.ConfirmEmail) > 254 || !code.MatchString(input.ConfirmEmployeeCode) {
		return Employee{}, ErrInvalid
	}
	repo, ok := s.repo.(accountLinkRepository)
	if !ok {
		return Employee{}, ErrNotFound
	}
	return repo.LinkAccount(ctx, actor, employeeID, input)
}

func (r *GormRepository) LinkAccount(ctx context.Context, actor, employeeID string, input AccountLinkInput) (Employee, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireAdministrator(tx, actor, "staff:manage"); err != nil {
			return err
		}
		if err := requireAdministrator(tx, actor, "users:read"); err != nil {
			return err
		}
		var employee struct {
			ID           string
			EmployeeCode string
			UserID       *string
			Kind         string
			IsActive     bool
		}
		err := tx.Table("employees").Select("id,employee_code,user_id,kind,is_active").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", employeeID).Take(&employee).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if employee.Kind != "teacher" || !employee.IsActive || employee.UserID != nil || employee.EmployeeCode != input.ConfirmEmployeeCode {
			return ErrConflict
		}
		var account struct {
			ID       string
			Email    string
			Role     string
			IsActive bool
		}
		err = tx.Table("users").Select("id,email,role,is_active").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", input.UserID).Take(&account).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		// This operation never links administrator, student or other accounts.
		if !account.IsActive || (account.Role != "teacher" && account.Role != "head_teacher") || !strings.EqualFold(account.Email, input.ConfirmEmail) {
			return ErrConflict
		}
		var linked int64
		if err := tx.Table("employees").Where("user_id=?", account.ID).Count(&linked).Error; err != nil {
			return err
		}
		if linked != 0 {
			return ErrConflict
		}
		updated := tx.Table("employees").Where("id=? AND user_id IS NULL", employeeID).Updates(map[string]any{"user_id": account.ID, "updated_at": time.Now().UTC()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrConflict
		}
		return audit.Record(tx, &actor, "staff.account_linked", "employee", employeeID, map[string]any{"user_id": account.ID, "role": account.Role})
	})
	if errors.Is(dbError(err), ErrDuplicate) {
		return Employee{}, ErrConflict
	}
	if err != nil {
		return Employee{}, err
	}
	return r.GetEmployee(ctx, employeeID)
}
