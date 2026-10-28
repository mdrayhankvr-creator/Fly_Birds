package blueprints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/results/calc"
	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("blueprint or term not found")
	ErrConflict = errors.New("blueprint or term conflict")
	ErrInvalid  = errors.New("invalid blueprint input")
	// ErrInUse: the term or version is used by exams and cannot change.
	ErrInUse = errors.New("in use by exams")
)

// ValidationError carries the issues that stop a version from activating.
type ValidationError struct{ Issues []Issue }

func (e *ValidationError) Error() string { return fmt.Sprintf("%d blueprint issues", len(e.Issues)) }

// Repository stores terms, blueprints, versions and subject choices.
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func dbErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return ErrConflict
		case "23503":
			return ErrInUse
		case "23514", "22P02", "23502":
			return ErrInvalid
		}
	}
	return err
}

// ---- terms -----------------------------------------------------------------

// TermView is one exam term with how many exams it has.
type TermView struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Name      string          `json:"name"`
	Sequence  int             `json:"sequence"`
	Kind      string          `json:"kind"`
	StartsOn  *string         `json:"starts_on"`
	EndsOn    *string         `json:"ends_on"`
	Status    string          `json:"status"`
	Legacy    bool            `json:"legacy"`
	Exams     int             `json:"exams"`
	Setup     json.RawMessage `json:"setup"`
}

// TermInput creates or edits a term.
type TermInput struct {
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	StartsOn  string `json:"starts_on"`
	EndsOn    string `json:"ends_on"`
}

const termColumns = `t.id, t.session_id, t.name, t.sequence, t.kind, to_char(t.starts_on, 'YYYY-MM-DD') AS starts_on,
 to_char(t.ends_on, 'YYYY-MM-DD') AS ends_on, t.status, (t.legacy_key IS NOT NULL) AS legacy, t.setup,
 (SELECT count(*) FROM exams e WHERE e.term_id = t.id) AS exams`

func (r *Repository) Terms(ctx context.Context, sessionID string) ([]TermView, error) {
	out := []TermView{}
	err := r.db.WithContext(ctx).Raw("SELECT "+termColumns+" FROM exam_terms t WHERE t.session_id = ? ORDER BY t.sequence", sessionID).Scan(&out).Error
	return out, err
}

func (r *Repository) term(tx *gorm.DB, id string) (TermView, error) {
	var t TermView
	err := tx.Raw("SELECT "+termColumns+" FROM exam_terms t WHERE t.id = ?", id).Take(&t).Error
	return t, dbErr(err)
}

func nullDate(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repository) SaveTerm(ctx context.Context, actor, id string, in TermInput) (TermView, error) {
	var out TermView
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if id == "" {
			if err := tx.Raw(`INSERT INTO exam_terms (session_id, name, sequence, kind, starts_on, ends_on, created_by)
VALUES (?, ?, coalesce((SELECT max(sequence) FROM exam_terms WHERE session_id = ?), 0) + 1, ?, ?, ?, ?) RETURNING id`,
				in.SessionID, in.Name, in.SessionID, in.Kind, nullDate(in.StartsOn), nullDate(in.EndsOn), actor).Row().Scan(&id); err != nil {
				return dbErr(err)
			}
		} else {
			old, err := r.term(tx, id)
			if err != nil {
				return err
			}
			if old.Kind != in.Kind && old.Exams > 0 {
				return ErrInUse
			}
			res := tx.Exec("UPDATE exam_terms SET name = ?, kind = ?, starts_on = ?, ends_on = ?, updated_at = now() WHERE id = ?",
				in.Name, in.Kind, nullDate(in.StartsOn), nullDate(in.EndsOn), id)
			if res.Error != nil {
				return dbErr(res.Error)
			}
		}
		var err error
		if out, err = r.term(tx, id); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.term_saved", "exam_term", id, map[string]any{"name": out.Name, "kind": out.Kind})
	})
	return out, dbErr(err)
}

func (r *Repository) DeleteTerm(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		t, err := r.term(tx, id)
		if err != nil {
			return err
		}
		var used int64
		if err := tx.Raw("SELECT (SELECT count(*) FROM exams WHERE term_id = ?) + (SELECT count(*) FROM blueprint_assessments WHERE term_id = ?)", id, id).Row().Scan(&used); err != nil {
			return err
		}
		if used > 0 || t.Legacy {
			return ErrInUse
		}
		if err := tx.Exec("DELETE FROM exam_terms WHERE id = ?", id).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &actor, "exam.term_deleted", "exam_term", id, map[string]any{"name": t.Name})
	}))
}

