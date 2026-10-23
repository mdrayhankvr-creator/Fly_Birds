package exams

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"gorm.io/gorm"
)

// Results is the E6 review, approval and publication workflow.
type Results struct{ db *gorm.DB }

// Results returns the workflow over the exams database, or nil without one.
func (s *Service) Results() *Results {
	if r, ok := s.repo.(*GormRepository); ok && r.db != nil {
		return &Results{db: r.db}
	}
	return nil
}

// ScopeRef names one scope of one class: an exam (assessment; its class is
// the exam's), a term and class, or a session and class (annual).
type ScopeRef struct {
	Scope   string `json:"scope"`
	ScopeID string `json:"scope_id"`
	ClassID string `json:"class_id"`
}

func (ref ScopeRef) valid() bool {
	switch ref.Scope {
	case ScopeAssessment:
		return uuid.MatchString(ref.ScopeID) && (ref.ClassID == "" || uuid.MatchString(ref.ClassID))
	case ScopeTerm, ScopeAnnual:
		return uuid.MatchString(ref.ScopeID) && uuid.MatchString(ref.ClassID)
	}
	return false
}

// VersionBrief summarises a published version.
type VersionBrief struct {
	ID              string  `json:"id"`
	Version         int     `json:"version"`
	Status          string  `json:"status"`
	Outcome         string  `json:"outcome"`
	GPA             *string `json:"gpa"`
	Total           *string `json:"total"`
	Full            *string `json:"full"`
	ClassPosition   *int    `json:"class_position"`
	SectionPosition *int    `json:"section_position"`
	PublishedAt     string  `json:"published_at"`
	Reason          string  `json:"reason"`
}

func briefOf(v versionRow) *VersionBrief {
	return &VersionBrief{ID: v.ID, Version: v.Version, Status: v.Status, Outcome: v.Outcome, GPA: v.GPA, Total: v.TotalMarks, Full: v.FullMarks,
		ClassPosition: v.ClassPosition, SectionPosition: v.SectionPosition, PublishedAt: v.PublishedAt, Reason: v.Reason}
}

