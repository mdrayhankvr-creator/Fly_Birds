package exams

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

// Staff views of published results (E6): the publication centre's overview,
// every version of a student's result and one version with its snapshot and
// a reproducibility check.

// VersionView is one version in a student's history.
type VersionView struct {
	ID              string  `json:"id"`
	PublicationID   string  `json:"publication_id"`
	Scope           string  `json:"scope"`
	ScopeID         string  `json:"scope_id"`
	StudentID       string  `json:"student_id"`
	Version         int     `json:"version"`
	Status          string  `json:"status"`
	Kind            string  `json:"kind"`
	Revision        *int    `json:"revision"`
	Source          string  `json:"source"`
	Outcome         string  `json:"outcome"`
	GPA             *string `json:"gpa"`
	Total           *string `json:"total"`
	Full            *string `json:"full"`
	ClassPosition   *int    `json:"class_position"`
	SectionPosition *int    `json:"section_position"`
	Reason          string  `json:"reason"`
	PublishedAt     string  `json:"published_at"`
	PublishedBy     string  `json:"published_by"`
	ClosedAt        *string `json:"closed_at"`
	ClosedBy        string  `json:"closed_by"`
	CloseReason     string  `json:"close_reason"`
	InputsSHA256    string  `json:"inputs_sha256"`
	SupersedesID    *string `json:"supersedes_id"`
}

const versionViewColumns = `v.id, v.publication_id, v.scope, v.scope_id, v.student_id, v.version, v.status, p.kind, p.revision, p.source, v.outcome,
 v.gpa::text AS gpa, v.total_marks::text AS total, v.full_marks::text AS full, v.class_position, v.section_position, v.reason,
 to_char(v.published_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') AS published_at,
 coalesce((SELECT coalesce(NULLIF(u.display_name, ''), u.email) FROM users u WHERE u.id = v.published_by), '') AS published_by,
 to_char(v.closed_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') AS closed_at,
 coalesce((SELECT coalesce(NULLIF(u.display_name, ''), u.email) FROM users u WHERE u.id = v.closed_by), '') AS closed_by,
 v.close_reason, v.inputs_sha256, v.supersedes_id`

// History lists every version of a student's result in a scope, newest
// first: published, superseded and withdrawn.
func (r *Results) History(ctx context.Context, scope, scopeID, studentID string) ([]VersionView, error) {
	if (scope != ScopeAssessment && scope != ScopeTerm && scope != ScopeAnnual) || !uuid.MatchString(scopeID) || !uuid.MatchString(studentID) {
		return nil, ErrInvalid
	}
	out := []VersionView{}
	err := r.db.WithContext(ctx).Table("result_versions v").Joins("JOIN result_publications p ON p.id = v.publication_id").Select(versionViewColumns).
		Where("v.scope = ? AND v.scope_id = ? AND v.student_id = ?", scope, scopeID, studentID).Order("v.version DESC").Find(&out).Error
	return out, err
}

// Verification is whether a version can be reproduced from its snapshot.
type Verification struct {
	// HashMatches: the stored inputs hash is the hash of the stored inputs.
	HashMatches bool `json:"hash_matches"`
	// Reproduced: recalculating from the inputs and the publication's rules
	// gives exactly the published values (positions excepted: they depend on
	// the whole class and are checked per publication by exam-verify).
	Reproduced  bool     `json:"reproduced"`
	Differences []string `json:"differences"`
	Note        string   `json:"note,omitempty"`
}

// VersionDetail is one version with its snapshot, rules and verification.
type VersionDetail struct {
	Version      VersionView     `json:"version"`
	Snapshot     json.RawMessage `json:"snapshot"`
	Rules        json.RawMessage `json:"rules"`
	RulesSHA256  string          `json:"rules_sha256"`
	Verification Verification    `json:"verification"`
}