// ---- blueprints and versions ----------------------------------------------

// VersionSummary describes one version.
type VersionSummary struct {
	ID          string  `json:"id"`
	Version     int     `json:"version"`
	Status      string  `json:"status"`
	Legacy      bool    `json:"legacy"`
	ActivatedAt *string `json:"activated_at"`
	CreatedAt   string  `json:"created_at"`
	Exams       int     `json:"exams"`
}

// ClassBlueprint is one class of a session with its blueprint, if any.
type ClassBlueprint struct {
	ClassID     string           `json:"class_id"`
	ClassName   string           `json:"class_name"`
	BlueprintID *string          `json:"blueprint_id"`
	Name        string           `json:"name"`
	Active      *VersionSummary  `json:"active"`
	Draft       *VersionSummary  `json:"draft"`
	Versions    []VersionSummary `json:"versions"`
}

const versionColumns = `v.id, v.version, v.status, v.legacy, to_char(v.activated_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD HH24:MI') AS activated_at,
 to_char(v.created_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD HH24:MI') AS created_at, (SELECT count(*) FROM exams e WHERE e.blueprint_version_id = v.id) AS exams`

func (r *Repository) versions(tx *gorm.DB, blueprintID string) ([]VersionSummary, error) {
	out := []VersionSummary{}
	err := tx.Raw("SELECT "+versionColumns+" FROM blueprint_versions v WHERE v.blueprint_id = ? ORDER BY v.version DESC", blueprintID).Scan(&out).Error
	return out, err
}

// ClassBlueprints lists the active classes with their blueprint for a session.
func (r *Repository) ClassBlueprints(ctx context.Context, sessionID string) ([]ClassBlueprint, error) {
	tx := r.db.WithContext(ctx)
	rows := []struct {
		ClassID, ClassName string
		BlueprintID        *string
		Name               *string
	}{}
	if err := tx.Raw(`SELECT c.id AS class_id, c.name AS class_name, b.id AS blueprint_id, b.name
  FROM academic_classes c LEFT JOIN assessment_blueprints b ON b.class_id = c.id AND b.session_id = ?
 WHERE c.is_active OR b.id IS NOT NULL
 ORDER BY c.display_order, c.name`, sessionID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ClassBlueprint, 0, len(rows))
	for _, row := range rows {
		cb := ClassBlueprint{ClassID: row.ClassID, ClassName: row.ClassName, BlueprintID: row.BlueprintID, Versions: []VersionSummary{}}
		if row.Name != nil {
			cb.Name = *row.Name
		}
		if row.BlueprintID != nil {
			vs, err := r.versions(tx, *row.BlueprintID)
			if err != nil {
				return nil, err
			}
			cb.Versions = vs
			for i := range vs {
				switch vs[i].Status {
				case "active":
					cb.Active = &vs[i]
				case "draft":
					cb.Draft = &vs[i]
				}
			}
		}
		out = append(out, cb)
	}
	return out, nil
}

