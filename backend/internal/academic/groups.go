package academic

import (
	"context"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// Groups (বিভাগ: Science, Humanities, Business Studies …) and shifts
// (প্রভাতী, দিবা …) are school-editable lists. A class opts into groups with
// uses_groups; each group's subjects are then mapped per class, and a subject
// mapped to any group is taken only by the students whose chosen group
// includes it (student_takes_subject, migration 1808). Shifts are a student
// detail printed on the transcript; schools without shifts never see the
// field.

type Group struct {
	ID           string `json:"id"`
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     bool   `json:"is_active"`
}

type GroupInput struct {
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     *bool  `json:"is_active"`
}

type Shift struct {
	ID           string `json:"id"`
	NameEN       string `json:"name_en"`
	NameBN       string `json:"name_bn"`
	DisplayOrder int    `json:"display_order"`
	IsActive     bool   `json:"is_active"`
}

type ShiftInput = GroupInput

// ClassGroup is one group's subjects in one class.
type ClassGroup struct {
	GroupID    string   `json:"group_id"`
	NameEN     string   `json:"name_en"`
	NameBN     string   `json:"name_bn"`
	IsActive   bool     `json:"is_active"`
	SubjectIDs []string `json:"subject_ids" gorm:"-"`
}

type GroupSubjectsInput struct {
	SubjectIDs []string `json:"subject_ids"`
}

func validListInput(in GroupInput) bool {
	return validText(in.NameEN, 80, true) && validText(in.NameBN, 80, false) && in.DisplayOrder >= 0 && in.DisplayOrder <= 100000 && in.IsActive != nil
}

// ---------------------------------------------------------------------------
// Service

func (s *Service) ListGroups(ctx context.Context, q Query) (Page[Group], error) {
	if !validQuery(q) {
		return Page[Group]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListGroups(ctx, q)
}

func (s *Service) SaveGroup(ctx context.Context, actor, id string, in GroupInput) (Group, error) {
	if !uuidPattern.MatchString(actor) || (id != "" && !uuidPattern.MatchString(id)) || !validListInput(in) {
		return Group{}, ErrInvalid
	}
	return s.repo.SaveGroup(ctx, actor, id, Group{NameEN: strings.TrimSpace(in.NameEN), NameBN: strings.TrimSpace(in.NameBN), DisplayOrder: in.DisplayOrder, IsActive: *in.IsActive})
}

func (s *Service) ListShifts(ctx context.Context, q Query) (Page[Shift], error) {
	if !validQuery(q) {
		return Page[Shift]{}, ErrInvalid
	}
	q.Search = strings.TrimSpace(q.Search)
	return s.repo.ListShifts(ctx, q)
}

func (s *Service) SaveShift(ctx context.Context, actor, id string, in ShiftInput) (Shift, error) {
	if !uuidPattern.MatchString(actor) || (id != "" && !uuidPattern.MatchString(id)) || !validListInput(in) {
		return Shift{}, ErrInvalid
	}
	return s.repo.SaveShift(ctx, actor, id, Shift{NameEN: strings.TrimSpace(in.NameEN), NameBN: strings.TrimSpace(in.NameBN), DisplayOrder: in.DisplayOrder, IsActive: *in.IsActive})
}

// ClassGroups lists every group with the subjects it has in the class.
func (s *Service) ClassGroups(ctx context.Context, classID string) ([]ClassGroup, error) {
	if !uuidPattern.MatchString(classID) {
		return nil, ErrInvalid
	}
	return s.repo.ClassGroups(ctx, classID)
}

// SaveGroupSubjects replaces one group's subjects in a class.
func (s *Service) SaveGroupSubjects(ctx context.Context, actor, classID, groupID string, in GroupSubjectsInput) ([]ClassGroup, error) {
	if !uuidPattern.MatchString(actor) || !uuidPattern.MatchString(classID) || !uuidPattern.MatchString(groupID) || len(in.SubjectIDs) > 100 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range in.SubjectIDs {
		if !uuidPattern.MatchString(id) || seen[id] {
			return nil, ErrInvalid
		}
		seen[id] = true
	}
	return s.repo.SaveGroupSubjects(ctx, actor, classID, groupID, in.SubjectIDs)
}

// ---------------------------------------------------------------------------
// Repository

func (r *GormRepository) ListGroups(ctx context.Context, q Query) (Page[Group], error) {
	return listNamed[Group](r.db.WithContext(ctx), "academic_groups", q)
}

func (r *GormRepository) ListShifts(ctx context.Context, q Query) (Page[Shift], error) {
	return listNamed[Shift](r.db.WithContext(ctx), "school_shifts", q)
}

func listNamed[T any](db *gorm.DB, table string, q Query) (Page[T], error) {
	base := db.Table(table)
	if q.Search != "" {
		term := "%" + q.Search + "%"
		base = base.Where("name_en ILIKE ? OR name_bn ILIKE ?", term, term)
	}
	if q.Status != "" {
		base = base.Where("is_active=?", pageBase(q))
	}
	result := Page[T]{Items: []T{}, Page: q.Page, PageSize: q.PageSize}
	if err := base.Count(&result.Total).Error; err != nil {
		return Page[T]{}, err
	}
	err := base.Select("id,name_en,name_bn,display_order,is_active").Order("display_order ASC,name_en ASC,id ASC").Limit(q.PageSize).Offset((q.Page - 1) * q.PageSize).Scan(&result.Items).Error
	return result, err
}

func (r *GormRepository) SaveGroup(ctx context.Context, actor, id string, v Group) (Group, error) {
	err := saveNamed(r.db.WithContext(ctx), actor, "academic_groups", "academic.group_saved", "academic_group", id, v.NameEN, v.NameBN, v.DisplayOrder, v.IsActive, &v.ID)
	return v, err
}

func (r *GormRepository) SaveShift(ctx context.Context, actor, id string, v Shift) (Shift, error) {
	err := saveNamed(r.db.WithContext(ctx), actor, "school_shifts", "academic.shift_saved", "school_shift", id, v.NameEN, v.NameBN, v.DisplayOrder, v.IsActive, &v.ID)
	return v, err
}

func saveNamed(db *gorm.DB, actor, table, action, entity, id, nameEN, nameBN string, order int, active bool, outID *string) error {
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		if id == "" {
			if err := tx.Raw("INSERT INTO "+table+" (name_en,name_bn,display_order,is_active) VALUES (?,?,?,?) RETURNING id", nameEN, nameBN, order, active).Scan(outID).Error; err != nil {
				return err
			}
		} else {
			updated := tx.Table(table).Where("id=?", id).Updates(map[string]any{"name_en": nameEN, "name_bn": nameBN, "display_order": order, "is_active": active, "updated_at": time.Now().UTC()})
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected == 0 {
				return ErrNotFound
			}
			*outID = id
		}
		return audit.Record(tx, &actor, action, entity, *outID, map[string]any{"created": id == ""})
	})
	return academicDBError(err)
}