// verifyVersion recalculates one version from its snapshot.
func verifyVersion(tx *gorm.DB, v versionRow, rules string) (Verification, error) {
	out := Verification{Differences: []string{}}
	if err := tx.Raw("SELECT inputs_sha256 = encode(sha256(convert_to((snapshot->'inputs')::text, 'UTF8')), 'hex') FROM result_versions WHERE id = ?", v.ID).Row().Scan(&out.HashMatches); err != nil {
		return out, err
	}
	snap, err := v.snapshot()
	if err != nil {
		return out, err
	}
	got, err := recompute(v.Scope, snap, rules, v.SectionID)
	if err != nil {
		out.Note = "the inputs cannot be graded with the recorded rules"
		return out, nil
	}
	published := withoutPositions(snap.Outputs)
	if v.Outcome == "withheld" {
		published.Subjects, got.Subjects = nil, nil
	}
	out.Differences = changes(published, withoutPositions(got))
	out.Reproduced = len(out.Differences) == 0
	if !out.Reproduced && snap.Source == "migration" {
		out.Note = "recorded from a result published before E6; exam-verify explains differences from the pre-E2 engine"
	}
	return out, nil
}

// Version returns one version with its snapshot and verification.
func (r *Results) Version(ctx context.Context, id string) (VersionDetail, error) {
	if !uuid.MatchString(id) {
		return VersionDetail{}, ErrInvalid
	}
	tx := r.db.WithContext(ctx)
	var out VersionDetail
	if err := tx.Table("result_versions v").Joins("JOIN result_publications p ON p.id = v.publication_id").Select(versionViewColumns).Where("v.id = ?", id).Take(&out.Version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, ErrNotFound
		}
		return out, err
	}
	var row versionRow
	if err := tx.Table("result_versions v").Select(versionColumns).Where("v.id = ?", id).Take(&row).Error; err != nil {
		return out, err
	}
	var rules struct{ Rules, Sha string }
	if err := tx.Raw("SELECT rules::text AS rules, rules_sha256 AS sha FROM result_publications WHERE id = ?", row.PublicationID).Scan(&rules).Error; err != nil {
		return out, err
	}
	out.Snapshot, out.Rules, out.RulesSHA256 = json.RawMessage(row.Snapshot), json.RawMessage(rules.Rules), rules.Sha
	var err error
	out.Verification, err = verifyVersion(tx, row, rules.Rules)
	return out, err
}

// ---------------------------------------------------------------------------
// Publication centre

// OverviewCell is one class in one term (or the annual result).
type OverviewCell struct {
	ClassID     string           `json:"class_id"`
	ClassName   string           `json:"class_name"`
	Exams       int              `json:"exams"`
	Sheets      int              `json:"sheets"`
	Approved    int              `json:"approved"`
	State       string           `json:"state"` // no_exams, sheets_pending, ready, approved, published, terms_pending, no_blueprint
	Published   *PublicationView `json:"published"`
	Approval    *PublicationView `json:"approval"`
	Corrections int              `json:"corrections"`
	Missing     []string         `json:"missing,omitempty"` // annual: terms not published yet
}

// OverviewTerm is one term with its classes.
type OverviewTerm struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Sequence int            `json:"sequence"`
	Status   string         `json:"status"`
	Classes  []OverviewCell `json:"classes"`
}

// Overview is the publication centre of a session.
type Overview struct {
	SessionID   string         `json:"session_id"`
	SessionName string         `json:"session_name"`
	Terms       []OverviewTerm `json:"terms"`
	Annual      []OverviewCell `json:"annual"`
}