// TermRef, SubjectRef and BandRef describe the context a version is edited in.
type TermRef struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sequence int    `json:"sequence"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Legacy   bool   `json:"legacy"` // made by the upgrade from existing exams
}

type SubjectRef struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameEN string `json:"name_en"`
	NameBN string `json:"name_bn"`
	Fourth bool   `json:"fourth"`
}

type BandRef struct {
	Grade string  `json:"grade"`
	Min   int     `json:"min"`
	Max   int     `json:"max"`
	GPA   float64 `json:"gpa"`
}

// VersionView is one version with everything its editor needs.
type VersionView struct {
	ID            string           `json:"id"`
	BlueprintID   string           `json:"blueprint_id"`
	BlueprintName string           `json:"blueprint_name"`
	SessionID     string           `json:"session_id"`
	SessionName   string           `json:"session_name"`
	ClassID       string           `json:"class_id"`
	ClassName     string           `json:"class_name"`
	Version       int              `json:"version"`
	Status        string           `json:"status"`
	Legacy        bool             `json:"legacy"`
	ActivatedAt   *string          `json:"activated_at"`
	SHA256        string           `json:"sha256"`
	Exams         int              `json:"exams"`
	Definition    Definition       `json:"definition"`
	Terms         []TermRef        `json:"terms"`
	Subjects      []SubjectRef     `json:"subjects"`
	Bands         []BandRef        `json:"bands"`
	Issues        []Issue          `json:"issues"`
	Versions      []VersionSummary `json:"versions"`
}

type versionRow struct {
	ID, BlueprintID, BlueprintName, SessionID, SessionName, ClassID, ClassName string
	Version                                                                    int
	Status                                                                     string
	Legacy                                                                     bool
	ActivatedAt                                                                *string
	Sha                                                                        string
	Exams                                                                      int
	Definition                                                                 string
}

func (r *Repository) versionRow(tx *gorm.DB, id string, lock bool) (versionRow, error) {
	q := `SELECT v.id, v.blueprint_id, b.name AS blueprint_name, b.session_id, s.name AS session_name, b.class_id, c.name AS class_name,
       v.version, v.status, v.legacy, to_char(v.activated_at AT TIME ZONE 'Asia/Dhaka', 'YYYY-MM-DD HH24:MI') AS activated_at,
       v.definition_sha256 AS sha, (SELECT count(*) FROM exams e WHERE e.blueprint_version_id = v.id) AS exams, v.definition::text AS definition
  FROM blueprint_versions v JOIN assessment_blueprints b ON b.id = v.blueprint_id
  JOIN academic_sessions s ON s.id = b.session_id JOIN academic_classes c ON c.id = b.class_id
 WHERE v.id = ?`
	if lock {
		q += " FOR UPDATE OF v"
	}
	var row versionRow
	return row, dbErr(tx.Raw(q, id).Take(&row).Error)
}

// context loads what a definition is validated against.
func (r *Repository) context(tx *gorm.DB, sessionID, classID string) ([]TermRef, []SubjectRef, []BandRef, Context, error) {
	terms := []TermRef{}
	if err := tx.Raw("SELECT id, name, sequence, kind, status, (legacy_key IS NOT NULL) AS legacy FROM exam_terms WHERE session_id = ? ORDER BY sequence", sessionID).Scan(&terms).Error; err != nil {
		return nil, nil, nil, Context{}, err
	}
	subjects := []SubjectRef{}
	if err := tx.Raw(`SELECT s.id, s.code, s.name_en, s.name_bn, acs.is_fourth_subject AS fourth
  FROM academic_class_subjects acs JOIN academic_subjects s ON s.id = acs.subject_id
 WHERE acs.class_id = ? ORDER BY acs.is_fourth_subject, s.code`, classID).Scan(&subjects).Error; err != nil {
		return nil, nil, nil, Context{}, err
	}
	bands := []BandRef{}
	if err := tx.Raw(`SELECT b.grade, b.min_marks AS min, b.max_marks AS max, b.gpa::float8 AS gpa
  FROM grading_bands b JOIN school_settings ss ON ss.grading_scheme_id = b.scheme_id AND ss.id = 1 ORDER BY b.min_marks DESC`).Scan(&bands).Error; err != nil {
		return nil, nil, nil, Context{}, err
	}
	c := Context{Terms: map[string]Term{}, Subjects: map[string]ClassSubject{}}
	for _, t := range terms {
		c.Terms[t.ID] = Term{Name: t.Name, Sequence: t.Sequence, Kind: t.Kind}
	}
	for _, s := range subjects {
		c.Subjects[s.ID] = ClassSubject{Name: s.NameEN, Fourth: s.Fourth}
	}
	for _, b := range bands {
		p, err := calc.PointsOf(b.GPA)
		if err != nil {
			return nil, nil, nil, Context{}, err
		}
		c.Bands = append(c.Bands, calc.Band{Grade: b.Grade, Min: b.Min, Max: b.Max, Point: p})
	}
	return terms, subjects, bands, c, nil
}

// legacyDefinition rebuilds a legacy version's design from its normalised
// rows: one assessment per term, one total per subject, final only.
func (r *Repository) legacyDefinition(tx *gorm.DB, versionID string) (Definition, error) {
	d := Definition{}
	if err := tx.Raw("SELECT key, name, term_id, kind, student_visibility AS visibility FROM blueprint_assessments WHERE version_id = ? ORDER BY sequence", versionID).Scan(&d.Assessments).Error; err != nil {
		return d, err
	}
	subjects := []struct{ ID, SubjectID, Role string }{}
	if err := tx.Raw("SELECT id, subject_id, role FROM blueprint_subjects WHERE version_id = ? ORDER BY display_order", versionID).Scan(&subjects).Error; err != nil {
		return d, err
	}
	for _, s := range subjects {
		comps := []struct {
			AssessmentKey string
			Full          float64
			Pass          *float64
		}{}
		if err := tx.Raw("SELECT assessment_key, full_marks::float8 AS full, pass_marks::float8 AS pass FROM blueprint_components WHERE blueprint_subject_id = ? ORDER BY assessment_key", s.ID).Scan(&comps).Error; err != nil {
			return d, err
		}
		rule := SubjectRule{SubjectID: s.SubjectID, Role: s.Role}
		for _, c := range comps {
			a, _ := d.Assessment(c.AssessmentKey)
			pass := 0.0
			if c.Pass != nil {
				pass = *c.Pass
			}
			rule.Sheets = append(rule.Sheets, Sheet{Assessment: c.AssessmentKey, Pass: pass, Components: []Component{{Key: "TOTAL", Full: c.Full}}})
			rule.Terms = append(rule.Terms, TermRule{TermID: a.TermID, Full: c.Full, Pass: pass, Contributions: []Contribution{{From: []string{c.AssessmentKey}, Combine: "single", Worth: c.Full}}})
		}
		d.Subjects = append(d.Subjects, rule)
	}
	d.Defaults()
	return d, nil
}

func (r *Repository) definitionOf(tx *gorm.DB, row versionRow) (Definition, error) {
	if row.Legacy {
		return r.legacyDefinition(tx, row.ID)
	}
	var d Definition
	if err := json.Unmarshal([]byte(row.Definition), &d); err != nil {
		return d, err
	}
	d.Defaults()
	return d, nil
}

// Version returns a version with its definition, context and current issues.
func (r *Repository) Version(ctx context.Context, id string) (VersionView, error) {
	return r.view(r.db.WithContext(ctx), id)
}

func (r *Repository) view(tx *gorm.DB, id string) (VersionView, error) {
	row, err := r.versionRow(tx, id, false)
	if err != nil {
		return VersionView{}, err
	}
	d, err := r.definitionOf(tx, row)
	if err != nil {
		return VersionView{}, err
	}
	terms, subjects, bands, c, err := r.context(tx, row.SessionID, row.ClassID)
	if err != nil {
		return VersionView{}, err
	}
	v := VersionView{ID: row.ID, BlueprintID: row.BlueprintID, BlueprintName: row.BlueprintName, SessionID: row.SessionID, SessionName: row.SessionName,
		ClassID: row.ClassID, ClassName: row.ClassName, Version: row.Version, Status: row.Status, Legacy: row.Legacy, ActivatedAt: row.ActivatedAt,
		SHA256: row.Sha, Exams: row.Exams, Definition: d, Terms: terms, Subjects: subjects, Bands: bands, Issues: []Issue{}}
	if row.Status == "draft" {
		v.Issues = Validate(d, c)
	}
	if v.Versions, err = r.versions(tx, row.BlueprintID); err != nil {
		return VersionView{}, err
	}
	return v, nil
}

// seed is the starting design for a class without a blueprint: one term
// exam per term of the session (not the terms the upgrade made from existing
// exams, which belong to the legacy blueprint), every class subject out of
// 100 (pass 33), final only — today's model.
func seed(terms []TermRef, subjects []SubjectRef) Definition {
	d := Definition{}
	for _, t := range terms {
		if t.Legacy {
			continue
		}
		d.Assessments = append(d.Assessments, Assessment{Key: fmt.Sprintf("EX%d", t.Sequence), Name: t.Name, TermID: t.ID, Kind: "term_exam", Visibility: "with_assessment_publication"})
	}
	for _, s := range subjects {
		rule := SubjectRule{SubjectID: s.ID, Role: "required"}
		if s.Fourth {
			rule.Role = "optional"
		}
		for _, a := range d.Assessments {
			rule.Sheets = append(rule.Sheets, Sheet{Assessment: a.Key, Pass: 33, Components: []Component{{Key: "TOTAL", Full: 100}}})
			rule.Terms = append(rule.Terms, TermRule{TermID: a.TermID, Full: 100, Pass: 33, Contributions: []Contribution{{From: []string{a.Key}, Combine: "single", Worth: 100}}})
		}
		d.Subjects = append(d.Subjects, rule)
	}
	d.Defaults()
	return d
}

// mapTerms rewrites a definition from another session onto this one by term
// sequence, creating any term the new session lacks (in planning).
func (r *Repository) mapTerms(tx *gorm.DB, actor string, d Definition, fromSession, toSession string) (Definition, error) {
	if fromSession == toSession {
		return d, nil
	}
	from := []TermRef{}
	if err := tx.Raw("SELECT id, name, sequence, kind, status FROM exam_terms WHERE session_id = ?", fromSession).Scan(&from).Error; err != nil {
		return d, err
	}
	ids := map[string]string{}
	for _, t := range from {
		var id string
		err := tx.Raw("SELECT id FROM exam_terms WHERE session_id = ? AND sequence = ?", toSession, t.Sequence).Row().Scan(&id)
		if err != nil {
			if err := tx.Raw(`INSERT INTO exam_terms (session_id, name, sequence, kind, created_by) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (session_id, lower(name)) DO UPDATE SET updated_at = now() RETURNING id`, toSession, t.Name, t.Sequence, t.Kind, actor).Row().Scan(&id); err != nil {
				return d, dbErr(err)
			}
		}
		ids[t.ID] = id
	}
	b, _ := json.Marshal(d)
	var out Definition
	_ = json.Unmarshal(b, &out)
	for i := range out.Assessments {
		out.Assessments[i].TermID = ids[out.Assessments[i].TermID]
	}
	for i := range out.Subjects {
		for j := range out.Subjects[i].Terms {
			out.Subjects[i].Terms[j].TermID = ids[out.Subjects[i].Terms[j].TermID]
		}
	}
	if len(out.Annual.Weights) > 0 {
		w := map[string]int{}
		for k, v := range out.Annual.Weights {
			w[ids[k]] = v
		}
		out.Annual.Weights = w
	}
	return out, nil
}

// CreateBlueprint opens a draft for a class: from a copied version (copy
// forward from last session, or another class) or from the seed design.
func (r *Repository) CreateBlueprint(ctx context.Context, actor, sessionID, classID, copyFrom string) (VersionView, error) {
	var id string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var exists int64
		if err := tx.Raw("SELECT count(*) FROM academic_classes WHERE id = ?", classID).Row().Scan(&exists); err != nil || exists == 0 {
			return ErrNotFound
		}
		var blueprintID string
		if err := tx.Raw(`INSERT INTO assessment_blueprints (session_id, class_id, name, created_by)
