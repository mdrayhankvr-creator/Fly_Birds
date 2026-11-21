package blueprints

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"gorm.io/gorm"
)

// ReligionCorrectionRequest is an academic course request, not a statement of
// religious identity. Family requests never alter the student's choice.
type ReligionCorrectionRequest struct {
	ID                string    `json:"id"`
	StudentID         string    `json:"student_id"`
	SessionID         string    `json:"session_id"`
	ProposedSubjectID string    `json:"proposed_subject_id"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
}

type ReligionRequestOptions struct {
	CurrentSubjectID string           `json:"current_subject_id"`
	Options          []ReligionOption `json:"options"`
}

func religionRequesterAllowed(tx *gorm.DB, actor, role, studentID string) (bool, error) {
	var n int64
	var err error
	if role == "student" {
		err = tx.Raw("SELECT count(*) FROM students WHERE id=? AND user_id=?", studentID, actor).Row().Scan(&n)
	} else if role == "guardian" {
		err = tx.Raw(`SELECT count(*) FROM student_guardians sg JOIN guardians g ON g.id=sg.guardian_id
WHERE sg.student_id=? AND g.user_id=?`, studentID, actor).Row().Scan(&n)
	}
	return n == 1, err
}

func (s *Service) ReligionCorrectionOptions(ctx context.Context, actor, role, studentID, sessionID string) (ReligionRequestOptions, error) {
	if !isUUID(actor) || !isUUID(studentID) || !isUUID(sessionID) || (role != "student" && role != "guardian") {
		return ReligionRequestOptions{}, ErrInvalid
	}
	db := s.repo.db.WithContext(ctx)
	ok, err := religionRequesterAllowed(db, actor, role, studentID)
	if err != nil {
		return ReligionRequestOptions{}, err
	}
	if !ok {
		return ReligionRequestOptions{}, ErrNotFound
	}
	out := ReligionRequestOptions{Options: []ReligionOption{}}
	err = db.Raw(`SELECT sub.id,sub.code,sub.name_en,sub.name_bn FROM student_enrolments en
JOIN class_religion_subjects cr ON cr.class_id=en.class_id
JOIN academic_subjects sub ON sub.id=cr.subject_id AND sub.is_active
JOIN academic_classes c ON c.id=en.class_id
WHERE en.student_id=? AND en.session_id=? AND en.state='current' AND en.student_status='active'
AND (NOT c.uses_groups OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs
 WHERE gs.class_id=en.class_id AND gs.subject_id=cr.subject_id)
 OR EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
 ON gc.group_id=gs.group_id AND gc.student_id=en.student_id AND gc.session_id=en.session_id
 WHERE gs.class_id=en.class_id AND gs.subject_id=cr.subject_id)) ORDER BY sub.code,sub.id`, studentID, sessionID).Scan(&out.Options).Error
	if err != nil {
		return ReligionRequestOptions{}, err
	}
	var current sql.NullString
	if err := db.Raw(`SELECT subject_id::text FROM student_subject_choices
WHERE student_id=? AND session_id=? AND role='religion'`, studentID, sessionID).Row().Scan(&current); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ReligionRequestOptions{}, err
	}
	if current.Valid {
		out.CurrentSubjectID = current.String
	}
	return out, nil
}

func (s *Service) RequestReligionCorrection(ctx context.Context, actor, role, studentID, sessionID, subjectID string) (ReligionCorrectionRequest, error) {
	if !isUUID(actor) || !isUUID(studentID) || !isUUID(sessionID) || !isUUID(subjectID) || (role != "student" && role != "guardian") {
		return ReligionCorrectionRequest{}, ErrInvalid
	}
	var out ReligionCorrectionRequest
	err := s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		permitted, err := religionRequesterAllowed(tx, actor, role, studentID)
		if err != nil {
			return err
		}
		if !permitted {
			return ErrNotFound
		}
		var eligible int64
		if err := tx.Raw(`SELECT count(*) FROM student_enrolments en
JOIN class_religion_subjects cr ON cr.class_id=en.class_id AND cr.subject_id=?
JOIN academic_classes c ON c.id=en.class_id
WHERE en.student_id=? AND en.session_id=? AND en.state='current' AND en.student_status='active'
AND (NOT c.uses_groups OR NOT EXISTS (SELECT 1 FROM class_group_subjects gs
 WHERE gs.class_id=en.class_id AND gs.subject_id=cr.subject_id)
 OR EXISTS (SELECT 1 FROM class_group_subjects gs JOIN student_group_choices gc
 ON gc.group_id=gs.group_id AND gc.student_id=en.student_id AND gc.session_id=en.session_id
 WHERE gs.class_id=en.class_id AND gs.subject_id=cr.subject_id))`, subjectID, studentID, sessionID).Row().Scan(&eligible); err != nil {
			return err
		}
		if eligible != 1 {
			return ErrInvalid
		}
		if err := tx.Raw(`INSERT INTO student_religion_choice_requests
(student_id,session_id,proposed_subject_id,requested_by) VALUES (?,?,?,?)
RETURNING id,student_id,session_id,proposed_subject_id,status,created_at`, studentID, sessionID, subjectID, actor).Scan(&out).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &actor, "academic.religion_correction_requested", "religion_choice_request", out.ID, nil)
	})
	return out, dbErr(err)
}

func (s *Service) ReligionCorrectionRequests(ctx context.Context, actor string) ([]ReligionCorrectionRequest, error) {
	if !isUUID(actor) {
		return nil, ErrInvalid
	}
	var admin int64
	if err := s.repo.db.WithContext(ctx).Raw("SELECT count(*) FROM users WHERE id=? AND role='admin' AND is_active", actor).Row().Scan(&admin); err != nil {
		return nil, err
	}
	if admin != 1 {
		return nil, ErrNotFound
	}
	out := []ReligionCorrectionRequest{}
	err := s.repo.db.WithContext(ctx).Raw(`SELECT id,student_id,session_id,proposed_subject_id,status,created_at
FROM student_religion_choice_requests WHERE status='pending' ORDER BY created_at,id LIMIT 500`).Scan(&out).Error
	return out, err
}

func (s *Service) ResolveReligionCorrection(ctx context.Context, actor, requestID, decision string) error {
	if !isUUID(actor) || !isUUID(requestID) || (decision != "resolved" && decision != "rejected") {
		return ErrInvalid
	}
	return dbErr(s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var admin int64
		if err := tx.Raw("SELECT count(*) FROM users WHERE id=? AND role='admin' AND is_active", actor).Row().Scan(&admin); err != nil {
			return err
		}
		if admin != 1 {
			return ErrNotFound
		}
		var req ReligionCorrectionRequest
		if err := tx.Raw(`SELECT id,student_id,session_id,proposed_subject_id,status
FROM student_religion_choice_requests WHERE id=? FOR UPDATE`, requestID).Scan(&req).Error; err != nil {
			return err
		}
		if req.ID == "" {
			return ErrNotFound
		}
		if req.Status != "pending" {
			return ErrConflict
		}
		if decision == "resolved" {
			var n int64
			if err := tx.Raw(`SELECT count(*) FROM student_subject_choices
WHERE student_id=? AND session_id=? AND role='religion' AND subject_id=?`, req.StudentID, req.SessionID, req.ProposedSubjectID).Row().Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return ErrConflict
			}
		}
		if err := tx.Exec(`UPDATE student_religion_choice_requests SET status=?,reviewed_by=?,reviewed_at=now()
WHERE id=?`, decision, actor, requestID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "academic.religion_correction_reviewed", "religion_choice_request", requestID, map[string]any{"decision": decision})
	}))
}
