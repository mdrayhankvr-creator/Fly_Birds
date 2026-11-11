package people

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/staff"
	"gorm.io/gorm"
)

// Student photo submissions: a student uploads their own photo, and it waits
// for the class teacher of their section (or the administrator) to approve
// it. Only approval copies it into student_photos, which the profile, the ID
// card, the marksheet and the admit card read; a pending or rejected photo is
// never shown anywhere but to the student and the reviewers.

var (
	// ErrPhotoUploadsOff: the school has turned student photo uploads off.
	ErrPhotoUploadsOff = errors.New("student photo uploads are turned off")
	// ErrNoStudent: the signed-in user is not linked to a student record.
	ErrNoStudent = errors.New("no student record for this user")
	// ErrNotPending: there is no photo waiting for review.
	ErrNotPending = errors.New("no photo is waiting for review")
)

// MaxPhotoReasonRunes is the longest rejection reason.
const MaxPhotoReasonRunes = 300

// PhotoSubmission is the state of a student's own photo upload.
type PhotoSubmission struct {
	Enabled     bool       `json:"enabled"`
	Status      string     `json:"status"` // "", pending, approved or rejected
	Reason      string     `json:"reason"`
	SubmittedAt *time.Time `json:"submitted_at"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
	HasPhoto    bool       `json:"has_photo"` // an approved photo is in use
	StudentID   string     `json:"student_id"`
}

// PhotoReview is one photo waiting for a reviewer.
type PhotoReview struct {
	StudentID   string    `json:"student_id"`
	NameEN      string    `json:"name_en"`
	NameBN      string    `json:"name_bn"`
	ClassName   string    `json:"class_name"`
	SectionName string    `json:"section_name"`
	RollNo      *int      `json:"roll_no"`
	SubmittedAt time.Time `json:"submitted_at"`
	HasPhoto    bool      `json:"has_photo"`
}

// PhotoSubmissions handles student photo uploads and their review.
type PhotoSubmissions struct{ db *gorm.DB }

func NewPhotoSubmissions(db *gorm.DB) *PhotoSubmissions { return &PhotoSubmissions{db: db} }

func (p *PhotoSubmissions) enabled(tx *gorm.DB) (bool, error) {
	var on bool
	err := tx.Raw("SELECT student_photo_uploads FROM school_settings WHERE id = 1").Scan(&on).Error
	return on, err
}

func (p *PhotoSubmissions) ownStudent(tx *gorm.DB, userID string) (string, error) {
	var id string
	if err := tx.Raw("SELECT id FROM students WHERE user_id = ?", userID).Scan(&id).Error; err != nil {
		return "", err
	}
	if id == "" {
		return "", ErrNoStudent
	}
	return id, nil
}

// Mine is the signed-in student's upload state.
func (p *PhotoSubmissions) Mine(ctx context.Context, a Actor) (PhotoSubmission, error) {
	if !validID(a.ID) {
		return PhotoSubmission{}, ErrInvalid
	}
	db := p.db.WithContext(ctx)
	id, err := p.ownStudent(db, a.ID)
	if err != nil {
		return PhotoSubmission{}, err
	}
	out := PhotoSubmission{StudentID: id}
	if out.Enabled, err = p.enabled(db); err != nil {
		return out, err
	}
	var row struct {
		Status                  string
		Reason                  string
		SubmittedAt, ReviewedAt *time.Time
	}
	if err := db.Raw("SELECT status, reason, submitted_at, reviewed_at FROM student_photo_submissions WHERE student_id = ?", id).Scan(&row).Error; err != nil {
		return out, err
	}
	out.Status, out.Reason, out.SubmittedAt, out.ReviewedAt = row.Status, row.Reason, row.SubmittedAt, row.ReviewedAt
	err = db.Raw("SELECT EXISTS (SELECT 1 FROM student_photos WHERE student_id = ?)", id).Scan(&out.HasPhoto).Error
	return out, err
}

// MyImage is the signed-in student's own pending or rejected photo.
func (p *PhotoSubmissions) MyImage(ctx context.Context, a Actor) ([]byte, error) {
	if !validID(a.ID) {
		return nil, ErrInvalid
	}
	db := p.db.WithContext(ctx)
	id, err := p.ownStudent(db, a.ID)
	if err != nil {
		return nil, err
	}
	return p.image(db, id)
}

func (p *PhotoSubmissions) image(db *gorm.DB, studentID string) ([]byte, error) {
	var content []byte
	err := db.Raw("SELECT content FROM student_photo_submissions WHERE student_id = ? AND content IS NOT NULL", studentID).Row().Scan(&content)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(content) == 0) {
		return nil, ErrNotFound
	}
	return content, err
}

// Submit stores the signed-in student's photo for review, replacing any
// pending or rejected one. The photo is cropped to a centred square, scaled
// down and re-encoded as JPEG (dropping camera metadata) first.
func (p *PhotoSubmissions) Submit(ctx context.Context, a Actor, contentType string, data []byte) (PhotoSubmission, error) {
	if !validID(a.ID) {
		return PhotoSubmission{}, ErrInvalid
	}
	clean, _, err := staff.ProcessPhoto(contentType, data)
	if err != nil {
		return PhotoSubmission{}, ErrInvalid
	}
	err = p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		on, err := p.enabled(tx)
		if err != nil {
			return err
		}
		if !on {
			return ErrPhotoUploadsOff
		}
		id, err := p.ownStudent(tx, a.ID)
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO student_photo_submissions (student_id, status, content, reason, submitted_by, submitted_at, reviewed_by, reviewed_at)
VALUES (?, 'pending', ?, '', ?, now(), NULL, NULL)
ON CONFLICT (student_id) DO UPDATE SET status = 'pending', content = EXCLUDED.content, reason = '', submitted_by = EXCLUDED.submitted_by,
  submitted_at = now(), reviewed_by = NULL, reviewed_at = NULL`, id, clean, a.ID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "student.photo_submitted", "student", id, map[string]any{"bytes": len(clean)})
	})
	if err != nil {
		return PhotoSubmission{}, err
	}
	return p.Mine(ctx, a)
}

