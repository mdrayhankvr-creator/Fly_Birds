// Package documents renders result documents (E7): student marksheets from
// published result versions, and class documents (tabulation sheet, merit
// list, failed subjects, result summary) for staff. Documents are cached by
// the published versions they come from, so a correction gives a new one.
package documents

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/audit"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/exams"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/files"
	"gorm.io/gorm"
)

// templateVersion changes whenever a template's output changes, so cached
// documents are rebuilt.
const templateVersion = "ms2-1"

var (
	ErrInvalid      = errors.New("invalid document request")
	ErrNotFound     = errors.New("document not found")
	ErrForbidden    = errors.New("not allowed")
	ErrNotPublished = errors.New("results are not published")
	ErrNotReady     = errors.New("results are not ready")
	ErrStale        = errors.New("the results changed while the document was prepared")
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Kinds of documents.
var kinds = map[string]bool{"marksheet": true, "marksheets": true, "tabulation": true, "merit_list": true, "failed_list": true, "summary": true}

// Actor is who asks.
type Actor struct {
	ID   string
	Role string
}

// Request names a document.
type Request struct {
	Kind        string `json:"kind"`
	Scope       string `json:"scope"`
	ScopeID     string `json:"scope_id"`
	ClassID     string `json:"class_id"`
	SectionID   string `json:"section_id"`
	StudentID   string `json:"student_id"`
	Language    string `json:"language"`
	Provisional bool   `json:"provisional"`
}

// Document is a document and its progress.
type Document struct {
	ID            string  `json:"id"`
	Kind          string  `json:"kind"`
	Scope         string  `json:"scope"`
	ScopeID       string  `json:"scope_id"`
	SessionID     string  `json:"session_id"`
	ClassID       string  `json:"class_id"`
	SectionID     *string `json:"section_id"`
	StudentID     *string `json:"student_id"`
	Language      string  `json:"language"`
	Provisional   bool    `json:"provisional"`
	Status        string  `json:"status"`
	ProgressDone  int     `json:"progress_done"`
	ProgressTotal int     `json:"progress_total"`
	Pages         *int    `json:"pages"`
	Bytes         *int64  `json:"bytes"`
	SHA256        *string `json:"sha256"`
	Error         string  `json:"error"`
	CreatedAt     string  `json:"created_at"`
	FinishedAt    *string `json:"finished_at"`
	FileName      string  `json:"file_name"`
	SourceKey     string  `json:"-"`
	StorageKey    *string `json:"-"`
	Attempts      int     `json:"-"`
}

const documentColumns = `id, kind, scope, scope_id, session_id, class_id, section_id, student_id, language, provisional, status, progress_done, progress_total, pages, bytes, sha256, last_error AS error,
 to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at, to_char(finished_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS finished_at,
 source_key, storage_key, attempts`

// Service creates, caches and serves result documents.
type Service struct {
	db       *gorm.DB
	results  *exams.Results
	store    files.Store
	renderer Renderer
	log      *slog.Logger
	// BaseURL is the school's public address for verification QR codes;
	// without it marksheets print the document code only.
	BaseURL string
	// Logo fetches the school's logo (nil: no logo, the initials are used).
	Logo func(ctx context.Context, url string) ([]byte, error)
	// Chunk is how many marksheets are rendered per part of a large
	// document; progress is reported after each part.
	Chunk int
	// Now is the clock (nil: the system clock); attendance counts no day
	// after today.
	Now func() time.Time
	// InlineLimit: marksheet bundles of at most this many students are
	// rendered during the request (about a second); larger ones by the
	// background worker. Class documents are always rendered at once.
	InlineLimit int
}

// NewService returns the documents service. renderer may be nil: requests
// then fail with ErrUnavailable.
func NewService(db *gorm.DB, results *exams.Results, store files.Store, renderer Renderer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, results: results, store: store, renderer: renderer, log: log, Chunk: 20, InlineLimit: 12}
}

// Available reports whether documents can be rendered.
func (s *Service) Available() bool { return s != nil && s.renderer != nil && s.results != nil }

// ---------------------------------------------------------------------------
// Access

func staffAll(a Actor) bool { return a.Role == "admin" || a.Role == "head_teacher" }