SELECT ?, ?, left(c.name || ' · ' || s.name, 160), ? FROM academic_classes c, academic_sessions s WHERE c.id = ? AND s.id = ?
ON CONFLICT (session_id, class_id) DO UPDATE SET updated_at = now() RETURNING id`, sessionID, classID, actor, classID, sessionID).Row().Scan(&blueprintID); err != nil {
			return dbErr(err)
		}
		var d Definition
		if copyFrom != "" {
			src, err := r.versionRow(tx, copyFrom, false)
			if err != nil {
				return err
			}
			if d, err = r.definitionOf(tx, src); err != nil {
				return err
			}
			if d, err = r.mapTerms(tx, actor, d, src.SessionID, sessionID); err != nil {
				return err
			}
		} else {
			terms, subjects, _, _, err := r.context(tx, sessionID, classID)
			if err != nil {
				return err
			}
			d = seed(terms, subjects)
		}
		var err error
		id, err = r.insertDraft(tx, actor, blueprintID, d)
		if err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.blueprint_drafted", "blueprint_version", id, map[string]any{"class_id": classID, "session_id": sessionID, "copied_from": copyFrom})
	})
	if err != nil {
		return VersionView{}, dbErr(err)
	}
	return r.Version(ctx, id)
}

func (r *Repository) insertDraft(tx *gorm.DB, actor, blueprintID string, d Definition) (string, error) {
	var draft int64
	if err := tx.Raw("SELECT count(*) FROM blueprint_versions WHERE blueprint_id = ? AND status = 'draft'", blueprintID).Row().Scan(&draft); err != nil {
		return "", err
	}
	if draft > 0 {
		return "", ErrConflict // one draft at a time: edit it or delete it
	}
	body, err := d.Canonical()
	if err != nil {
		return "", err
	}
	var id string
	err = tx.Raw(`INSERT INTO blueprint_versions (blueprint_id, version, definition, created_by)