// PublicationView summarises a publication or approval.
type PublicationView struct {
	ID           string  `json:"id"`
	Revision     *int    `json:"revision"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	Source       string  `json:"source"`
	Reason       string  `json:"reason"`
	Students     int     `json:"students"`
	Changed      int     `json:"changed"`
	InputsSHA256 string  `json:"inputs_sha256"`
	RulesSHA256  string  `json:"rules_sha256"`
	ApprovedBy   *string `json:"approved_by"`
	ApprovedAt   *string `json:"approved_at"`
	PublishedBy  *string `json:"published_by"`
	PublishedAt  *string `json:"published_at"`
	// Stale: an approval whose inputs changed since; it must be approved again.
	Stale bool `json:"stale"`
}

func publicationView(p *publicationRow) *PublicationView {
	if p == nil {
		return nil
	}
	return &PublicationView{ID: p.ID, Revision: p.Revision, Kind: p.Kind, Status: p.Status, Source: p.Source, Reason: p.Reason, Students: p.Students,
		Changed: p.Changed, InputsSHA256: p.InputsSHA256, RulesSHA256: p.RulesSHA256, ApprovedBy: p.ApprovedByName, ApprovedAt: p.ApprovedAt,
		PublishedBy: p.PublishedByName, PublishedAt: p.PublishedAt}
}

// ReviewStudent is one student's computed result beside what is published.
type ReviewStudent struct {
	StudentID   string        `json:"student_id"`
	Name        string        `json:"name"`
	NameBN      string        `json:"name_bn"`
	AdmissionNo string        `json:"admission_no"`
	RollNo      *int          `json:"roll_no"`
	SectionName string        `json:"section_name"`
	Blocker     string        `json:"blocker"`
	Outputs     Outputs       `json:"outputs"`
	Current     *VersionBrief `json:"current"`
	Previous    *Outputs      `json:"previous,omitempty"` // the published outputs, when they change
	Change      string        `json:"change"`             // new, changed or same
	Fields      []string      `json:"fields"`
}

// ReviewSubject is one column of the review tabulation.
type ReviewSubject struct {
	Key    string `json:"key"`
	Code   string `json:"code"`
	NameEN string `json:"name_en"`
	NameBN string `json:"name_bn"`
}

// ReviewView is a scope's results for review: computed now (provisional),
// with what is published and what is approved.
type ReviewView struct {
	Scope                string           `json:"scope"`
	ScopeID              string           `json:"scope_id"`
	ClassID              string           `json:"class_id"`
	ClassName            string           `json:"class_name"`
	SessionID            string           `json:"session_id"`
	Title                string           `json:"title"`
	Locked               bool             `json:"locked"`
	State                string           `json:"state"` // blocked, ready, approved, changes or published
	Blockers             []ReviewBlocker  `json:"blockers"`
	InputsSHA256         string           `json:"inputs_sha256"`
	Rules                json.RawMessage  `json:"rules"`
	Subjects             []ReviewSubject  `json:"subjects"`
	Students             []ReviewStudent  `json:"students"`
	Published            *PublicationView `json:"published"`
	Approval             *PublicationView `json:"approval"`
	Changed              int              `json:"changed"`
	Outcomes             map[string]int   `json:"outcomes"`
	Corrections          []Correction     `json:"corrections"`
	OtherPositionChanges int              `json:"other_position_changes"`
	CanApprove           bool             `json:"can_approve"`
	Provisional          bool             `json:"provisional"`
}

// loadBatch computes a scope's results now. For an exam it needs a
// publication (first publication is the exam's own Publish).
func loadBatch(tx *gorm.DB, ref *ScopeRef) (batch, *Exam, error) {
	switch ref.Scope {
	case ScopeAssessment:
		e, err := examByID(tx, ref.ScopeID)
		if err != nil {
			return batch{}, nil, err
		}
		ref.ClassID = e.ClassID
		if e.Status == "draft" {
			return batch{}, &e, ErrNotPublished
		}
		rules, _, err := storedAssessmentRules(tx, e.ID, e.ClassID)
		if err != nil {
			return batch{}, &e, err
		}
		b, err := assessmentBatch(tx, e, rules)
		if err != nil {
			return b, &e, err
		}
		_, _, mode := rules.engine()
		return b, &e, rankGroup(tx, e, &b, mode, true)
	case ScopeTerm:
		stored, err := storedTermRules(tx, ref.ScopeID, ref.ClassID)
		if err != nil {
			return batch{}, nil, err
		}
		b, _, err := termBatch(tx, ref.ScopeID, ref.ClassID, stored)
		if errors.Is(err, blueprints.ErrNotFound) {
			err = ErrNotFound
		}
		return b, nil, err
	case ScopeAnnual:
		var n int64
		if err := tx.Raw("SELECT (SELECT count(*) FROM academic_sessions WHERE id = ?) * (SELECT count(*) FROM academic_classes WHERE id = ?)", ref.ScopeID, ref.ClassID).Row().Scan(&n); err != nil {
			return batch{}, nil, err
		}
		if n == 0 {
			return batch{}, nil, ErrNotFound
		}
		stored, err := storedAnnualRules(tx, ref.ScopeID, ref.ClassID)
		if err != nil {
			return batch{}, nil, err
		}
		b, err := annualBatch(tx, ref.ScopeID, ref.ClassID, stored)
		return b, nil, err
	}
	return batch{}, nil, ErrInvalid
}

// changedCandidates are the candidates whose outputs differ from their
// published version (all of them when nothing is published yet).
func changedCandidates(b batch, current map[string]versionRow) ([]candidate, map[string][]string, error) {
	out := []candidate{}
	fields := map[string][]string{}
	for _, c := range b.Candidates {
		v, ok := current[c.StudentID]
		if !ok {
			out = append(out, c)
			fields[c.StudentID] = []string{"new"}
			continue
		}
		snap, err := v.snapshot()
		if err != nil {
			return nil, nil, err
		}
		if f := changes(snap.Outputs, c.Outputs); len(f) > 0 {
			out = append(out, c)
			fields[c.StudentID] = f
		}
	}
	return out, fields, nil
}

func (r *Results) review(tx *gorm.DB, ref ScopeRef) (ReviewView, batch, error) {
	b, exam, err := loadBatch(tx, &ref)
	if err != nil {
		return ReviewView{}, b, err
	}
	v := ReviewView{Scope: ref.Scope, ScopeID: ref.ScopeID, ClassID: ref.ClassID, SessionID: b.SessionID, Blockers: b.Blockers, Subjects: []ReviewSubject{},
		Students: []ReviewStudent{}, Outcomes: map[string]int{}, Corrections: []Correction{}, OtherPositionChanges: len(b.others), Provisional: true}
	if v.Blockers == nil {
		v.Blockers = []ReviewBlocker{}
	}
	if v.InputsSHA256, err = b.hash(); err != nil {
		return v, b, err
	}
	rules, err := json.Marshal(b.Rules)
	if err != nil {
		return v, b, err
	}
	v.Rules = rules
	if err := tx.Raw("SELECT name FROM academic_classes WHERE id = ?", ref.ClassID).Row().Scan(&v.ClassName); err != nil {
		return v, b, err
	}
	switch ref.Scope {
	case ScopeAssessment:
		v.Title, v.Locked = exam.Name+" · "+exam.SectionName, exam.Status == "locked"
		if v.Corrections, err = corrections(tx, exam.ID, true); err != nil {
			return v, b, err
		}
	case ScopeTerm:
		if err := tx.Raw("SELECT name FROM exam_terms WHERE id = ?", ref.ScopeID).Row().Scan(&v.Title); err != nil {
			return v, b, err
		}
	case ScopeAnnual:
		var session string
		if err := tx.Raw("SELECT name FROM academic_sessions WHERE id = ?", ref.ScopeID).Row().Scan(&session); err != nil {
			return v, b, err
		}
		v.Title = session
	}
	current, err := currentVersions(tx, ref.Scope, ref.ScopeID, ref.ClassID)
	if err != nil {
		return v, b, err
	}
	_, fields, err := changedCandidates(b, current)
	if err != nil {
		return v, b, err
	}
	published, err := latestPublished(tx, ref.Scope, ref.ScopeID, ref.ClassID)
	if err != nil {
		return v, b, err
	}
	approval, err := pendingApproval(tx, ref.Scope, ref.ScopeID, ref.ClassID, false)
	if err != nil {
		return v, b, err
	}
	v.Published, v.Approval = publicationView(published), publicationView(approval)
	if v.Approval != nil {
		v.Approval.Stale = approval.InputsSHA256 != v.InputsSHA256
	}
	seen := map[string]bool{}
	for _, c := range b.Candidates {
		s := ReviewStudent{StudentID: c.StudentID, Name: c.Student.NameEN, NameBN: c.Student.NameBN, AdmissionNo: c.Student.AdmissionNo, RollNo: c.Student.RollNo,
			SectionName: c.Student.SectionName, Blocker: c.Blocker, Outputs: c.Outputs, Change: "same", Fields: []string{}}
		if cur, ok := current[c.StudentID]; ok {
			s.Current = briefOf(cur)
		}
		if f, ok := fields[c.StudentID]; ok {
			s.Change, s.Fields = "changed", f
			if s.Current == nil {
				s.Change = "new"
			} else if snap, err := current[c.StudentID].snapshot(); err == nil {
				prev := snap.Outputs
				s.Previous = &prev
			}
		}
		if published == nil {
			s.Change, s.Fields, s.Previous = "new", []string{}, nil
		} else if s.Change != "same" {
			v.Changed++
		}
		outcome := c.Outputs.Outcome
		if c.Blocker != "" {
			outcome = "incomplete"
		}
		v.Outcomes[outcome]++
		for _, l := range c.Outputs.Subjects {
			if !seen[l.Key] {
				seen[l.Key] = true
				v.Subjects = append(v.Subjects, ReviewSubject{Key: l.Key, Code: l.Code, NameEN: l.NameEN, NameBN: l.NameBN})
			}
		}
		v.Students = append(v.Students, s)
	}
	switch {
	case b.blocked():
		v.State = "blocked"
	case v.Approval != nil && !v.Approval.Stale:
		v.State = "approved"
	case published == nil:
		v.State = "ready"
	case v.Changed > 0 || len(v.Corrections) > 0 || len(b.others) > 0:
		v.State = "changes"
	default:
		v.State = "published"
	}
	v.CanApprove = !b.blocked() && (published == nil || v.Changed > 0 || len(v.Corrections) > 0 || len(b.others) > 0) && (v.Approval == nil || v.Approval.Stale)
	return v, b, nil
}

// Review computes a scope's results for staff (nothing is stored).
func (r *Results) Review(ctx context.Context, ref ScopeRef) (ReviewView, error) {
	if !ref.valid() {
		return ReviewView{}, ErrInvalid
	}
	v, _, err := r.review(r.db.WithContext(ctx), ref)
	return v, dbErr(err)
}

// ApproveInput approves exactly the reviewed results.
type ApproveInput struct {
	ScopeRef
	InputsSHA256 string `json:"inputs_sha256"`
	Reason       string `json:"reason"`
}

// Approve records that the head teacher (or an administrator) approved the
// results as reviewed. It binds to their inputs: if anything changes before
// publication, publication is refused until they are approved again.
func (r *Results) Approve(ctx context.Context, actor string, in ApproveInput) (ReviewView, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !uuid.MatchString(actor) || !in.valid() || len(in.InputsSHA256) != 64 || utf8.RuneCountInString(in.Reason) > 500 || !utf8.ValidString(in.Reason) {
		return ReviewView{}, ErrInvalid
	}
	var out ReviewView
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if in.Scope == ScopeAssessment {
			if _, err := lockExam(tx, in.ScopeID, "UPDATE"); err != nil {
				return err
			}
		}
		v, b, err := r.review(tx, in.ScopeRef)
		if err != nil {
			return err
		}
		in.ClassID = v.ClassID
		if b.blocked() {
			return ErrResultsNotReady
		}
		if v.InputsSHA256 != in.InputsSHA256 {
			return ErrReviewStale
		}
		if v.Published != nil && v.Changed == 0 && len(v.Corrections) == 0 && v.OtherPositionChanges == 0 {
			return ErrNothingToApprove
		}
		kind := "publication"
		if v.Published != nil {
			kind = "correction"
			if in.Reason == "" {
				reasons := []string{}
				for _, c := range v.Corrections {
					reasons = append(reasons, c.Reason)
				}
				in.Reason = strings.Join(reasons, "; ")
			}
			if in.Reason == "" {
				return ErrReasonRequired
			}
			if utf8.RuneCountInString(in.Reason) > 500 {
				in.Reason = string([]rune(in.Reason)[:500])
			}
		}
		if err := tx.Exec("UPDATE result_publications SET status = 'discarded' WHERE scope = ? AND scope_id = ? AND class_id = ? AND status = 'approved'", in.Scope, in.ScopeID, in.ClassID).Error; err != nil {
			return dbErr(err)
		}
		changed := v.Changed
		if v.Published == nil {
			changed = len(b.Candidates)
		}
		id, err := insertPublication(tx, b, kind, "approved", in.Reason, v.InputsSHA256, actor, len(b.Candidates), changed)
		if err != nil {
			return err
		}
		if err := recordAudit(tx, actor, "exam.results_approved", "result_publication", id, map[string]any{"scope": in.Scope, "scope_id": in.ScopeID,
			"class_id": in.ClassID, "inputs_sha256": v.InputsSHA256, "students": len(b.Candidates), "changed": changed, "reason": in.Reason}); err != nil {
			return err
		}
		out, _, err = r.review(tx, in.ScopeRef)
		return err
	})
	return out, dbErr(err)
}

// PublishInput publishes an approval.
type PublishInput struct {
	ScopeRef
	PublicationID string `json:"publication_id"`
}

// Publish publishes approved results in one transaction: every student of
// the class at once, or, for a correction, every student whose result
// changed. Previous versions are superseded, never changed or deleted.
func (r *Results) Publish(ctx context.Context, actor string, in PublishInput) (ReviewView, error) {
	if !uuid.MatchString(actor) || !in.valid() || !uuid.MatchString(in.PublicationID) {
		return ReviewView{}, ErrInvalid
	}
	var out ReviewView
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lock(tx); err != nil {
			return err
		}
		if in.Scope == ScopeAssessment {
			if _, err := lockExam(tx, in.ScopeID, "UPDATE"); err != nil {
				return err
			}
		}
		ref := in.ScopeRef
		b, exam, err := loadBatch(tx, &ref)
		if err != nil {
			return err
		}
		approval, err := pendingApproval(tx, ref.Scope, ref.ScopeID, ref.ClassID, true)
		if err != nil {
			return err
		}
		if approval == nil || approval.ID != in.PublicationID {
			var status string
			_ = tx.Raw("SELECT status FROM result_publications WHERE id = ?", in.PublicationID).Row().Scan(&status)
			if status == "published" {
				return ErrAlreadyPublished
			}
			return ErrNotApproved
		}
		if b.blocked() {
			return ErrResultsNotReady
		}
		hash, err := b.hash()
		if err != nil {
			return err
		}
		if hash != approval.InputsSHA256 {
			return ErrApprovalStale
		}
		current, err := currentVersions(tx, ref.Scope, ref.ScopeID, ref.ClassID)
		if err != nil {
			return err
		}
		published, err := latestPublished(tx, ref.Scope, ref.ScopeID, ref.ClassID)
		if err != nil {
			return err
		}
		list := b.Candidates
		if published != nil {
			if list, _, err = changedCandidates(b, current); err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE result_publications SET status = 'published', published_by = ?, published_at = now(), changed = ?, students = ?,
 revision = coalesce((SELECT max(revision) FROM result_publications x WHERE x.scope = ? AND x.scope_id = ? AND x.class_id = ?), 0) + 1 WHERE id = ?`,
			actor, len(list), len(b.Candidates), ref.Scope, ref.ScopeID, ref.ClassID, approval.ID).Error; err != nil {
			return dbErr(err)
		}
		reason := approval.Reason
		meta := map[string]any{"scope": ref.Scope, "scope_id": ref.ScopeID, "class_id": ref.ClassID, "students": len(b.Candidates), "changed": len(list), "reason": reason, "kind": approval.Kind}
		switch ref.Scope {
		case ScopeAssessment:
			if err := writeAssessment(tx, actor, approval.ID, b, list, current, reason); err != nil {
				return err
			}
			moved, err := applyPositionChanges(tx, actor, b.others, "Class position changed by a correction of "+exam.Name+" · "+exam.SectionName)
			if err != nil {
				return err
			}
			meta["position_updates"] = moved
			if err := tx.Exec("UPDATE result_corrections SET status = 'applied', closed_by = ?, closed_at = now(), publication_id = ? WHERE exam_id = ? AND status = 'open'", actor, approval.ID, exam.ID).Error; err != nil {
				return err
			}
			action := "exam.result_corrected"
			if exam.Status == "locked" {
				action = "exam.locked_result_corrected"
			}
			if err := recordAudit(tx, actor, action, "exam", exam.ID, meta); err != nil {
				return err
			}
		default:
			for _, c := range list {
				var supersedes *string
				if old, ok := current[c.StudentID]; ok {
					if err := closeVersion(tx, old.ID, "superseded", actor, reason); err != nil {
						return err
					}
					supersedes = &old.ID
				}
				if _, err := insertVersion(tx, approval.ID, b, c, supersedes, reason, actor); err != nil {
					return err
				}
			}
			action := "exam.results_published"
			if published != nil {
				action = "exam.result_corrected"
			}
			if err := recordAudit(tx, actor, action, "result_publication", approval.ID, meta); err != nil {
				return err
			}
		}
		out, _, err = r.review(tx, ref)
		return err
	})
	return out, dbErr(err)
}
