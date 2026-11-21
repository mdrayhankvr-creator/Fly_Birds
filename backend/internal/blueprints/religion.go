package blueprints

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ReligionChoice is an academic course choice. It never records or infers a
// student's religious affiliation. Only administrators receive this roster.
type ReligionChoice struct {
	StudentID string `json:"student_id"`
	SubjectID string `json:"subject_id"`
}

type ReligionOption struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameEN string `json:"name_en"`
	NameBN string `json:"name_bn"`
}

type ReligionStudent struct {
	StudentID          string   `json:"student_id"`
	AdmissionNo        string   `json:"admission_no"`
	Name               string   `json:"name"`
	SectionName        string   `json:"section_name"`
	SubjectID          string   `json:"subject_id"`
	EligibleSubjectIDs []string `json:"eligible_subject_ids" gorm:"-"`
}

type ReligionView struct {
	SessionID  string            `json:"session_id"`
	ClassID    string            `json:"class_id"`
	Options    []ReligionOption  `json:"options"`
	Students   []ReligionStudent `json:"students"`
	Warnings   []string          `json:"warnings"`
	Unresolved int               `json:"unresolved"`
	Digest     string            `json:"digest"`
}

func (r *Repository) religionView(tx *gorm.DB, sessionID, classID string) (ReligionView, error) {
	v := ReligionView{SessionID: sessionID, ClassID: classID, Options: []ReligionOption{}, Students: []ReligionStudent{}, Warnings: []string{}}
	var n int64
	if err := tx.Raw("SELECT count(*) FROM academic_sessions s JOIN academic_classes c ON c.id=? AND c.is_active WHERE s.id=? AND s.is_active", classID, sessionID).Row().Scan(&n); err != nil {
		return v, err
	}
	if n != 1 {
		return v, ErrInvalid
	}
	if err := tx.Raw(`SELECT sub.id,sub.code,sub.name_en,sub.name_bn FROM class_religion_subjects cr
JOIN academic_subjects sub ON sub.id=cr.subject_id WHERE cr.class_id=? AND sub.is_active ORDER BY sub.code,sub.id`, classID).Scan(&v.Options).Error; err != nil {
		return v, err
	}
	if err := tx.Raw(`SELECT st.id AS student_id,st.admission_no,st.name_en AS name,sec.name AS section_name,
coalesce(ch.subject_id::text,'') AS subject_id FROM student_enrolments en
JOIN students st ON st.id=en.student_id JOIN academic_sections sec ON sec.id=en.section_id
LEFT JOIN student_subject_choices ch ON ch.student_id=en.student_id AND ch.session_id=en.session_id AND ch.role='religion'
WHERE en.session_id=? AND en.class_id=? AND en.state='current' AND en.student_status='active'
ORDER BY sec.display_order,en.roll_no NULLS LAST,st.admission_no,st.id`, sessionID, classID).Scan(&v.Students).Error; err != nil {
		return v, err
	}
	rows := []struct{ StudentID, SubjectID string }{}
	if err := tx.Raw(`SELECT en.student_id,cr.subject_id FROM student_enrolments en
JOIN academic_classes c ON c.id=en.class_id JOIN class_religion_subjects cr ON cr.class_id=en.class_id
JOIN academic_subjects sub ON sub.id=cr.subject_id AND sub.is_active
WHERE en.session_id=? AND en.class_id=? AND en.state='current' AND en.student_status='active'
AND (NOT c.uses_groups OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs
  WHERE gs.class_id=en.class_id AND gs.subject_id=cr.subject_id)
 OR EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
   ON gc.group_id=gs.group_id AND gc.student_id=en.student_id AND gc.session_id=en.session_id
   WHERE gs.class_id=en.class_id AND gs.subject_id=cr.subject_id))`, sessionID, classID).Scan(&rows).Error; err != nil {
		return v, err
	}
	eligible := map[string][]string{}
	for _, row := range rows {
		eligible[row.StudentID] = append(eligible[row.StudentID], row.SubjectID)
	}
	for i := range v.Students {
		st := &v.Students[i]
		st.EligibleSubjectIDs = eligible[st.StudentID]
		if st.EligibleSubjectIDs == nil {
			st.EligibleSubjectIDs = []string{}
		}
		sort.Strings(st.EligibleSubjectIDs)
		if st.SubjectID == "" {
			v.Unresolved++
		}
	}
	if len(v.Options) == 0 {
		v.Warnings = append(v.Warnings, "no_religion_options")
	}
	var generic int64
	if err := tx.Raw(`SELECT count(*) FROM academic_class_subjects acs JOIN academic_subjects s ON s.id=acs.subject_id
WHERE acs.class_id=? AND acs.is_required AND s.code='111'
AND NOT EXISTS (SELECT 1 FROM class_religion_subjects cr WHERE cr.class_id=acs.class_id AND cr.subject_id=acs.subject_id)`, classID).Row().Scan(&generic); err != nil {
		return v, err
	}
	if generic > 0 && len(v.Options) > 0 {
		v.Warnings = append(v.Warnings, "review_generic_religion_mapping")
	}
	var published int64
	if err := tx.Raw("SELECT count(*) FROM result_versions WHERE class_id=? AND session_id=?", classID, sessionID).Row().Scan(&published); err != nil {
		return v, err
	}
	if published > 0 {
		v.Warnings = append(v.Warnings, "result_version_blocks_edits")
	}
	// The digest binds an explicit Admin roster decision to fresh assignments.
	// It is not a credential and does not include religion identity metadata.
	body, err := json.Marshal(struct {
		Options  []ReligionOption
		Students []ReligionStudent
	}{v.Options, v.Students})
	if err != nil {
		return v, err
	}
	sum := sha256.Sum256(body)
	v.Digest = hex.EncodeToString(sum[:])
	return v, nil
}