VALUES (?, coalesce((SELECT max(version) FROM blueprint_versions WHERE blueprint_id = ?), 0) + 1, ?::jsonb, ?) RETURNING id`,
		blueprintID, blueprintID, string(body), actor).Row().Scan(&id)
	return id, dbErr(err)
}

// NewDraft opens the next version of a blueprint from one of its versions.
func (r *Repository) NewDraft(ctx context.Context, actor, blueprintID, fromVersion string) (VersionView, error) {
	var id string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		src, err := r.versionRow(tx, fromVersion, false)
		if err != nil {
			return err
		}
		if src.BlueprintID != blueprintID {
			return ErrInvalid
		}
		d, err := r.definitionOf(tx, src)
		if err != nil {
			return err
		}
		if id, err = r.insertDraft(tx, actor, blueprintID, d); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.blueprint_drafted", "blueprint_version", id, map[string]any{"from_version": src.Version})
	})
	if err != nil {
		return VersionView{}, dbErr(err)
	}
	return r.Version(ctx, id)
}

// SaveDraft replaces a draft's definition and returns it with its issues.
func (r *Repository) SaveDraft(ctx context.Context, actor, id string, d Definition) (VersionView, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.versionRow(tx, id, true)
		if err != nil {
			return err
		}
		if row.Status != "draft" {
			return ErrInUse
		}
		d.Defaults()
		body, err := d.Canonical()
		if err != nil {
			return err
		}
		return tx.Exec("UPDATE blueprint_versions SET definition = ?::jsonb, updated_at = now() WHERE id = ?", string(body), id).Error
	})
	if err != nil {
		return VersionView{}, dbErr(err)
	}
	return r.Version(ctx, id)
}

func (r *Repository) DeleteDraft(ctx context.Context, actor, id string) error {
	return dbErr(r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.versionRow(tx, id, true)
		if err != nil {
			return err
		}
		if row.Status != "draft" {
			return ErrInUse
		}
		if err := tx.Exec("DELETE FROM blueprint_versions WHERE id = ?", id).Error; err != nil {
			return err
		}
		// A blueprint left without any version goes too.
		if err := tx.Exec("DELETE FROM assessment_blueprints b WHERE b.id = ? AND NOT EXISTS (SELECT 1 FROM blueprint_versions v WHERE v.blueprint_id = b.id)", row.BlueprintID).Error; err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.blueprint_draft_deleted", "blueprint_version", id, map[string]any{"version": row.Version})
	}))
}

// Activate validates a draft, writes its normalised rows, freezes it and
// retires the previously active version (whose exams keep it).
func (r *Repository) Activate(ctx context.Context, actor, id string) (VersionView, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.versionRow(tx, id, true)
		if err != nil {
			return err
		}
		if row.Status != "draft" {
			return ErrInUse
		}
		d, err := r.definitionOf(tx, row)
		if err != nil {
			return err
		}
		_, _, bands, c, err := r.context(tx, row.SessionID, row.ClassID)
		if err != nil {
			return err
		}
		if issues := Validate(d, c); len(issues) > 0 {
			return &ValidationError{Issues: issues}
		}
		sha, err := d.SHA256()
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(bands)
		for i, a := range d.Assessments {
			if err := tx.Exec("INSERT INTO blueprint_assessments (version_id, key, name, term_id, kind, sequence, student_visibility) VALUES (?, ?, ?, ?, ?, ?, ?)",
				id, a.Key, a.Name, a.TermID, a.Kind, i+1, a.Visibility).Error; err != nil {
				return dbErr(err)
			}
		}
		for i, s := range d.Subjects {
			var combined any
			if s.CombinedKey != "" {
				combined = s.CombinedKey
			}
			var sid string
			if err := tx.Raw("INSERT INTO blueprint_subjects (version_id, subject_id, role, combined_key, display_order) VALUES (?, ?, ?, ?, ?) RETURNING id",
				id, s.SubjectID, s.Role, combined, i+1).Row().Scan(&sid); err != nil {
				return dbErr(err)
			}
			for _, sh := range s.Sheets {
				for k, comp := range sh.Components {
					full, _ := calc.MarksOf(comp.Full)
					var pass any
					if comp.Pass != nil {
						p, _ := calc.MarksOf(*comp.Pass)
						pass = p.String()
					}
					if err := tx.Exec("INSERT INTO blueprint_components (blueprint_subject_id, assessment_key, component_key, full_marks, pass_marks, display_order) VALUES (?, ?, ?, ?::numeric, ?::numeric, ?)",
						sid, sh.Assessment, comp.Key, full.String(), pass, k+1).Error; err != nil {
						return dbErr(err)
					}
				}
			}
		}
		if err := tx.Exec("UPDATE blueprint_versions SET status = 'retired', updated_at = now() WHERE blueprint_id = ? AND status = 'active'", row.BlueprintID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE blueprint_versions SET status = 'active', definition_sha256 = ?, grading_scheme_snapshot = ?::jsonb,
 gpa_rounding = ?, absent_policy = ?, rank_mode = ?, paper_rule = ?, activated_by = ?, activated_at = now(), updated_at = now() WHERE id = ?`,
			sha, string(snapshot), d.Policy.GPARounding, d.Policy.Absent, d.Policy.Ranking, d.PaperRule, actor, id).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &actor, "exam.blueprint_activated", "blueprint_version", id, map[string]any{"version": row.Version, "sha256": sha, "class_id": row.ClassID, "session_id": row.SessionID})
	})
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			return VersionView{}, err
		}
		return VersionView{}, dbErr(err)
	}
	return r.Version(ctx, id)
}