// reviewScope limits a query to the students the actor may review: every
// student for the administrator, otherwise the sections whose class teacher
// the actor is.
func reviewScope(a Actor) (string, []any) {
	if a.Role == "admin" {
		return "TRUE", nil
	}
	return `s.section_id IN (SELECT ct.section_id FROM section_class_teachers ct JOIN employees e ON e.id = ct.employee_id
 WHERE e.user_id = ? AND e.is_active)`, []any{a.ID}
}

// Pending lists the photos waiting for the actor's review, oldest first.
func (p *PhotoSubmissions) Pending(ctx context.Context, a Actor) ([]PhotoReview, error) {
	if !validID(a.ID) {
		return nil, ErrInvalid
	}
	scope, args := reviewScope(a)
	out := []PhotoReview{}
	err := p.db.WithContext(ctx).Raw(`SELECT s.id AS student_id, s.name_en, s.name_bn, c.name AS class_name, sec.name AS section_name, s.roll_no,
  ps.submitted_at, EXISTS (SELECT 1 FROM student_photos sp WHERE sp.student_id = s.id) AS has_photo
FROM student_photo_submissions ps
JOIN students s ON s.id = ps.student_id
JOIN academic_classes c ON c.id = s.class_id
JOIN academic_sections sec ON sec.id = s.section_id
WHERE ps.status = 'pending' AND s.status = 'active' AND `+scope+`
ORDER BY ps.submitted_at, s.name_en
LIMIT 200`, args...).Scan(&out).Error
	return out, err
}

func (p *PhotoSubmissions) mayReview(tx *gorm.DB, a Actor, studentID string) error {
	scope, args := reviewScope(a)
	var n int64
	if err := tx.Raw("SELECT count(*) FROM students s WHERE s.id = ? AND "+scope, append([]any{studentID}, args...)...).Scan(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReviewImage is a submitted photo, for a reviewer.
func (p *PhotoSubmissions) ReviewImage(ctx context.Context, a Actor, studentID string) ([]byte, error) {
	if !validID(a.ID) || !validID(studentID) {
		return nil, ErrInvalid
	}
	db := p.db.WithContext(ctx)
	if err := p.mayReview(db, a, studentID); err != nil {
		return nil, err
	}
	return p.image(db, studentID)
}

// Approve puts the pending photo into use and keeps no other copy of it.
func (p *PhotoSubmissions) Approve(ctx context.Context, a Actor, studentID string) error {
	if !validID(a.ID) || !validID(studentID) {
		return ErrInvalid
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := p.mayReview(tx, a, studentID); err != nil {
			return err
		}
		var content []byte
		err := tx.Raw("SELECT content FROM student_photo_submissions WHERE student_id = ? AND status = 'pending' FOR UPDATE", studentID).Row().Scan(&content)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotPending
		}
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO student_photos (student_id, content_type, content) VALUES (?, 'image/jpeg', ?)
ON CONFLICT (student_id) DO UPDATE SET content_type = EXCLUDED.content_type, content = EXCLUDED.content, updated_at = now()`, studentID, content).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE student_photo_submissions SET status = 'approved', content = NULL, reason = '', reviewed_by = ?, reviewed_at = now() WHERE student_id = ?", a.ID, studentID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &a.ID, "student.photo_approved", "student", studentID, map[string]any{"bytes": len(content)})
	})
}

// Reject turns the pending photo down with a reason the student sees.
func (p *PhotoSubmissions) Reject(ctx context.Context, a Actor, studentID, reason string) error {
	reason = strings.TrimSpace(reason)
	if !validID(a.ID) || !validID(studentID) || reason == "" || utf8.RuneCountInString(reason) > MaxPhotoReasonRunes || strings.ContainsRune(reason, 0) {
		return ErrInvalid
	}
	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := p.mayReview(tx, a, studentID); err != nil {
			return err
		}
		res := tx.Exec("UPDATE student_photo_submissions SET status = 'rejected', reason = ?, reviewed_by = ?, reviewed_at = now() WHERE student_id = ? AND status = 'pending'", reason, a.ID, studentID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotPending
		}
		return audit.Record(tx, &a.ID, "student.photo_rejected", "student", studentID, map[string]any{"reason": reason})
	})
}