func classGroupsTx(tx *gorm.DB, classID string) ([]ClassGroup, error) {
	var exists int64
	if err := tx.Table("academic_classes").Where("id=?", classID).Count(&exists).Error; err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrNotFound
	}
	groups := []ClassGroup{}
	if err := tx.Table("academic_groups").Select("id AS group_id,name_en,name_bn,is_active").Order("display_order ASC,name_en ASC,id ASC").Scan(&groups).Error; err != nil {
		return nil, err
	}
	var rows []struct{ GroupID, SubjectID string }
	if err := tx.Table("class_group_subjects AS gs").Joins("JOIN academic_subjects AS s ON s.id=gs.subject_id").
		Select("gs.group_id,gs.subject_id").Where("gs.class_id=?", classID).Order("s.code ASC,gs.subject_id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range groups {
		groups[i].SubjectIDs = []string{}
		for _, row := range rows {
			if row.GroupID == groups[i].GroupID {
				groups[i].SubjectIDs = append(groups[i].SubjectIDs, row.SubjectID)
			}
		}
	}
	return groups, nil
}

func (r *GormRepository) ClassGroups(ctx context.Context, classID string) ([]ClassGroup, error) {
	return classGroupsTx(r.db.WithContext(ctx), classID)
}

func (r *GormRepository) SaveGroupSubjects(ctx context.Context, actor, classID, groupID string, subjectIDs []string) ([]ClassGroup, error) {
	var out []ClassGroup
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireManage(tx, actor); err != nil {
			return err
		}
		var exists int64
		if err := tx.Table("academic_groups").Where("id=?", groupID).Count(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		if err := tx.Exec("DELETE FROM class_group_subjects WHERE class_id=? AND group_id=?", classID, groupID).Error; err != nil {
			return err
		}
		for _, subjectID := range subjectIDs {
			if err := tx.Exec("INSERT INTO class_group_subjects (class_id,group_id,subject_id) VALUES (?,?,?)", classID, groupID, subjectID).Error; err != nil {
				return err
			}
		}
		var err error
		out, err = classGroupsTx(tx, classID)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic.group_subjects_saved", "academic_class", classID, map[string]any{"group_id": groupID, "subjects": len(subjectIDs)})
	})
	if err != nil {
		return nil, academicDBError(err)
	}
	return out, nil
}
