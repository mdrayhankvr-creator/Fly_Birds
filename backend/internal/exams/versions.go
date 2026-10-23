package exams

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Result versions (E6, migration 1503). A published result is an immutable
// version with a snapshot: who it belongs to, the inputs and rules it was
// calculated from and the outputs that were published. Corrections add a
// version and supersede the previous one. exam_results is the projection of
// the current assessment versions; students only ever read snapshots.

// Result scopes.
const (
	ScopeAssessment = "assessment"
	ScopeTerm       = "term"
	ScopeAnnual     = "annual"
)

// Errors of the result workflow. Each satisfies errors.Is(err, ErrConflict)
// or ErrInvalid so existing handlers map them; the HTTP layer gives each its
// own code.
var (
	ErrResultsNotReady  = fmt.Errorf("%w: the results are not ready", ErrConflict)
	ErrReviewStale      = fmt.Errorf("%w: the results changed since they were reviewed", ErrConflict)
	ErrApprovalStale    = fmt.Errorf("%w: the results changed since they were approved", ErrConflict)
	ErrNotApproved      = fmt.Errorf("%w: the results have not been approved", ErrConflict)
	ErrNothingToApprove = fmt.Errorf("%w: nothing changed since publication", ErrConflict)
	ErrAlreadyPublished = fmt.Errorf("%w: the results are already published", ErrConflict)
	ErrNotPublished     = fmt.Errorf("%w: the results are not published", ErrConflict)
	ErrReasonRequired   = fmt.Errorf("%w: a reason is required", ErrInvalid)
	ErrHasVersions      = fmt.Errorf("%w: results were published for it", ErrConflict)
)

// SnapshotStudent is the student as they were when the version was made.
type SnapshotStudent struct {
	ID          string `json:"id"`
	NameEN      string `json:"name_en"`
	NameBN      string `json:"name_bn"`
	AdmissionNo string `json:"admission_no"`
	RollNo      *int   `json:"roll_no"`
	SectionID   string `json:"section_id"`
	SectionName string `json:"section_name"`
	ClassID     string `json:"class_id"`
	ClassName   string `json:"class_name"`
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
}

// SnapshotContext names what the result is of.
type SnapshotContext struct {
	ExamID        string  `json:"exam_id,omitempty"`
	ExamName      string  `json:"exam_name,omitempty"`
	ExamType      string  `json:"exam_type,omitempty"`
	TermID        *string `json:"term_id,omitempty"`
	TermName      string  `json:"term_name,omitempty"`
	TermSequence  int     `json:"term_sequence,omitempty"`
	AssessmentKey string  `json:"assessment_key,omitempty"`
	Method        string  `json:"method,omitempty"` // annual: final_only, equal or weighted
}

// Outputs are the published values.
type Outputs struct {
	Outcome         string    `json:"outcome"` // passed, failed or withheld
	Total           *string   `json:"total"`
	Full            *string   `json:"full"`
	GPA             *string   `json:"gpa"`
	Passed          bool      `json:"passed"`
	ClassPosition   *int      `json:"class_position"`
	SectionPosition *int      `json:"section_position"`
	Subjects        []OutLine `json:"subjects"`
}

// OutLine is one subject line of a published result.
type OutLine struct {
	Key           string                        `json:"key"`
	SheetID       string                        `json:"sheet_id,omitempty"`
	SubjectIDs    []string                      `json:"subject_ids"`
	Code          string                        `json:"code"`
	NameEN        string                        `json:"name_en"`
	NameBN        string                        `json:"name_bn"`
	Optional      bool                          `json:"optional"`
	Combined      string                        `json:"combined"`
	Status        string                        `json:"status"`
	Marks         *string                       `json:"marks"`
	Full          string                        `json:"full"`
	Pass          string                        `json:"pass"`
	Grade         *string                       `json:"grade"`
	Point         *string                       `json:"point"`
	Passed        *bool                         `json:"passed"`
	Reasons       []blueprints.Reason           `json:"reasons"`
	Components    []OutComponent                `json:"components"`
	Contributions []blueprints.ContributionLine `json:"contributions,omitempty"`
	Terms         []blueprints.AnnualTermLine   `json:"terms,omitempty"`
}

// OutComponent is one component of a present mark.
type OutComponent struct {
	Key   string  `json:"key"`
	Marks *string `json:"marks"`
	Full  string  `json:"full"`
	Pass  *string `json:"pass"`
}