// canClass reports whether staff may have a class's documents: the
// administrator and the head teacher always; a teacher for a class they
// teach in that session.
func (s *Service) canClass(ctx context.Context, a Actor, sessionID, classID string) (bool, error) {
	if staffAll(a) {
		return true, nil
	}
	if a.Role != "teacher" || !uuidPattern.MatchString(a.ID) {
		return false, nil
	}
	var n int64
	err := s.db.WithContext(ctx).Raw(`SELECT count(*) FROM teacher_assignments ta JOIN employees emp ON emp.id = ta.employee_id
 WHERE emp.user_id = ? AND emp.is_active AND ta.session_id = ? AND ta.class_id = ?`, a.ID, sessionID, classID).Scan(&n).Error
	return n > 0, err
}

// sessionOf is the session a term, annual or assessment scope belongs to.
func (s *Service) sessionOf(ctx context.Context, scope, scopeID string) (string, error) {
	if scope == exams.ScopeAnnual {
		return scopeID, nil
	}
	table := "exam_terms"
	if scope == exams.ScopeAssessment {
		table = "exams"
	}
	var id string
	err := s.db.WithContext(ctx).Raw("SELECT session_id FROM "+table+" WHERE id = ?", scopeID).Row().Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (r *Request) normalize() error {
	r.Kind, r.Scope, r.Language = strings.TrimSpace(r.Kind), strings.TrimSpace(r.Scope), strings.TrimSpace(r.Language)
	if r.Language == "" {
		r.Language = "en"
	}
	if !kinds[r.Kind] || (r.Scope != exams.ScopeTerm && r.Scope != exams.ScopeAnnual && r.Scope != exams.ScopeAssessment) || (r.Language != "en" && r.Language != "bn") ||
		!uuidPattern.MatchString(r.ScopeID) || !uuidPattern.MatchString(r.ClassID) ||
		(r.SectionID != "" && !uuidPattern.MatchString(r.SectionID)) || (r.StudentID != "" && !uuidPattern.MatchString(r.StudentID)) {
		return ErrInvalid
	}
	// An exam's own published result prints marksheets only; the class
	// documents stay with term and annual results.
	if r.Scope == exams.ScopeAssessment && r.Kind != "marksheet" && r.Kind != "marksheets" {
		return ErrInvalid
	}
	// A provisional preview exists for term and annual results only.
	if r.Scope == exams.ScopeAssessment && r.Provisional {
		return ErrInvalid
	}
	if (r.Kind == "marksheet") != (r.StudentID != "") {
		return ErrInvalid
	}
	if r.Kind == "marksheet" {
		r.SectionID = ""
	}
	return nil
}

// ---------------------------------------------------------------------------
// Requests

// Request returns the document, cached when its source has not changed;
// otherwise it is created (small ones at once, large ones in the background).
func (s *Service) Request(ctx context.Context, a Actor, in Request) (Document, error) {
	if err := in.normalize(); err != nil {
		return Document{}, err
	}
	if !s.Available() {
		return Document{}, ErrUnavailable
	}
	sessionID, err := s.sessionOf(ctx, in.Scope, in.ScopeID)
	if err != nil {
		return Document{}, err
	}
	ok, err := s.canClass(ctx, a, sessionID, in.ClassID)
	if err != nil {
		return Document{}, err
	}
	if !ok || (in.Provisional && !staffAll(a)) {
		return Document{}, ErrForbidden
	}
	return s.request(ctx, a, in, sessionID)
}

// StudentMarksheet is a student's own marksheet of a current published
// version.
func (s *Service) StudentMarksheet(ctx context.Context, a Actor, versionID, language string) (Document, error) {
	if !s.Available() {
		return Document{}, ErrUnavailable
	}
	if a.Role != "student" || !uuidPattern.MatchString(a.ID) {
		return Document{}, ErrForbidden
	}
	ref, err := s.results.DocumentVersion(ctx, versionID)
	if errors.Is(err, exams.ErrNotFound) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	var own int64
	if err := s.db.WithContext(ctx).Raw("SELECT count(*) FROM students WHERE id = ? AND user_id = ?", ref.StudentID, a.ID).Scan(&own).Error; err != nil {
		return Document{}, err
	}
	// Someone else's result, or a superseded or withdrawn one, is not found.
	if own == 0 || ref.Status != "published" {
		return Document{}, ErrNotFound
	}
	// An exam's own result (a class test …) follows its visibility rule:
	// while it is hidden from students, so is its marksheet.
	if ref.Scope == exams.ScopeAssessment {
		visible, err := s.results.StudentsMaySee(ctx, ref.ScopeID)
		if err != nil {
			return Document{}, err
		}
		if !visible {
			return Document{}, ErrNotFound
		}
	}
	in := Request{Kind: "marksheet", Scope: ref.Scope, ScopeID: ref.ScopeID, ClassID: ref.ClassID, StudentID: ref.StudentID, Language: language}
	if err := in.normalize(); err != nil {
		return Document{}, err
	}
	return s.request(ctx, a, in, ref.SessionID)
}

func (s *Service) request(ctx context.Context, a Actor, in Request, sessionID string) (Document, error) {
	set, results, err := s.load(ctx, in)
	if err != nil {
		return Document{}, err
	}
	brand, err := s.branding(ctx, lang(in.Language))
	if err != nil {
		return Document{}, err
	}
	ex, err := s.extrasFor(ctx, in, set, results)
	if err != nil {
		return Document{}, err
	}
	key, err := s.sourceKey(ctx, in, set, results, brand.key+ex.key())
	if err != nil {
		return Document{}, err
	}
	var section, student any
	if in.SectionID != "" {
		section = in.SectionID
	}
	if in.StudentID != "" {
		student = in.StudentID
	}
	var actor any
	if uuidPattern.MatchString(a.ID) {
		actor = a.ID
	}
	var id string
	err = s.db.WithContext(ctx).Raw(`INSERT INTO result_documents (kind, scope, scope_id, session_id, class_id, section_id, student_id, language, provisional, source_key, progress_total, requested_by)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (kind, scope, scope_id, class_id, coalesce(section_id, '00000000-0000-0000-0000-000000000000'::uuid), coalesce(student_id, '00000000-0000-0000-0000-000000000000'::uuid), language, provisional, source_key)
DO UPDATE SET status = CASE WHEN result_documents.status = 'failed' THEN 'queued' ELSE result_documents.status END,
 attempts = CASE WHEN result_documents.status = 'failed' THEN 0 ELSE result_documents.attempts END,
 last_error = CASE WHEN result_documents.status = 'failed' THEN '' ELSE result_documents.last_error END
RETURNING id`, in.Kind, in.Scope, in.ScopeID, sessionID, in.ClassID, section, student, in.Language, in.Provisional, key, progressTotal(in.Kind, len(results)), actor).Row().Scan(&id)
	if err != nil {
		return Document{}, err
	}
	doc, err := s.get(ctx, id)
	if err != nil {
		return Document{}, err
	}
	if doc.Status == "queued" && (in.Kind == "marksheet" || len(results) <= s.InlineLimit || in.Kind != "marksheets") {
		// Small documents are rendered now; the request waits for them.
		if claimed, err := s.claim(ctx, id); err != nil {
			return Document{}, err
		} else if claimed {
			s.process(ctx, id)
			return s.get(ctx, id)
		}
	}
	return doc, nil
}

// extrasFor reads what a marksheet prints besides the results (nothing for
// class documents).
func (s *Service) extrasFor(ctx context.Context, in Request, set exams.DocSet, results []exams.DocResult) (extras, error) {
	if in.Kind != "marksheet" && in.Kind != "marksheets" {
		return extras{}, nil
	}
	return s.loadExtras(ctx, set, results)
}

func progressTotal(kind string, students int) int {
	if kind == "marksheets" {
		return students
	}
	return 1
}

// load reads the results a request shows, narrowed to its section or
// student.
func (s *Service) load(ctx context.Context, in Request) (exams.DocSet, []exams.DocResult, error) {
	ref := exams.ScopeRef{Scope: in.Scope, ScopeID: in.ScopeID, ClassID: in.ClassID}
	var set exams.DocSet
	var err error
	if in.Provisional {
		set, err = s.results.ProvisionalDocSet(ctx, ref)
	} else {
		set, err = s.results.PublishedDocSet(ctx, ref)
	}
	switch {
	case errors.Is(err, exams.ErrNotPublished):
		return set, nil, ErrNotPublished
	case errors.Is(err, exams.ErrNotFound):
		return set, nil, ErrNotFound
	case errors.Is(err, exams.ErrResultsNotReady):
		return set, nil, ErrNotReady
	case errors.Is(err, exams.ErrInvalid):
		return set, nil, ErrInvalid
	case err != nil:
		return set, nil, err
	}
	out := []exams.DocResult{}
	for _, r := range set.Results {
		if in.SectionID != "" && r.Student.SectionID != in.SectionID {
			continue
		}
		if in.StudentID != "" && r.Student.ID != in.StudentID {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return set, nil, ErrNotFound
	}
	return set, out, nil
}

// sourceKey is what a document depends on: the results (all of the class,
// since highest marks and positions come from it), the template, the
// branding and the photos of the students shown.
func (s *Service) sourceKey(ctx context.Context, in Request, set exams.DocSet, results []exams.DocResult, brand string) (string, error) {
	parts := []string{templateVersion, in.Kind, in.Language, fmt.Sprint(in.Provisional), set.Key, in.SectionID, in.StudentID, brand}
	if in.Kind == "marksheet" || in.Kind == "marksheets" {
		ids := make([]string, 0, len(results))
		for _, r := range results {
			ids = append(ids, r.Student.ID)
		}
		var stamps []string
		if err := s.db.WithContext(ctx).Raw("SELECT student_id::text || ':' || extract(epoch FROM updated_at)::text FROM student_photos WHERE student_id IN ? ORDER BY student_id", ids).Scan(&stamps).Error; err != nil {
			return "", err
		}
		parts = append(parts, stamps...)
		if !in.Provisional {
			parts = append(parts, s.BaseURL)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

// Get returns a document if the actor may have it.
func (s *Service) Get(ctx context.Context, a Actor, id string) (Document, error) {
	doc, err := s.get(ctx, id)
	if err != nil {
		return Document{}, err
	}
	if err := s.allowed(ctx, a, doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// Open returns a finished document's file.
func (s *Service) Open(ctx context.Context, a Actor, id string) (Document, io.ReadCloser, error) {
	doc, err := s.Get(ctx, a, id)
	if err != nil {
		return Document{}, nil, err
	}
	if doc.Status != "done" || doc.StorageKey == nil {
		return Document{}, nil, ErrNotReady
	}
	rc, err := s.store.Open(ctx, *doc.StorageKey)
	if errors.Is(err, files.ErrNotFound) {
		return Document{}, nil, ErrNotFound
	}
	return doc, rc, err
}

func (s *Service) allowed(ctx context.Context, a Actor, doc Document) error {
	if a.Role == "student" {
		var own int64
		if doc.StudentID == nil || doc.Provisional {
			return ErrNotFound
		}
		if err := s.db.WithContext(ctx).Raw("SELECT count(*) FROM students WHERE id = ? AND user_id = ?", *doc.StudentID, a.ID).Scan(&own).Error; err != nil {
			return err
		}
		if own == 0 {
			return ErrNotFound
		}
		return nil
	}
	ok, err := s.canClass(ctx, a, doc.SessionID, doc.ClassID)
	if err != nil {
		return err
	}
	if !ok || (doc.Provisional && !staffAll(a)) {
		return ErrNotFound
	}
	return nil
}

func (s *Service) get(ctx context.Context, id string) (Document, error) {
	if !uuidPattern.MatchString(id) {
		return Document{}, ErrNotFound
	}
	var d Document
	err := s.db.WithContext(ctx).Raw("SELECT "+documentColumns+" FROM result_documents WHERE id = ?", id).Take(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Document{}, ErrNotFound
	}
	if err != nil {
		return Document{}, err
	}
	d.FileName = fileName(d)
	return d, nil
}

func fileName(d Document) string {
	name := strings.ReplaceAll(d.Kind, "_", "-")
	if d.Provisional {
		name += "-provisional"
	}
	return fmt.Sprintf("%s-%s-%s.pdf", name, d.ID[:8], d.Language)
}

// ---------------------------------------------------------------------------
// Rendering and the worker

// claim marks a queued document as running; false when someone else did.
func (s *Service) claim(ctx context.Context, id string) (bool, error) {
	res := s.db.WithContext(ctx).Exec("UPDATE result_documents SET status = 'running', started_at = now(), attempts = attempts + 1 WHERE id = ? AND status = 'queued'", id)
	return res.RowsAffected == 1, res.Error
}

// Work renders queued documents until there are none; it returns how many
// it processed.
func (s *Service) Work(ctx context.Context) (int, error) {
	if !s.Available() {
		return 0, nil
	}
	// A document left running by a stopped server is queued again.
	if err := s.db.WithContext(ctx).Exec("UPDATE result_documents SET status = 'queued' WHERE status = 'running' AND started_at < now() - interval '10 minutes' AND attempts < 3").Error; err != nil {
		return 0, err
	}
	n := 0
	for ctx.Err() == nil {
		var id string
		err := s.db.WithContext(ctx).Raw(`UPDATE result_documents SET status = 'running', started_at = now(), attempts = attempts + 1
 WHERE id = (SELECT id FROM result_documents WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id`).Row().Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		s.process(ctx, id)
		n++
	}
	return n, ctx.Err()
}

// Run works the queue every interval until ctx ends.
func (s *Service) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := s.Work(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("result documents", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// process renders a claimed document and records the outcome.
func (s *Service) process(ctx context.Context, id string) {
	doc, err := s.get(ctx, id)
	if err != nil {
		return
	}
	pdf, err := s.render(ctx, doc)
	if err != nil {
		s.log.Warn("result document failed", "id", id, "kind", doc.Kind, "error", err)
		msg := err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		// Three attempts, then it stays failed until someone asks again.
		_ = s.db.WithContext(ctx).Exec("UPDATE result_documents SET status = CASE WHEN attempts >= 3 OR ? THEN 'failed' ELSE 'queued' END, last_error = ?, finished_at = now() WHERE id = ?",
			errors.Is(err, ErrStale), msg, id).Error
		return
	}
	key, err := files.NewKey()
	if err == nil {
		err = s.store.Put(ctx, key, pdf)
	}
	if err != nil {
		_ = s.db.WithContext(ctx).Exec("UPDATE result_documents SET status = 'failed', last_error = ?, finished_at = now() WHERE id = ?", "storage: "+err.Error(), id).Error
		return
	}
	sum := sha256.Sum256(pdf)
	if err := s.db.WithContext(ctx).Exec(`UPDATE result_documents SET status = 'done', storage_key = ?, bytes = ?, pages = ?, sha256 = ?, progress_done = progress_total, last_error = '', finished_at = now() WHERE id = ?`,
		key, len(pdf), Pages(pdf), hex.EncodeToString(sum[:]), id).Error; err != nil {
		_ = s.store.Delete(ctx, key)
		return
	}
	s.prune(ctx, doc)
	_ = audit.Record(s.db.WithContext(ctx), nil, "result.document_created", "result_document", id, map[string]any{"kind": doc.Kind, "scope": doc.Scope, "language": doc.Language, "provisional": doc.Provisional, "pages": Pages(pdf)})
}

// prune deletes older versions of the same document (other source keys),
// keeping only the newest finished one.
func (s *Service) prune(ctx context.Context, doc Document) {
	var old []struct {
		ID         string
		StorageKey *string
	}
	err := s.db.WithContext(ctx).Raw(`SELECT id, storage_key FROM result_documents
 WHERE kind = ? AND scope = ? AND scope_id = ? AND class_id = ? AND section_id IS NOT DISTINCT FROM ? AND student_id IS NOT DISTINCT FROM ? AND language = ? AND provisional = ?
 AND id <> ? AND status IN ('done', 'failed')`, doc.Kind, doc.Scope, doc.ScopeID, doc.ClassID, doc.SectionID, doc.StudentID, doc.Language, doc.Provisional, doc.ID).Scan(&old).Error
	if err != nil {
		return
	}
	for _, o := range old {
		if err := s.db.WithContext(ctx).Exec("DELETE FROM result_documents WHERE id = ?", o.ID).Error; err != nil {
			continue
		}
		if o.StorageKey != nil {
			_ = s.store.Delete(ctx, *o.StorageKey)
		}
	}
}

func (s *Service) setProgress(ctx context.Context, id string, done int) {
	_ = s.db.WithContext(ctx).Exec("UPDATE result_documents SET progress_done = ? WHERE id = ?", done, id).Error
}

// render builds a document from the current results; if they changed since
// it was requested, it fails as stale (the next request makes a new one).
func (s *Service) render(ctx context.Context, doc Document) ([]byte, error) {
	in := Request{Kind: doc.Kind, Scope: doc.Scope, ScopeID: doc.ScopeID, ClassID: doc.ClassID, Language: doc.Language, Provisional: doc.Provisional}
	if doc.SectionID != nil {
		in.SectionID = *doc.SectionID
	}
	if doc.StudentID != nil {
		in.StudentID = *doc.StudentID
	}
	l := lang(doc.Language)
	set, results, err := s.load(ctx, in)
	if err != nil {
		return nil, err
	}
	brand, err := s.branding(ctx, l)
	if err != nil {
		return nil, err
	}
	ex, err := s.extrasFor(ctx, in, set, results)
	if err != nil {
		return nil, err
	}
	key, err := s.sourceKey(ctx, in, set, results, brand.key+ex.key())
	if err != nil {
		return nil, err
	}
	if key != doc.SourceKey {
		return nil, ErrStale
	}
	assets := map[string][]byte{}
	brand.assets(assets)
	created := time.Now()
	if t, err := time.Parse(time.RFC3339, set.PublishedAt); err == nil && !set.Provisional {
		created = t
	}
	section := ""
	if in.SectionID != "" && len(results) > 0 {
		section = results[0].Student.SectionName
	}
	if doc.Kind != "marksheet" && doc.Kind != "marksheets" {
		data := s.classData(l, doc.Kind, set, results, section, brand.school)
		return s.renderer.Render(ctx, Render{Template: "class.typ", Data: data, Assets: assets, Created: created})
	}
	ordered := byRoll(results)
	chunk := s.Chunk
	if chunk < 1 {
		chunk = 20
	}
	if len(ordered) <= chunk {
		data, err := s.sheetData(ctx, l, set, ordered, brand, assets, ex)
		if err != nil {
			return nil, err
		}
		return s.renderer.Render(ctx, Render{Template: "marksheet.typ", Data: data, Assets: assets, Created: created})
	}
	// Large documents: render parts, report progress, then join them.
	type part struct {
		File  string `json:"file"`
		Pages int    `json:"pages"`
	}
	parts := []part{}
	partFiles := map[string][]byte{}
	for start := 0; start < len(ordered); start += chunk {
		end := min(start+chunk, len(ordered))
		partAssets := map[string][]byte{}
		brand.assets(partAssets)
		data, err := s.sheetData(ctx, l, set, ordered[start:end], brand, partAssets, ex)
		if err != nil {
			return nil, err
		}
		pdf, err := s.renderer.Render(ctx, Render{Template: "marksheet.typ", Data: data, Assets: partAssets, Created: created})
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf("part-%03d.pdf", len(parts))
		parts = append(parts, part{File: name, Pages: Pages(pdf)})
		partFiles[name] = pdf
		s.setProgress(ctx, doc.ID, end)
	}
	title := l.t("marksheet") + " — " + set.ClassName
	return s.renderer.Render(ctx, Render{Template: "merge.typ", Data: map[string]any{"title": title, "paper": "a4", "flipped": false, "parts": parts}, Assets: partFiles, Created: created})
}

// sheetData builds marksheet data for some of a set's results; images go
// into assets.
func (s *Service) sheetData(ctx context.Context, l lang, set exams.DocSet, results []exams.DocResult, brand branding, assets map[string][]byte, ex extras) (sheetData, error) {
	lo := layoutOf(set)
	top := highest(set)
	d := sheetData{Lang: string(l), Provisional: set.Provisional, Labels: labelsFor(l), School: brand.school, Columns: lo.columns(l), ShowCT: lo.showCT && set.Scope == exams.ScopeTerm, Grading: lo.grading(l)}
	d.Heading, d.HeadingAlt, d.ExamName, d.SessionName = l.t("marksheet"), l.other(labelsEN["marksheet"], labelsBN["marksheet"]), examName(l, set), l.num(set.SessionName)
	if l == "bn" {
		d.HeadingAlt = labelsEN["marksheet"]
	}
	d.Title = d.Heading + " — " + set.ClassName
	if d.Columns == nil {
		d.Columns = []string{}
	}
	// Positions are out of the class (and the section) as published.
	classN, sectionN := len(set.Results), map[string]int{}
	for _, r := range set.Results {
		sectionN[r.Student.SectionID]++
	}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.Student.ID)
	}
	photos, err := s.photos(ctx, ids)
	if err != nil {
		return d, err
	}
	for i, r := range results {
		st := r.Student
		o := r.Outputs
		x := ex.students[st.ID]
		if x == nil {
			x = &studentExtra{}
		}
		row := sheetStudent{Name: l.pick(st.NameEN, st.NameBN), NameAlt: l.other(st.NameEN, st.NameBN), StudentID: l.code(studentCode(x.Code, st.AdmissionNo)), Class: st.ClassName, Section: st.SectionName,
			Roll: rollText(l, st.RollNo), Session: l.num(st.SessionName), Exam: examName(l, set), Withheld: r.Outcome == "withheld", Rows: []sheetRow{}, Optional: []sheetRow{},
			Total: "—", GPA: "—", GPAWithout: "—", Letter: "—", Result: outcomeText(l, r.Outcome), Failed: r.Outcome == "failed", FailedCount: "—",
			ClassPos: posText(l, o.ClassPosition, classN), SectionPos: posText(l, o.SectionPosition, sectionN[st.SectionID]), Attendance: "—", Published: "—"}
		row.Father, row.Mother = orDash(l.pick(x.FatherEN, x.FatherBN)), orDash(l.pick(x.MotherEN, x.MotherBN))
		row.DOB = "—"
		if x.DOB != nil {
			row.DOB, _ = l.day(*x.DOB)
		}
		if x.Working > 0 && x.HasAttendance {
			row.Attendance = fmt.Sprintf(l.t("attendance_value"), l.int(x.Present), l.int(x.Working), l.int(x.Present*100/x.Working))
		}
		switch {
		case x.Custom != "":
			row.Comment = x.Custom
		case x.PresetEN != "":
			row.Comment = l.pick(x.PresetEN, x.PresetBN)
		}
		if p, ok := photos[st.ID]; ok {
			name := fmt.Sprintf("photo-%d.%s", i, p.ext)
			assets[name] = p.data
			row.Photo = name
		}
		if !row.Withheld {
			row.Rows, row.Optional = lo.rows(l, r, top, ex.subjects)
			if o.Total != nil && o.Full != nil {
				row.Total = l.marks(o.Total) + " / " + l.marks(o.Full)
			}
			row.GPA = l.points(o.GPA)
			row.GPAWithout = l.points(lo.requiredGPA(o))
			row.Letter = lo.letter(o)
			failed := 0
			for _, line := range o.Subjects {
				if !line.Optional && line.Passed != nil && !*line.Passed {
					failed++
				}
			}
			row.FailedCount = l.int(failed)
		}
		// Signatures, placed automatically: the class teacher's, the head
		// teacher's (ID-card settings) and a blank one for the guardian.
		teacher := signature{Title: l.t("sig_class_teacher"), Name: l.pick(x.TeacherEN, x.TeacherBN)}
		if len(x.Signature) > 0 {
			name := "ctsig-" + x.SignatureVersion + ".png"
			assets[name] = x.Signature
			teacher.Image = name
		}
		head := signature{Title: l.t("sig_head"), Name: l.pick(ex.headEN, ex.headBN), Image: brand.school.Signature}
		if brand.school.Signatory != "" {
			head.Title = brand.school.Signatory
		}
		row.Signatures = []signature{teacher, head, {Title: l.t("sig_guardian"), Date: true}}
		if set.Provisional {
			row.Footer = l.t("footer_provisional")
		} else {
			row.Published = l.date(r.PublishedAt)
			token, err := s.token(ctx, r.VersionID)
			if err != nil {
				return d, err
			}
			code := documentCode(token)
			version := l.int(r.Version)
			if s.BaseURL != "" {
				png, err := qrPNG(strings.TrimRight(s.BaseURL, "/") + "/verify/result/" + token)
				if err != nil {
					return d, err
				}
				name := fmt.Sprintf("qr-%d.png", i)
				assets[name] = png
				row.QR = name
				row.Footer = fmt.Sprintf(l.t("footer"), version, row.Published, code)
			} else {
				row.Footer = fmt.Sprintf(l.t("footer_noqr"), version, row.Published, code)
			}
		}
		d.Students = append(d.Students, row)
	}
	return d, nil
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

type photo struct {
	data []byte
	ext  string
}

func (s *Service) photos(ctx context.Context, ids []string) (map[string]photo, error) {
	out := map[string]photo{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		StudentID   string
		ContentType string
		Content     []byte
	}
	if err := s.db.WithContext(ctx).Raw("SELECT student_id, content_type, content FROM student_photos WHERE student_id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		switch r.ContentType {
		case "image/jpeg":
			out[r.StudentID] = photo{r.Content, "jpg"}
		case "image/png":
			out[r.StudentID] = photo{r.Content, "png"}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Classes with documents

// ClassSet is a class's published term or annual results, for the list of
// documents a member of staff may create.
type ClassSet struct {
	Scope       string    `json:"scope"`
	ScopeID     string    `json:"scope_id"`
	SessionID   string    `json:"session_id"`
	SessionName string    `json:"session_name"`
	ClassID     string    `json:"class_id"`
	ClassName   string    `json:"class_name"`
	Title       string    `json:"title"`
	Revision    int       `json:"revision"`
	PublishedAt string    `json:"published_at"`
	Students    int       `json:"students"`
	Sections    []Section `json:"sections" gorm:"-"`
}

// Section is one section with published results.
type Section struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Classes lists the published term and annual results the actor may have
// documents of, newest first.
func (s *Service) Classes(ctx context.Context, a Actor) ([]ClassSet, error) {
	if !staffAll(a) && a.Role != "teacher" {
		return nil, ErrForbidden
	}
	rows := []ClassSet{}
	q := `SELECT * FROM (SELECT DISTINCT ON (p.scope, p.scope_id, p.class_id) p.scope, p.scope_id, p.session_id, s.name AS session_name, p.class_id, c.name AS class_name,
 coalesce(t.name, e.name, s.name) AS title, p.revision, to_char(p.published_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS published_at,
 (SELECT count(*) FROM result_versions v WHERE v.scope = p.scope AND v.scope_id = p.scope_id AND v.class_id = p.class_id AND v.status = 'published') AS students
 FROM result_publications p JOIN academic_sessions s ON s.id = p.session_id JOIN academic_classes c ON c.id = p.class_id
 LEFT JOIN exam_terms t ON p.scope = 'term' AND t.id = p.scope_id
 LEFT JOIN exams e ON p.scope = 'assessment' AND e.id = p.scope_id
 WHERE p.status = 'published' AND p.scope IN ('term', 'annual', 'assessment')`
	args := []any{}
	if !staffAll(a) {
		q += ` AND EXISTS (SELECT 1 FROM teacher_assignments ta JOIN employees emp ON emp.id = ta.employee_id WHERE emp.user_id = ? AND emp.is_active AND ta.session_id = p.session_id AND ta.class_id = p.class_id)`
		args = append(args, a.ID)
	}
	q += ` ORDER BY p.scope, p.scope_id, p.class_id, p.revision DESC) x WHERE students > 0 ORDER BY published_at DESC, class_name LIMIT 200`
	if err := s.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Sections = []Section{}
		if err := s.db.WithContext(ctx).Raw(`SELECT DISTINCT sec.id, sec.name FROM result_versions v JOIN academic_sections sec ON sec.id = v.section_id
 WHERE v.scope = ? AND v.scope_id = ? AND v.class_id = ? AND v.status = 'published' ORDER BY sec.name`, rows[i].Scope, rows[i].ScopeID, rows[i].ClassID).Scan(&rows[i].Sections).Error; err != nil {
			return nil, err
		}
	}
	return rows, nil
}
