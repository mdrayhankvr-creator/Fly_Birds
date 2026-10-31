package exams

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// What result documents (E7) read. A published document is built only from
// a class's current published versions (their snapshots) and the rules of
// their publication, never from live marks. A provisional preview comes from
// the same calculation the review shows; it is never stored as a result.

// DocResult is one student's result in a document.
type DocResult struct {
	VersionID   string          `json:"version_id"` // "" in a provisional set
	Version     int             `json:"version"`
	PublishedAt string          `json:"published_at"`
	Revised     bool            `json:"revised"`
	Outcome     string          `json:"outcome"`
	Student     SnapshotStudent `json:"student"`
	Context     SnapshotContext `json:"context"`
	Inputs      json.RawMessage `json:"inputs"`
	Outputs     Outputs         `json:"outputs"`
}

// DocSet is a class's term or annual results, as documents use them.
type DocSet struct {
	ScopeRef
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	ClassName   string `json:"class_name"`
	// Title is the term's name (term) or the session's name (annual).
	Title       string `json:"title"`
	Provisional bool   `json:"provisional"`
	Revision    int    `json:"revision"`
	PublishedAt string `json:"published_at"`
	// Key changes whenever any result in the set changes: the published
	// version ids, or the inputs hash of a provisional calculation.
	Key     string               `json:"key"`
	Term    *TermRules           `json:"-"`
	Annual  *AnnualRulesSnapshot `json:"-"`
	Results []DocResult          `json:"results"`
}

// VersionRef is where a published version belongs.
type VersionRef struct {
	ScopeRef
	StudentID string
	SessionID string
	SectionID string
	Status    string
}

// DocumentVersion locates a term or annual version by id.
func (r *Results) DocumentVersion(ctx context.Context, versionID string) (VersionRef, error) {
	if !uuid.MatchString(versionID) {
		return VersionRef{}, ErrNotFound
	}
	var v VersionRef
	err := r.db.WithContext(ctx).Raw("SELECT scope, scope_id, class_id, student_id, session_id, section_id, status FROM result_versions WHERE id = ? AND scope IN ('term','annual')", versionID).
		Row().Scan(&v.Scope, &v.ScopeID, &v.ClassID, &v.StudentID, &v.SessionID, &v.SectionID, &v.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return VersionRef{}, ErrNotFound
	}
	return v, err
}

// PublishedDocSet is a class's current published term or annual results.
func (r *Results) PublishedDocSet(ctx context.Context, ref ScopeRef) (DocSet, error) {
	if (ref.Scope != ScopeTerm && ref.Scope != ScopeAnnual) || !ref.valid() {
		return DocSet{}, ErrInvalid
	}
	tx := r.db.WithContext(ctx)
	pub, err := latestPublished(tx, ref.Scope, ref.ScopeID, ref.ClassID)
	if err != nil {
		return DocSet{}, err
	}
	if pub == nil {
		return DocSet{}, ErrNotPublished
	}
	rows := []versionRow{}
	if err := tx.Table("result_versions v").Select(versionColumns).
		Where("v.scope = ? AND v.scope_id = ? AND v.class_id = ? AND v.status = 'published'", ref.Scope, ref.ScopeID, ref.ClassID).
		Order("v.id").Find(&rows).Error; err != nil {
		return DocSet{}, err
	}
	if len(rows) == 0 {
		return DocSet{}, ErrNotPublished
	}
	set := DocSet{ScopeRef: ref, SessionID: pub.SessionID, Results: make([]DocResult, 0, len(rows))}
	if pub.Revision != nil {
		set.Revision = *pub.Revision
	}
	if pub.PublishedAt != nil {
		set.PublishedAt = *pub.PublishedAt
	}
	// Corrections reuse the rules of the scope's first publication.
	rules, err := rulesPublication(tx, ref.Scope, ref.ScopeID, ref.ClassID)
	if err != nil {
		return DocSet{}, err
	}
	if rules == nil {
		rules = pub
	}
	if err := set.parseRules(rules.Rules); err != nil {
		return DocSet{}, err
	}
	keys := make([]string, 0, len(rows))
	for _, v := range rows {
		snap, err := v.snapshot()
		if err != nil {
			return DocSet{}, err
		}
		snap.Student.ID = v.StudentID
		set.Results = append(set.Results, DocResult{VersionID: v.ID, Version: v.Version, PublishedAt: v.PublishedAt, Revised: v.SupersedesID != nil,
			Outcome: v.Outcome, Student: snap.Student, Context: snap.Context, Inputs: snap.Inputs, Outputs: snap.Outputs})
		keys = append(keys, v.ID)
	}
	sort.Strings(keys)
	sum := sha256.Sum256([]byte("published\n" + strings.Join(keys, "\n")))
	set.Key = hex.EncodeToString(sum[:])
	return set, set.names(tx)
}

// ProvisionalDocSet is the calculation the review shows, for a staff-only
// preview of results that are not published yet.
func (r *Results) ProvisionalDocSet(ctx context.Context, ref ScopeRef) (DocSet, error) {
	if (ref.Scope != ScopeTerm && ref.Scope != ScopeAnnual) || !ref.valid() {
		return DocSet{}, ErrInvalid
	}
	tx := r.db.WithContext(ctx)
	b, _, err := loadBatch(tx, &ref)
	if err != nil {
		return DocSet{}, err
	}
	if len(b.Blockers) > 0 && len(b.Candidates) == 0 {
		return DocSet{}, ErrResultsNotReady
	}
	set := DocSet{ScopeRef: ref, SessionID: b.SessionID, Provisional: true, Results: make([]DocResult, 0, len(b.Candidates))}
	rules, err := json.Marshal(b.Rules)
	if err != nil {
		return DocSet{}, err
	}
	if err := set.parseRules(string(rules)); err != nil {
		return DocSet{}, err
	}
	hash, err := b.hash()
	if err != nil {
		return DocSet{}, err
	}
	set.Key = hash
	for _, c := range b.Candidates {
		inputs, err := json.Marshal(c.Inputs)
		if err != nil {
			return DocSet{}, err
		}
		st := c.Student
		st.ID = c.StudentID
		set.Results = append(set.Results, DocResult{Outcome: c.Outputs.Outcome, Student: st, Context: c.Context, Inputs: inputs, Outputs: c.Outputs})
	}
	return set, set.names(tx)
}

func (s *DocSet) parseRules(rules string) error {
	switch s.Scope {
	case ScopeTerm:
		var t TermRules
		if err := json.Unmarshal([]byte(rules), &t); err != nil {
			return err
		}
		s.Term = &t
	case ScopeAnnual:
		var a AnnualRulesSnapshot
		if err := json.Unmarshal([]byte(rules), &a); err != nil {
			return err
		}
		s.Annual = &a
	}
	return nil
}

func (s *DocSet) names(tx *gorm.DB) error {
	session, class, err := sessionClassNames(tx, s.SessionID, s.ClassID)
	if err != nil {
		return err
	}
	s.SessionName, s.ClassName, s.Title = session, class, session
	if s.Scope == ScopeTerm {
		if s.Term != nil && s.Term.TermName != "" {
			s.Title = s.Term.TermName
		} else if err := tx.Raw("SELECT name FROM exam_terms WHERE id = ?", s.ScopeID).Row().Scan(&s.Title); err != nil {
			return err
		}
	}
	return nil
}

// TermInputsOf decodes a term result's inputs.
func TermInputsOf(raw json.RawMessage) (TermInputs, error) {
	var in TermInputs
	err := json.Unmarshal(raw, &in)
	return in, err
}