func (s *Service) ReligionChoices(ctx context.Context, sessionID, classID string) (ReligionView, error) {
	if !isUUID(sessionID) || !isUUID(classID) {
		return ReligionView{}, ErrInvalid
	}
	return s.repo.religionView(s.repo.db.WithContext(ctx), sessionID, classID)
}

func (s *Service) SaveReligionOptions(ctx context.Context, actor, sessionID, classID string, ids []string, digest string, confirmed bool) (ReligionView, error) {
	if !confirmed || !isUUID(actor) || !isUUID(sessionID) || !isUUID(classID) || len(digest) != 64 || len(ids) > 30 {
		return ReligionView{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !isUUID(id) || seen[id] {
			return ReligionView{}, ErrInvalid
		}
		seen[id] = true
	}
	err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", classID).Error; err != nil {
			return err
		}
		v, err := s.repo.religionView(tx, sessionID, classID)
		if err != nil {
			return err
		}
		if v.Digest != digest {
			return ErrConflict
		}
		var versions int64
		if err := tx.Raw("SELECT count(*) FROM result_versions WHERE class_id=?", classID).Row().Scan(&versions); err != nil {
			return err
		}
		if versions > 0 {
			return ErrInUse
		}
		for _, id := range ids {
			var n int64
			if err := tx.Raw(`SELECT count(*) FROM academic_class_subjects acs JOIN academic_subjects sub ON sub.id=acs.subject_id
WHERE acs.class_id=? AND acs.subject_id=? AND acs.is_required AND sub.is_active`, classID, id).Row().Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return ErrInvalid
			}
		}
		for _, old := range v.Options {
			if !seen[old.ID] {
				if err := tx.Exec("DELETE FROM class_religion_subjects WHERE class_id=? AND subject_id=?", classID, old.ID).Error; err != nil {
					return dbErr(err)
				}
			}
		}
		for _, id := range ids {
			if err := tx.Exec(`INSERT INTO class_religion_subjects(class_id,subject_id,created_by) VALUES (?,?,?) ON CONFLICT DO NOTHING`, classID, id, actor).Error; err != nil {
				return dbErr(err)
			}
		}
		return audit.Record(tx, &actor, "academic.religion_options_saved", "class", classID, map[string]any{"session_id": sessionID, "option_count": len(ids)})
	})
	if err != nil {
		return ReligionView{}, dbErr(err)
	}
	return s.ReligionChoices(ctx, sessionID, classID)
}

func (s *Service) SaveReligionChoices(ctx context.Context, actor, sessionID, classID string, choices []ReligionChoice, digest string, confirmed bool) (ReligionView, error) {
	if !confirmed || !isUUID(actor) || !isUUID(sessionID) || !isUUID(classID) || len(digest) != 64 || len(choices) == 0 || len(choices) > 500 {
		return ReligionView{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, c := range choices {
		if !isUUID(c.StudentID) || seen[c.StudentID] || (c.SubjectID != "" && !isUUID(c.SubjectID)) {
			return ReligionView{}, ErrInvalid
		}
		seen[c.StudentID] = true
	}
	err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", classID).Error; err != nil {
			return err
		}
		v, err := s.repo.religionView(tx, sessionID, classID)
		if err != nil {
			return err
		}
		if v.Digest != digest {
			return ErrConflict
		}
		roster := map[string]ReligionStudent{}
		for _, st := range v.Students {
			roster[st.StudentID] = st
		}
		changed := 0
		for _, c := range choices {
			st, ok := roster[c.StudentID]
			if !ok {
				return ErrInvalid
			}
			if st.SubjectID == c.SubjectID {
				continue
			}
			if c.SubjectID != "" {
				valid := false
				for _, id := range st.EligibleSubjectIDs {
					if id == c.SubjectID {
						valid = true
						break
					}
				}
				if !valid {
					return ErrInvalid
				}
			}
			if c.SubjectID == "" {
				if err := tx.Exec("DELETE FROM student_subject_choices WHERE student_id=? AND session_id=? AND role='religion'", c.StudentID, sessionID).Error; err != nil {
					return dbErr(err)
				}
			} else {
				if err := tx.Exec(`INSERT INTO student_subject_choices(student_id,session_id,role,subject_id,updated_by) VALUES (?,?,'religion',?,?)
ON CONFLICT (student_id,session_id,role) DO UPDATE SET subject_id=EXCLUDED.subject_id,updated_by=EXCLUDED.updated_by,updated_at=now()`, c.StudentID, sessionID, c.SubjectID, actor).Error; err != nil {
					return dbErr(err)
				}
			}
			changed++
		}
		// Deliberately omit student and subject identifiers from the audit event.
		return audit.Record(tx, &actor, "academic.religion_choices_saved", "class", classID, map[string]any{"session_id": sessionID, "changed": changed})
	})
	if err != nil {
		return ReligionView{}, dbErr(err)
	}
	return s.ReligionChoices(ctx, sessionID, classID)
}