// Snapshot is what a version stores. Inputs stay raw so a copy (a position
// update) keeps them byte for byte.
type Snapshot struct {
	Format  int             `json:"format"`
	Source  string          `json:"source"` // engine or migration
	Scope   string          `json:"scope"`
	Student SnapshotStudent `json:"student"`
	Context SnapshotContext `json:"context"`
	Inputs  json.RawMessage `json:"inputs"`
	Outputs Outputs         `json:"outputs"`
}

// candidate is one student's computed result before it is published.
type candidate struct {
	StudentID string
	Student   SnapshotStudent
	Context   SnapshotContext
	Inputs    any
	Outputs   Outputs
	// Blocker is why the result cannot be published yet ("" when it can).
	Blocker string
	ranked  *calc.Ranked
	grades  []markGrade // assessment: the grades stored on exam_marks
}

type markGrade struct {
	MarkID string
	Grade  *string
	Point  *string
	Passed *bool
}

// batch is a scope's computed results for one class (or one exam).
type batch struct {
	Scope              string
	ScopeID            string
	SessionID          string
	ClassID            string
	SectionID          *string
	BlueprintVersionID *string
	Rules              any
	Candidates         []candidate
	Blockers           []ReviewBlocker
	// Group: the other published versions the positions depend on.
	Group []string
	// others: students of other exams whose class position changes.
	others []positionChange
}

// ReviewBlocker is one reason results cannot be published yet.
type ReviewBlocker struct {
	Code    string `json:"code"`
	Detail  string `json:"detail,omitempty"`
	Count   int    `json:"count,omitempty"`
	ExamID  string `json:"exam_id,omitempty"`
	SheetID string `json:"sheet_id,omitempty"`
}