// SetVisibility changes who among students and guardians sees an
// assessment's own results. It is a display policy, so an active version may
// change it; the change is audited.
func (r *Repository) SetVisibility(ctx context.Context, actor, versionID, key, visibility string) (VersionView, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := r.versionRow(tx, versionID, true)
		if err != nil {
			return err
		}
		if row.Status == "draft" {
			var d Definition
			if err := json.Unmarshal([]byte(row.Definition), &d); err != nil {
				return err
			}
			found := false
			for i := range d.Assessments {
				if d.Assessments[i].Key == key {
					d.Assessments[i].Visibility, found = visibility, true
				}
			}
			if !found {
				return ErrNotFound
			}
			body, _ := d.Canonical()
			return tx.Exec("UPDATE blueprint_versions SET definition = ?::jsonb, updated_at = now() WHERE id = ?", string(body), versionID).Error
		}
		var old string
		if err := tx.Raw("SELECT student_visibility FROM blueprint_assessments WHERE version_id = ? AND key = ? FOR UPDATE", versionID, key).Row().Scan(&old); err != nil {
			return ErrNotFound
		}
		if err := tx.Exec("UPDATE blueprint_assessments SET student_visibility = ? WHERE version_id = ? AND key = ?", visibility, versionID, key).Error; err != nil {
			return dbErr(err)
		}
		return audit.Record(tx, &actor, "exam.assessment_visibility_changed", "blueprint_version", versionID, map[string]any{"assessment": key, "from": old, "to": visibility})
	})
	if err != nil {
		return VersionView{}, dbErr(err)
	}
	return r.Version(ctx, versionID)
}