// Overview is where each class's term and annual results stand.
func (r *Results) Overview(ctx context.Context, sessionID string) (Overview, error) {
	if !uuid.MatchString(sessionID) {
		return Overview{}, ErrInvalid
	}
	tx := r.db.WithContext(ctx)
	out := Overview{SessionID: sessionID, Terms: []OverviewTerm{}, Annual: []OverviewCell{}}
	if err := tx.Raw("SELECT name FROM academic_sessions WHERE id = ?", sessionID).Row().Scan(&out.SessionName); err != nil {
		return out, ErrNotFound
	}
	classes := []struct{ ID, Name string }{}
	if err := tx.Raw(`SELECT DISTINCT c.id, c.name, c.display_order FROM academic_classes c
 WHERE EXISTS (SELECT 1 FROM academic_sections s WHERE s.class_id = c.id AND s.session_id = ?) ORDER BY c.display_order, c.name`, sessionID).Scan(&classes).Error; err != nil {
		return out, err
	}
	terms := []struct {
		ID, Name, Status string
		Sequence         int
	}{}
	if err := tx.Raw("SELECT id, name, status, sequence FROM exam_terms WHERE session_id = ? ORDER BY sequence", sessionID).Scan(&terms).Error; err != nil {
		return out, err
	}
	counts := []struct {
		TermID, ClassID         string
		Exams, Sheets, Approved int
		Corrections             int
	}{}
	if err := tx.Raw(`SELECT e.term_id, e.class_id, count(DISTINCT e.id) AS exams, count(es.id) AS sheets, count(es.id) FILTER (WHERE es.sheet_status = 'approved') AS approved,
       (SELECT count(*) FROM result_corrections rc JOIN exams x ON x.id = rc.exam_id WHERE x.term_id = e.term_id AND x.class_id = e.class_id AND rc.status = 'open') AS corrections
  FROM exams e LEFT JOIN exam_subjects es ON es.exam_id = e.id
 WHERE e.session_id = ? AND e.term_id IS NOT NULL GROUP BY e.term_id, e.class_id`, sessionID).Scan(&counts).Error; err != nil {
		return out, err
	}
	countOf := map[string]int{}
	for i, c := range counts {
		countOf[c.TermID+"|"+c.ClassID] = i
	}
	pubs := []publicationRow{}
	if err := tx.Table("result_publications p").Select(publicationColumns).Where("p.session_id = ? AND p.scope IN ('term','annual') AND p.status IN ('published','approved')", sessionID).
		Order("p.revision NULLS LAST").Find(&pubs).Error; err != nil {
		return out, err
	}
	published, approvals := map[string]*publicationRow{}, map[string]*publicationRow{}
	for i := range pubs {
		p := &pubs[i]
		key := p.Scope + "|" + p.ScopeID + "|" + p.ClassID
		if p.Status == "approved" {
			approvals[key] = p
		} else {
			published[key] = p // ordered by revision: the latest wins
		}
	}
	cell := func(scope, scopeID, classID, className string) OverviewCell {
		key := scope + "|" + scopeID + "|" + classID
		c := OverviewCell{ClassID: classID, ClassName: className, Published: publicationView(published[key]), Approval: publicationView(approvals[key])}
		switch {
		case c.Approval != nil:
			c.State = "approved"
		case c.Published != nil:
			c.State = "published"
		}
		return c
	}
	for _, t := range terms {
		ot := OverviewTerm{ID: t.ID, Name: t.Name, Sequence: t.Sequence, Status: t.Status, Classes: []OverviewCell{}}
		for _, cl := range classes {
			c := cell(ScopeTerm, t.ID, cl.ID, cl.Name)
			i, ok := countOf[t.ID+"|"+cl.ID]
			if !ok {
				if c.State == "" {
					continue // no exams of this class in the term
				}
			} else {
				c.Exams, c.Sheets, c.Approved, c.Corrections = counts[i].Exams, counts[i].Sheets, counts[i].Approved, counts[i].Corrections
			}
			if c.State == "" {
				c.State = "ready"
				if c.Exams == 0 {
					c.State = "no_exams"
				} else if c.Approved < c.Sheets {
					c.State = "sheets_pending"
				}
			}
			ot.Classes = append(ot.Classes, c)
		}
		out.Terms = append(out.Terms, ot)
	}
	for _, cl := range classes {
		versionID, _, err := activeVersion(tx, sessionID, cl.ID)
		if err != nil {
			return out, err
		}
		c := cell(ScopeAnnual, sessionID, cl.ID, cl.Name)
		if versionID == "" && c.State == "" {
			continue
		}
		if c.State == "" {
			d, _, err := (&Results{db: r.db}).definition(tx, versionID)
			if err != nil {
				return out, err
			}
			c.State, c.Missing = "ready", []string{}
			seen := map[string]bool{}
			for _, a := range d {
				if seen[a.ID] {
					continue
				}
				seen[a.ID] = true
				if published[ScopeTerm+"|"+a.ID+"|"+cl.ID] == nil {
					c.State = "terms_pending"
					c.Missing = append(c.Missing, a.Name)
				}
			}
		}
		out.Annual = append(out.Annual, c)
	}
	return out, nil
}

type termRef struct{ ID, Name string }

// definition lists the terms a version's assessments belong to, in order.
func (r *Results) definition(tx *gorm.DB, versionID string) ([]termRef, bool, error) {
	out := []termRef{}
	err := tx.Raw(`SELECT DISTINCT t.id, t.name, t.sequence FROM blueprint_assessments a JOIN exam_terms t ON t.id = a.term_id
 WHERE a.version_id = ? ORDER BY t.sequence`, versionID).Scan(&out).Error
	return out, false, err
}