// hash binds an approval to the exact inputs: the rules, every student's
// inputs and the versions the positions depend on, canonically encoded.
func (b batch) hash() (string, error) {
	type student struct {
		ID     string `json:"id"`
		Inputs any    `json:"inputs"`
	}
	students := make([]student, 0, len(b.Candidates))
	for _, c := range b.Candidates {
		students = append(students, student{ID: c.StudentID, Inputs: c.Inputs})
	}
	sort.Slice(students, func(i, j int) bool { return students[i].ID < students[j].ID })
	group := append([]string{}, b.Group...)
	sort.Strings(group)
	raw, err := json.Marshal(struct {
		Scope    string    `json:"scope"`
		ScopeID  string    `json:"scope_id"`
		ClassID  string    `json:"class_id"`
		Rules    any       `json:"rules"`
		Students []student `json:"students"`
		Group    []string  `json:"group"`
	}{b.Scope, b.ScopeID, b.ClassID, b.Rules, students, group})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// blocked reports whether any result cannot be published.
func (b batch) blocked() bool {
	if len(b.Blockers) > 0 {
		return true
	}
	for _, c := range b.Candidates {
		if c.Blocker != "" {
			return true
		}
	}
	return false
}

// versionRow is one stored version.
type versionRow struct {
	ID              string
	PublicationID   string
	Scope           string
	ScopeID         string
	SessionID       string
	ClassID         string
	SectionID       string
	StudentID       string
	Version         int
	Status          string
	Outcome         string
	TotalMarks      *string
	FullMarks       *string
	GPA             *string
	ClassPosition   *int
	SectionPosition *int
	InputsSHA256    string
	Snapshot        string
	Reason          string
	PublishedAt     string
	PublishedBy     *string
	ClosedAt        *string
	CloseReason     string
	SupersedesID    *string
}

const versionColumns = `v.id, v.publication_id, v.scope, v.scope_id, v.session_id, v.class_id, v.section_id, v.student_id, v.version, v.status, v.outcome,
 v.total_marks::text AS total_marks, v.full_marks::text AS full_marks, v.gpa::text AS gpa, v.class_position, v.section_position,
 v.inputs_sha256, v.snapshot::text AS snapshot, v.reason, to_char(v.published_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') AS published_at, v.published_by,
 to_char(v.closed_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') AS closed_at, v.close_reason, v.supersedes_id`

// currentVersions are a scope's published versions, by student; classID
// narrows term and annual scopes to one class.
func currentVersions(tx *gorm.DB, scope, scopeID, classID string) (map[string]versionRow, error) {
	rows := []versionRow{}
	q := tx.Table("result_versions v").Select(versionColumns).Where("v.scope = ? AND v.scope_id = ? AND v.status = 'published'", scope, scopeID)
	if classID != "" {
		q = q.Where("v.class_id = ?", classID)
	}
	if err := q.Order("v.student_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]versionRow, len(rows))
	for _, r := range rows {
		out[r.StudentID] = r
	}
	return out, nil
}

func (v versionRow) snapshot() (Snapshot, error) {
	var s Snapshot
	err := json.Unmarshal([]byte(v.Snapshot), &s)
	return s, err
}

// publicationRow is one stored publication.
type publicationRow struct {
	ID                 string
	Scope              string
	ScopeID            string
	SessionID          string
	ClassID            string
	SectionID          *string
	Revision           *int
	Kind               string
	Status             string
	Source             string
	BlueprintVersionID *string
	Rules              string
	RulesSHA256        string
	InputsSHA256       string
	Reason             string
	Students           int
	Changed            int
	ApprovedBy         *string
	ApprovedByName     *string
	ApprovedAt         *string
	PublishedBy        *string
	PublishedByName    *string
	PublishedAt        *string
}

const publicationColumns = `p.id, p.scope, p.scope_id, p.session_id, p.class_id, p.section_id, p.revision, p.kind, p.status, p.source, p.blueprint_version_id,
 p.rules::text AS rules, p.rules_sha256, p.inputs_sha256, p.reason, p.students, p.changed,
 p.approved_by, (SELECT coalesce(NULLIF(u.display_name, ''), u.email) FROM users u WHERE u.id = p.approved_by) AS approved_by_name, to_char(p.approved_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') AS approved_at,
 p.published_by, (SELECT coalesce(NULLIF(u.display_name, ''), u.email) FROM users u WHERE u.id = p.published_by) AS published_by_name, to_char(p.published_at, 'YYYY-MM-DD"T"HH24:MI:SSOF') AS published_at`

// latestPublished is a scope's most recent published publication, if any.
func latestPublished(tx *gorm.DB, scope, scopeID, classID string) (*publicationRow, error) {
	var p publicationRow
	err := tx.Table("result_publications p").Select(publicationColumns).
		Where("p.scope = ? AND p.scope_id = ? AND p.class_id = ? AND p.status = 'published'", scope, scopeID, classID).
		Order("p.revision DESC").Limit(1).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

// firstPublished is the publication whose rules later corrections reuse:
// the earliest publication since the scope was last withdrawn.
func rulesPublication(tx *gorm.DB, scope, scopeID, classID string) (*publicationRow, error) {
	var p publicationRow
	err := tx.Table("result_publications p").Select(publicationColumns).
		Where("p.scope = ? AND p.scope_id = ? AND p.class_id = ? AND p.status = 'published' AND p.kind = 'publication'", scope, scopeID, classID).
		Order("p.revision DESC").Limit(1).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

// pendingApproval is the approval waiting for publication, if any.
func pendingApproval(tx *gorm.DB, scope, scopeID, classID string, lock bool) (*publicationRow, error) {
	var p publicationRow
	q := tx.Table("result_publications p").Select(publicationColumns).Where("p.scope = ? AND p.scope_id = ? AND p.class_id = ? AND p.status = 'approved'", scope, scopeID, classID)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

// insertPublication stores a publication and returns its id.
func insertPublication(tx *gorm.DB, b batch, kind, status, reason, inputsHash, actor string, students, changed int) (string, error) {
	rules, err := json.Marshal(b.Rules)
	if err != nil {
		return "", err
	}
	var id string
	q := `INSERT INTO result_publications (scope, scope_id, session_id, class_id, section_id, revision, kind, status, blueprint_version_id, rules, rules_sha256, inputs_sha256, reason, students, changed,
 approved_by, approved_at, published_by, published_at)
VALUES (?, ?, ?, ?, ?, CASE WHEN ? = 'published' THEN coalesce((SELECT max(revision) FROM result_publications WHERE scope = ? AND scope_id = ? AND class_id = ?), 0) + 1 END,
 ?, ?, ?, ?::jsonb, '', ?, ?, ?, ?,
 CASE WHEN ? = 'approved' THEN ?::uuid END, CASE WHEN ? = 'approved' THEN now() END,
 CASE WHEN ? = 'published' THEN ?::uuid END, CASE WHEN ? = 'published' THEN now() END) RETURNING id`
	err = tx.Raw(q, b.Scope, b.ScopeID, b.SessionID, b.ClassID, b.SectionID, status, b.Scope, b.ScopeID, b.ClassID,
		kind, status, b.BlueprintVersionID, string(rules), inputsHash, reason, students, changed,
		status, actor, status, status, actor, status).Row().Scan(&id)
	return id, dbErr(err)
}

// insertVersion stores a candidate as the student's next version.
func insertVersion(tx *gorm.DB, pubID string, b batch, c candidate, supersedes *string, reason, actor string) (string, error) {
	inputs, err := json.Marshal(c.Inputs)
	if err != nil {
		return "", err
	}
	snap, err := json.Marshal(Snapshot{Format: 1, Source: "engine", Scope: b.Scope, Student: c.Student, Context: c.Context, Inputs: inputs, Outputs: c.Outputs})
	if err != nil {
		return "", err
	}
	var id string
	err = tx.Raw(`INSERT INTO result_versions (publication_id, scope, scope_id, session_id, class_id, section_id, student_id, version, status, outcome,
 total_marks, full_marks, gpa, class_position, section_position, inputs_sha256, snapshot, reason, supersedes_id, published_by, published_at)
VALUES (?, ?, ?, ?, ?, ?, ?, coalesce((SELECT max(version) FROM result_versions WHERE scope = ? AND scope_id = ? AND student_id = ?), 0) + 1, 'published', ?,
 ?::numeric, ?::numeric, ?::numeric, ?, ?, '', ?::jsonb, ?, ?, ?, now()) RETURNING id`,
		pubID, b.Scope, b.ScopeID, b.SessionID, b.ClassID, c.Student.SectionID, c.StudentID, b.Scope, b.ScopeID, c.StudentID, c.Outputs.Outcome,
		c.Outputs.Total, c.Outputs.Full, c.Outputs.GPA, c.Outputs.ClassPosition, c.Outputs.SectionPosition, string(snap), reason, supersedes, actor).Row().Scan(&id)
	return id, dbErr(err)
}

// closeVersion supersedes or withdraws a published version.
func closeVersion(tx *gorm.DB, id, status, actor, reason string) error {
	res := tx.Exec("UPDATE result_versions SET status = ?, closed_by = ?, closed_at = now(), close_reason = ? WHERE id = ? AND status = 'published'", status, actor, reason, id)
	if res.Error != nil {
		return dbErr(res.Error)
	}
	if res.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}

// ---------------------------------------------------------------------------
// Comparing outputs

// changes lists what differs between a published version and a candidate.
func changes(old, next Outputs) []string {
	out := []string{}
	eq := func(a, b *string) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	eqi := func(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	if old.Outcome != next.Outcome {
		out = append(out, "outcome")
	}
	if !eq(old.GPA, next.GPA) {
		out = append(out, "gpa")
	}
	if !eq(old.Total, next.Total) || !eq(old.Full, next.Full) {
		out = append(out, "total")
	}
	if !eqi(old.ClassPosition, next.ClassPosition) {
		out = append(out, "class_position")
	}
	if !eqi(old.SectionPosition, next.SectionPosition) {
		out = append(out, "section_position")
	}
	type line struct{ marks, status, grade, point, passed, full string }
	str := func(s *string) string {
		if s == nil {
			return "—"
		}
		return *s
	}
	lines := func(o Outputs) map[string]line {
		m := map[string]line{}
		for _, l := range o.Subjects {
			p := "—"
			if l.Passed != nil {
				p = fmt.Sprint(*l.Passed)
			}
			m[l.Key] = line{str(l.Marks), l.Status, str(l.Grade), str(l.Point), p, l.Full}
		}
		return m
	}
	a, b := lines(old), lines(next)
	diff := len(a) != len(b)
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			diff = true
		}
	}
	if diff {
		out = append(out, "subjects")
	}
	return out
}

// ---------------------------------------------------------------------------
// Small helpers

// two formats hundredths as a two-decimal string.
func two(m calc.Marks) *string { s := m.String(); return &s }

func ptr[T any](v T) *T { return &v }

// marksString turns a stored float (numeric(6,2)) into its two-decimal text.
func marksString(v float64) (string, error) {
	m, err := calc.MarksOf(v)
	if err != nil {
		return "", err
	}
	return m.String(), nil
}

func recordAudit(tx *gorm.DB, actor, action, targetType, targetID string, meta map[string]any) error {
	return audit.Record(tx, &actor, action, targetType, targetID, meta)
}