// ---- optional subject choices ---------------------------------------------

// ChoiceStudent is a student of the class with their optional subject.
type ChoiceStudent struct {
	StudentID   string  `json:"student_id"`
	Name        string  `json:"name"`
	AdmissionNo string  `json:"admission_no"`
	RollNo      *int    `json:"roll_no"`
	SectionName string  `json:"section_name"`
	SubjectID   *string `json:"subject_id"`
}

// ChoicesView lists a class's fourth subjects and its students' choices.
type ChoicesView struct {
	Optional []SubjectRef    `json:"optional"`
	Students []ChoiceStudent `json:"students"`
	InUse    bool            `json:"in_use"` // any student has a choice: choices decide
	Counts   map[string]int  `json:"counts"`
}

func (r *Repository) Choices(ctx context.Context, sessionID, classID string) (ChoicesView, error) {
	tx := r.db.WithContext(ctx)
	v := ChoicesView{Optional: []SubjectRef{}, Students: []ChoiceStudent{}, Counts: map[string]int{}}
	if err := tx.Raw(`SELECT s.id, s.code, s.name_en, s.name_bn, true AS fourth FROM academic_class_subjects acs JOIN academic_subjects s ON s.id = acs.subject_id
 WHERE acs.class_id = ? AND acs.is_fourth_subject ORDER BY s.code`, classID).Scan(&v.Optional).Error; err != nil {
		return v, err
	}
	if err := tx.Raw(`SELECT st.id AS student_id, st.name_en AS name, st.admission_no, r.roll_no, sec.name AS section_name, ch.subject_id
  FROM student_rolls r JOIN students st ON st.id = r.student_id JOIN academic_sections sec ON sec.id = r.section_id
  LEFT JOIN student_subject_choices ch ON ch.student_id = r.student_id AND ch.session_id = r.session_id
 WHERE r.session_id = ? AND r.class_id = ?
 ORDER BY sec.display_order, sec.name, r.roll_no NULLS LAST, st.admission_no`, sessionID, classID).Scan(&v.Students).Error; err != nil {
		return v, err
	}
	for _, s := range v.Students {
		if s.SubjectID != nil {
			v.InUse = true
			v.Counts[*s.SubjectID]++
		}
	}
	return v, nil
}

