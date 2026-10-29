// Package promotion moves a class of one academic session into the next:
// a recommendation per student from the published annual result, manual
// decisions with reasons, head-teacher approval bound to the exact inputs,
// and an administrator's apply (or revert) in one transaction.
package promotion

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

var (
	ErrInvalid  = errors.New("invalid promotion input")
	ErrNotFound = errors.New("promotion record not found")
	// ErrForbidden: approving needs the head teacher.
	ErrForbidden = errors.New("promotion action not allowed for this role")
	// ErrOpenBatch: the class already has an open batch for that year.
	ErrOpenBatch = errors.New("an open promotion batch exists for this class and session")
	// ErrState: the batch's status does not allow the action.
	ErrState = errors.New("the promotion batch's status does not allow this")
	// ErrBlocked: the batch has blockers (undecided students, missing sections, …).
	ErrBlocked = errors.New("the promotion batch has blockers")
	// ErrStale: the inputs changed since they were reviewed or approved.
	ErrStale = errors.New("the promotion inputs changed since review or approval")
	// ErrReasonRequired: a decision that differs from the recommendation
	// (or replaces a blocked one), or a revert, needs a reason.
	ErrReasonRequired = errors.New("a reason is required")
	// ErrRevertRefused: the new year already has records for a student, or a
	// student has changed since the apply.
	ErrRevertRefused = errors.New("the promotion can no longer be reverted")
	// ErrConflict: a student changed while applying, or a roll is taken.
	ErrConflict = errors.New("promotion conflict")

	uuidRE  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hexHash = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Actor is the signed-in user.
type Actor struct{ ID, Role string }

// Decisions an item can take.
const (
	Promote  = "promote"
	Repeat   = "repeat"
	Graduate = "graduate"
	Withdraw = "withdraw"
	Transfer = "transfer"
	Blocked  = "blocked"
)

var decisions = map[string]bool{Promote: true, Repeat: true, Graduate: true, Withdraw: true, Transfer: true}
var rollPolicies = map[string]bool{"keep": true, "merit": true, "name": true, "manual": true}

type CreateInput struct {
	FromSessionID string `json:"from_session_id"`
	ToSessionID   string `json:"to_session_id"`
	ClassID       string `json:"class_id"`
}

// ItemInput changes one student's decision or placement. Omitted fields
// keep their value; target_roll_no makes the batch's rolls manual.
type ItemInput struct {
	Decision        *string `json:"decision"`
	Reason          *string `json:"reason"`
	TargetSectionID *string `json:"target_section_id"`
	TargetRollNo    *int    `json:"target_roll_no"`
}

type HashInput struct {
	InputsSHA256 string `json:"inputs_sha256"`
}

type RevertInput struct {
	Reason string `json:"reason"`
}

type ArrangeInput struct {
	RollPolicy string `json:"roll_policy"`
}

type ProgressionInput struct {
	NextClassID *string `json:"next_class_id"`
}

type Query struct {
	SessionID, ClassID string
}

type Batch struct {
	ID              string  `json:"id"`
	FromSessionID   string  `json:"from_session_id"`
	FromSessionName string  `json:"from_session_name"`
	ToSessionID     string  `json:"to_session_id"`
	ToSessionName   string  `json:"to_session_name"`
	ClassID         string  `json:"class_id"`
	ClassName       string  `json:"class_name"`
	NextClassID     *string `json:"next_class_id"`
	NextClassName   *string `json:"next_class_name"`
	Status          string  `json:"status"`
	RollPolicy      string  `json:"roll_policy"`
	ApprovedSHA256  *string `json:"approved_sha256"`
	ApprovedBy      *string `json:"approved_by"`
	ApprovedAt      *string `json:"approved_at"`
	AppliedBy       *string `json:"applied_by"`
	AppliedAt       *string `json:"applied_at"`
	RevertedBy      *string `json:"reverted_by"`
	RevertedAt      *string `json:"reverted_at"`
	RevertReason    string  `json:"revert_reason"`
	CreatedBy       string  `json:"created_by"`
	CreatedAt       string  `json:"created_at"`
	// Live view (not stored): the hash an approval must name, what stops
	// approval or apply, and what to double-check.
	InputsSHA256 string         `json:"inputs_sha256" gorm:"-"`
	Counts       map[string]int `json:"counts" gorm:"-"`
	Blockers     []Issue        `json:"blockers" gorm:"-"`
	Warnings     []Issue        `json:"warnings" gorm:"-"`
	Items        []Item         `json:"items" gorm:"-"`
}

type Item struct {
	StudentID           string  `json:"student_id"`
	StudentName         string  `json:"student_name"`
	AdmissionNo         string  `json:"admission_no"`
	EnrolmentID         string  `json:"enrolment_id"`
	SourceSectionID     string  `json:"source_section_id"`
	SourceSectionName   string  `json:"source_section_name"`
	SourceRollNo        *int    `json:"source_roll_no"`
	AnnualVersionID     *string `json:"annual_version_id"`
	AnnualOutcome       *string `json:"annual_outcome"`
	AnnualClassPosition *int    `json:"annual_class_position"`
	Recommendation      string  `json:"recommendation"`
	Decision            *string `json:"decision"`
	Reason              string  `json:"reason"`
	DecidedBy           *string `json:"decided_by"`
	TargetClassID       *string `json:"target_class_id"`
	TargetClassName     *string `json:"target_class_name"`
	TargetSectionID     *string `json:"target_section_id"`
	TargetSectionName   *string `json:"target_section_name"`
	TargetRollNo        *int    `json:"target_roll_no"`
	AppliedEnrolmentID  *string `json:"applied_enrolment_id"`
}

// Issue is a blocker or a warning, for the batch or one student.
type Issue struct {
	Code      string `json:"code"`
	StudentID string `json:"student_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

type ClassProgression struct {
	ClassID       string  `json:"class_id"`
	ClassName     string  `json:"class_name"`
	DisplayOrder  int     `json:"display_order"`
	IsActive      bool    `json:"is_active"`
	NextClassID   *string `json:"next_class_id"`
	NextClassName *string `json:"next_class_name"`
}

type Service struct{ repo *Repository }

func NewService(db *gorm.DB) *Service { return &Service{repo: &Repository{db: db}} }

func validID(s string) bool { return uuidRE.MatchString(s) }

func cleanReason(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, utf8.ValidString(s) && utf8.RuneCountInString(s) <= 500
}

func (s *Service) List(ctx context.Context, q Query) ([]Batch, error) {
	if (q.SessionID != "" && !validID(q.SessionID)) || (q.ClassID != "" && !validID(q.ClassID)) {
		return nil, ErrInvalid
	}
	return s.repo.List(ctx, q)
}

func (s *Service) Get(ctx context.Context, id string) (Batch, error) {
	if !validID(id) {
		return Batch{}, ErrInvalid
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, a Actor, in CreateInput) (Batch, error) {
	if !validID(a.ID) || !validID(in.FromSessionID) || !validID(in.ToSessionID) || !validID(in.ClassID) || in.FromSessionID == in.ToSessionID {
		return Batch{}, ErrInvalid
	}
	return s.repo.Create(ctx, a, in)
}

func (s *Service) UpdateItem(ctx context.Context, a Actor, id, studentID string, in ItemInput) (Batch, error) {
	if !validID(a.ID) || !validID(id) || !validID(studentID) {
		return Batch{}, ErrInvalid
	}
	if in.Decision != nil && !decisions[*in.Decision] {
		return Batch{}, ErrInvalid
	}
	if in.Reason != nil {
		r, ok := cleanReason(*in.Reason)
		if !ok {
			return Batch{}, ErrInvalid
		}
		in.Reason = &r
	}
	if in.TargetSectionID != nil && !validID(*in.TargetSectionID) {
		return Batch{}, ErrInvalid
	}
	if in.TargetRollNo != nil && (*in.TargetRollNo < 1 || *in.TargetRollNo > 999999) {
		return Batch{}, ErrInvalid
	}
	if in.Decision == nil && in.Reason == nil && in.TargetSectionID == nil && in.TargetRollNo == nil {
		return Batch{}, ErrInvalid
	}
	return s.repo.UpdateItem(ctx, a, id, studentID, in)
}

func (s *Service) Arrange(ctx context.Context, a Actor, id string, in ArrangeInput) (Batch, error) {
	if !validID(a.ID) || !validID(id) || !rollPolicies[in.RollPolicy] {
		return Batch{}, ErrInvalid
	}
	return s.repo.Arrange(ctx, a, id, in.RollPolicy)
}

func (s *Service) Refresh(ctx context.Context, a Actor, id string) (Batch, error) {
	if !validID(a.ID) || !validID(id) {
		return Batch{}, ErrInvalid
	}
	return s.repo.Refresh(ctx, a, id)
}

func (s *Service) Discard(ctx context.Context, a Actor, id string) (Batch, error) {
	if !validID(a.ID) || !validID(id) {
		return Batch{}, ErrInvalid
	}
	return s.repo.Discard(ctx, a, id)
}

// Approve is the head teacher's: approval binds to the inputs reviewed.
func (s *Service) Approve(ctx context.Context, a Actor, id string, in HashInput) (Batch, error) {
	if a.Role != "head_teacher" {
		return Batch{}, ErrForbidden
	}
	if !validID(a.ID) || !validID(id) || !hexHash.MatchString(in.InputsSHA256) {
		return Batch{}, ErrInvalid
	}
	return s.repo.Approve(ctx, a, id, in.InputsSHA256)
}

// Apply is the administrator's. Applying an applied batch changes nothing.
func (s *Service) Apply(ctx context.Context, a Actor, id string) (Batch, error) {
	if a.Role != "admin" {
		return Batch{}, ErrForbidden
	}
	if !validID(a.ID) || !validID(id) {
		return Batch{}, ErrInvalid
	}
	return s.repo.Apply(ctx, a, id)
}

func (s *Service) Revert(ctx context.Context, a Actor, id string, in RevertInput) (Batch, error) {
	if a.Role != "admin" {
		return Batch{}, ErrForbidden
	}
	reason, ok := cleanReason(in.Reason)
	if !validID(a.ID) || !validID(id) || !ok {
		return Batch{}, ErrInvalid
	}
	if utf8.RuneCountInString(reason) < 3 {
		return Batch{}, ErrReasonRequired
	}
	return s.repo.Revert(ctx, a, id, reason)
}

func (s *Service) Progression(ctx context.Context) ([]ClassProgression, error) {
	return s.repo.Progression(ctx)
}

func (s *Service) SetProgression(ctx context.Context, a Actor, classID string, in ProgressionInput) ([]ClassProgression, error) {
	if !validID(a.ID) || !validID(classID) || (in.NextClassID != nil && (!validID(*in.NextClassID) || *in.NextClassID == classID)) {
		return nil, ErrInvalid
	}
	return s.repo.SetProgression(ctx, a, classID, in.NextClassID)
}
