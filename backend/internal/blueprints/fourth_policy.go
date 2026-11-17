package blueprints

import (
	"context"
	"fmt"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// FourthPolicy is opt-in per class. Empty policies preserve legacy behavior.
// The subject IDs must already be mapped as fourth subjects of this class.
type FourthPolicy struct {
	Enabled         bool   `json:"enabled"`
	MaleSubjectID   string `json:"male_subject_id"`
	FemaleSubjectID string `json:"female_subject_id"`
	SwapGroupID     string `json:"swap_group_id"`
	SwapSubjectAID  string `json:"swap_subject_a_id"`
	SwapSubjectBID  string `json:"swap_subject_b_id"`
}

func (r *Repository) fourthPolicy(tx *gorm.DB, classID string) (FourthPolicy, error) {
	var p FourthPolicy
	err := tx.Raw(`SELECT enabled,coalesce(male_subject_id::text,'') AS male_subject_id,
coalesce(female_subject_id::text,'') AS female_subject_id,coalesce(swap_group_id::text,'') AS swap_group_id,
coalesce(swap_subject_a_id::text,'') AS swap_subject_a_id,coalesce(swap_subject_b_id::text,'') AS swap_subject_b_id
FROM class_fourth_subject_policies WHERE class_id=?`, classID).Scan(&p).Error
	return p, err
}

// Existing explicit choices must remain applicable when a class opts in.
// This checks every session because the policy is class-wide.
func (r *Repository) ineligibleFourthChoices(tx *gorm.DB, classID string) (int64, error) {
	var count int64
	err := tx.Raw(`SELECT count(*) FROM student_subject_choices ch
JOIN student_enrolments en ON en.student_id=ch.student_id AND en.session_id=ch.session_id
JOIN academic_classes cl ON cl.id=en.class_id
LEFT JOIN academic_class_subjects acs ON acs.class_id=en.class_id AND acs.subject_id=ch.subject_id
WHERE en.class_id=? AND ch.role='optional' AND
(acs.subject_id IS NULL OR NOT acs.is_fourth_subject OR
 (cl.uses_groups AND EXISTS (SELECT 1 FROM class_group_subjects gs WHERE gs.class_id=en.class_id AND gs.subject_id=ch.subject_id)
  AND NOT EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc ON gc.group_id=gs.group_id
    WHERE gs.class_id=en.class_id AND gs.subject_id=ch.subject_id AND gc.student_id=ch.student_id AND gc.session_id=ch.session_id)))`, classID).Row().Scan(&count)
	return count, err
}

func (s *Service) SaveFourthPolicy(ctx context.Context, actor, classID string, p FourthPolicy) (FourthPolicy, error) {
	if !isUUID(actor) || !isUUID(classID) || (p.MaleSubjectID != "" && !isUUID(p.MaleSubjectID)) ||
		(p.FemaleSubjectID != "" && !isUUID(p.FemaleSubjectID)) || (p.SwapGroupID != "" && !isUUID(p.SwapGroupID)) ||
		(p.SwapSubjectAID != "" && !isUUID(p.SwapSubjectAID)) || (p.SwapSubjectBID != "" && !isUUID(p.SwapSubjectBID)) {
		return FourthPolicy{}, ErrInvalid
	}
	if p.Enabled && (p.MaleSubjectID == "" || p.FemaleSubjectID == "" || p.MaleSubjectID == p.FemaleSubjectID) {
		return FourthPolicy{}, ErrInvalid
	}
	if (p.SwapGroupID == "") != (p.SwapSubjectAID == "") || (p.SwapGroupID == "") != (p.SwapSubjectBID == "") ||
		(p.SwapGroupID != "" && p.SwapSubjectAID == p.SwapSubjectBID) {
		return FourthPolicy{}, ErrInvalid
	}
	if p.Enabled && p.SwapGroupID != "" && (p.SwapSubjectAID == p.MaleSubjectID || p.SwapSubjectAID == p.FemaleSubjectID ||
		p.SwapSubjectBID == p.MaleSubjectID || p.SwapSubjectBID == p.FemaleSubjectID) {
		return FourthPolicy{}, ErrInvalid
	}
	return s.repo.SaveFourthPolicy(ctx, actor, classID, p)
}

func (r *Repository) SaveFourthPolicy(ctx context.Context, actor, classID string, p FourthPolicy) (FourthPolicy, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var exists int64
		if err := tx.Raw("SELECT count(*) FROM academic_classes WHERE id=?", classID).Row().Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		old, err := r.fourthPolicy(tx, classID)
		if err != nil {
			return err
		}
		if old == p {
			return nil
		}
		if err := tx.Raw("SELECT count(*) FROM result_versions WHERE class_id=?", classID).Row().Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			return ErrInUse
		}
		if p.Enabled {
			ineligible, err := r.ineligibleFourthChoices(tx, classID)
			if err != nil {
				return err
			}
			if ineligible != 0 {
				return fmt.Errorf("%w: existing fourth-subject choices are ineligible; review them before enabling the policy", ErrInvalid)
			}
			for _, subject := range []string{p.MaleSubjectID, p.FemaleSubjectID} {
				if err := tx.Raw(`SELECT count(*) FROM academic_class_subjects acs JOIN academic_subjects s ON s.id=acs.subject_id
WHERE acs.class_id=? AND acs.subject_id=? AND acs.is_fourth_subject AND s.is_active`, classID, subject).Row().Scan(&exists); err != nil {
					return err
				}
				if exists == 0 {
					return ErrInvalid
				}
			}
			// Religion must be corrected through the normal academic mapping UI.
			if err := tx.Raw(`SELECT count(*) FROM academic_class_subjects acs JOIN academic_subjects s ON s.id=acs.subject_id
WHERE acs.class_id=? AND acs.is_fourth_subject AND (s.code='111' OR lower(s.name_en) LIKE '%religion%')`, classID).Row().Scan(&exists); err != nil {
				return err
			}
			if exists != 0 {
				return fmt.Errorf("%w: Religion is configured as fourth; correct the class mapping first", ErrInvalid)
			}
			if err := tx.Raw(`SELECT count(*) FROM academic_class_subjects acs JOIN academic_subjects s ON s.id=acs.subject_id
WHERE acs.class_id=? AND (s.code='111' OR lower(s.name_en) LIKE '%religion%')`, classID).Row().Scan(&exists); err != nil {
				return err
			}
			if exists > 1 {
				return fmt.Errorf("%w: multiple religion courses need student-specific applicability before this policy can be enabled", ErrInvalid)
			}
			if p.SwapGroupID != "" {
				if err := tx.Raw(`SELECT count(*) FROM academic_classes c JOIN academic_groups g ON g.id=? AND g.is_active
WHERE c.id=? AND c.uses_groups`, p.SwapGroupID, classID).Row().Scan(&exists); err != nil {
					return err
				}
				if exists == 0 {
					return ErrInvalid
				}
				for _, subject := range []string{p.SwapSubjectAID, p.SwapSubjectBID} {
					if err := tx.Raw(`SELECT count(*) FROM academic_class_subjects acs JOIN academic_subjects s ON s.id=acs.subject_id AND s.is_active JOIN class_group_subjects gs
ON gs.class_id=acs.class_id AND gs.subject_id=acs.subject_id
WHERE acs.class_id=? AND acs.subject_id=? AND acs.is_fourth_subject AND gs.group_id=?`, classID, subject, p.SwapGroupID).Row().Scan(&exists); err != nil {
						return err
					}
					if exists == 0 {
						return ErrInvalid
					}
				}
			}
		}
		res := tx.Exec(`INSERT INTO class_fourth_subject_policies
(class_id,enabled,male_subject_id,female_subject_id,swap_group_id,swap_subject_a_id,swap_subject_b_id,updated_by)
VALUES (?, ?, nullif(?,'')::uuid, nullif(?,'')::uuid, nullif(?,'')::uuid, nullif(?,'')::uuid, nullif(?,'')::uuid, ?)
ON CONFLICT (class_id) DO UPDATE SET enabled=EXCLUDED.enabled,male_subject_id=EXCLUDED.male_subject_id,
female_subject_id=EXCLUDED.female_subject_id,swap_group_id=EXCLUDED.swap_group_id,
swap_subject_a_id=EXCLUDED.swap_subject_a_id,swap_subject_b_id=EXCLUDED.swap_subject_b_id,
updated_by=EXCLUDED.updated_by,updated_at=now()`, classID, p.Enabled, p.MaleSubjectID, p.FemaleSubjectID, p.SwapGroupID, p.SwapSubjectAID, p.SwapSubjectBID, actor)
		if res.Error != nil {
			return dbErr(res.Error)
		}
		return audit.Record(tx, &actor, "exam.fourth_policy_saved", "class", classID, map[string]any{"before": old, "after": p})
	})
	if err != nil {
		return FourthPolicy{}, dbErr(err)
	}
	return p, nil
}