// Choice sets (or, with an empty subject, clears) a student's optional subject.
type Choice struct {
	StudentID string `json:"student_id"`
	SubjectID string `json:"subject_id"`
}

func (r *Repository) SaveChoices(ctx context.Context, actor, sessionID, classID string, choices []Choice) (ChoicesView, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed := 0
		var changes []map[string]string // who moved from which subject to which, for the audit
		for _, c := range choices {
			var ok int64
			if err := tx.Raw("SELECT count(*) FROM student_enrolments WHERE student_id = ? AND session_id = ? AND class_id = ?", c.StudentID, sessionID, classID).Row().Scan(&ok); err != nil {
				return err
			}
			if ok == 0 {
				return ErrInvalid
			}
			var before string
			if err := tx.Raw("SELECT coalesce((SELECT subject_id::text FROM student_subject_choices WHERE student_id = ? AND session_id = ? AND role = 'optional'), '')", c.StudentID, sessionID).Row().Scan(&before); err != nil {
				return err
			}
			if before != c.SubjectID {
				changes = append(changes, map[string]string{"student_id": c.StudentID, "from": before, "to": c.SubjectID})
			}
			if c.SubjectID == "" {
				res := tx.Exec("DELETE FROM student_subject_choices WHERE student_id = ? AND session_id = ?", c.StudentID, sessionID)
				if res.Error != nil {
					return res.Error
				}
				changed += int(res.RowsAffected)
				continue
			}
			if err := tx.Raw("SELECT count(*) FROM academic_class_subjects WHERE class_id = ? AND subject_id = ? AND is_fourth_subject", classID, c.SubjectID).Row().Scan(&ok); err != nil {
				return err
			}
			if ok == 0 {
				return ErrInvalid
			}
			res := tx.Exec(`INSERT INTO student_subject_choices (student_id, session_id, subject_id, updated_by) VALUES (?, ?, ?, ?)
ON CONFLICT (student_id, session_id, role) DO UPDATE SET subject_id = EXCLUDED.subject_id, updated_by = EXCLUDED.updated_by, updated_at = now()
WHERE student_subject_choices.subject_id <> EXCLUDED.subject_id`, c.StudentID, sessionID, c.SubjectID, actor)
			if res.Error != nil {
				return dbErr(res.Error)
			}
			changed += int(res.RowsAffected)
		}
		// Published results keep their stored figures; the audit says whose
		// choice changed while published exams of the class exist, so a later
		// check (E6) can tell why a recomputation differs.
		var published int64
		if err := tx.Raw("SELECT count(*) FROM exams WHERE session_id = ? AND class_id = ? AND status = 'published'", sessionID, classID).Row().Scan(&published); err != nil {
			return err
		}
		return audit.Record(tx, &actor, "exam.subject_choices_saved", "class", classID, map[string]any{"session_id": sessionID, "changed": changed, "changes": changes, "published_exams": published})
	})
	if err != nil {
		return ChoicesView{}, dbErr(err)
	}
	return r.Choices(ctx, sessionID, classID)
}
