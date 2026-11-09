package routine

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/documents"
	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/routine/planner"
)

// Term exam routines (E8): one routine per exam term, kept as versions.
// draft → submitted → approved (head teacher) → published (admin); an
// amendment of a published routine is a new version with a reason, and
// publishing it supersedes the old one, which is kept.

var (
	// ErrState: the version is not in a state that allows the action.
	ErrState = errors.New("routine version state does not allow this")
	// ErrDocuments: routine PDFs need the document renderer.
	ErrDocuments = errors.New("routine documents unavailable")
)

// IssuesError carries the rules a change or a step breaks.
type IssuesError struct {
	Code   string          // routine_conflict (a placement) or routine_not_ready (a step)
	Issues []planner.Issue // the hard issues
}

func (e *IssuesError) Error() string        { return e.Code }
func (e *IssuesError) Is(target error) bool { return target == ErrConflict }

type Room struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NameBN   string `json:"name_bn"`
	Kind     string `json:"kind"`
	Capacity *int   `json:"capacity"`
	IsActive bool   `json:"is_active"`
}
type RoomInput struct {
	Name     string `json:"name"`
	NameBN   string `json:"name_bn"`
	Kind     string `json:"kind"`
	Capacity *int   `json:"capacity"`
	IsActive *bool  `json:"is_active"`
}

// TermSummary is an exam term with its routine versions.
type TermSummary struct {
	TermID      string           `json:"term_id"`
	TermName    string           `json:"term_name"`
	SessionID   string           `json:"session_id"`
	SessionName string           `json:"session_name"`
	StartsOn    *string          `json:"starts_on"`
	EndsOn      *string          `json:"ends_on"`
	Versions    []VersionSummary `json:"versions" gorm:"-"`
}

type VersionSummary struct {
	ID             string  `json:"id"`
	TermID         string  `json:"term_id"`
	Version        int     `json:"version"`
	Status         string  `json:"status"`
	Reason         *string `json:"reason"`
	ReturnedReason *string `json:"returned_reason"`
	StartsOn       string  `json:"starts_on"`
	EndsOn         string  `json:"ends_on"`
	CreatedAt      string  `json:"created_at"`
	SubmittedAt    *string `json:"submitted_at"`
	ApprovedAt     *string `json:"approved_at"`
	ApprovedByName *string `json:"approved_by_name"`
	PublishedAt    *string `json:"published_at"`
	SupersededAt   *string `json:"superseded_at"`
	ContentSHA256  *string `json:"content_sha256"`
	Papers         int     `json:"papers"`
	Placed         int     `json:"placed"`
}

type Sitting struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	NameBN    string `json:"name_bn"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Minutes   int    `json:"minutes"`
	Start     int    `json:"-"`
	End       int    `json:"-"`
}
type SittingInput struct {
	Name      string `json:"name"`
	NameBN    string `json:"name_bn"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type PaperRoom struct {
	SectionID   string `json:"section_id"`
	SectionName string `json:"section_name"`
	RoomID      string `json:"room_id"`
	RoomName    string `json:"room_name"`
	RoomNameBN  string `json:"room_name_bn"`
	Students    int    `json:"students"`
}

type Paper struct {
	ID            string      `json:"id"`
	ClassID       string      `json:"class_id"`
	ClassName     string      `json:"class_name"`
	ClassOrder    int         `json:"-"`
	SubjectID     string      `json:"subject_id"`
	SubjectCode   string      `json:"subject_code"`
	SubjectName   string      `json:"subject_name"`
	SubjectNameBN string      `json:"subject_name_bn"`
	Kind          string      `json:"kind"`
	PaperNo       int         `json:"paper_no"`
	Duration      int         `json:"duration_minutes"`
	NeedsLab      bool        `json:"needs_lab"`
	Optional      bool        `json:"optional"`
	Pinned        bool        `json:"pinned"`
	ExamDate      *string     `json:"exam_date"`
	SittingID     *string     `json:"sitting_id"`
	Rooms         []PaperRoom `json:"rooms" gorm:"-"`
	Students      int         `json:"students" gorm:"-"`
}
type PaperInput struct {
	ClassID   string `json:"class_id"`
	SubjectID string `json:"subject_id"`
	Kind      string `json:"kind"`
	PaperNo   int    `json:"paper_no"`
	Duration  int    `json:"duration_minutes"`
	NeedsLab  bool   `json:"needs_lab"`
}

// Placement is a paper's date, sitting and rooms, set by hand.
type Placement struct {
	Duration  int              `json:"duration_minutes"`
	NeedsLab  bool             `json:"needs_lab"`
	ExamDate  string           `json:"exam_date"`  // "" leaves it unplaced
	SittingID string           `json:"sitting_id"` // with exam_date
	Pinned    *bool            `json:"pinned"`     // default: pinned when placed
	Rooms     []RoomAssignment `json:"rooms"`
}
type RoomAssignment struct {
	SectionID string `json:"section_id"`
	RoomID    string `json:"room_id"`
}

type Duty struct {
	ID             string `json:"id"`
	ExamDate       string `json:"exam_date"`
	SittingID      string `json:"sitting_id"`
	RoomID         string `json:"room_id"`
	RoomName       string `json:"room_name"`
	RoomNameBN     string `json:"room_name_bn"`
	EmployeeID     string `json:"employee_id"`
	EmployeeName   string `json:"employee_name"`
	EmployeeNameBN string `json:"employee_name_bn"`
}
type DutyInput struct {
	ExamDate   string `json:"exam_date"`
	SittingID  string `json:"sitting_id"`
	RoomID     string `json:"room_id"`
	EmployeeID string `json:"employee_id"`
}

type PlanSection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Students int    `json:"students"`
}
type PlanClass struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Sections []PlanSection `json:"sections"`
}
type Holiday struct {
	Date    string `json:"date"`
	TitleEN string `json:"title_en"`
	TitleBN string `json:"title_bn"`
}
type Teacher struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	NameBN string `json:"name_bn"`
}

// Change is one line of an amendment's difference from the version it
// amends.
type Change struct {
	Change        string `json:"change"` // added, removed, moved, rooms, duration
	ClassName     string `json:"class_name"`
	SubjectName   string `json:"subject_name"`
	SubjectNameBN string `json:"subject_name_bn"`
	Kind          string `json:"kind"`
	PaperNo       int    `json:"paper_no"`
	FromDate      string `json:"from_date,omitempty"`
	FromSitting   string `json:"from_sitting,omitempty"`
	ToDate        string `json:"to_date,omitempty"`
	ToSitting     string `json:"to_sitting,omitempty"`
	FromRooms     string `json:"from_rooms,omitempty"`
	ToRooms       string `json:"to_rooms,omitempty"`
	FromDuration  int    `json:"from_duration,omitempty"`
	ToDuration    int    `json:"to_duration,omitempty"`
	key           string
}

// Version is a routine version with everything the planner shows.
type Version struct {
	VersionSummary
	TermName      string          `json:"term_name"`
	SessionID     string          `json:"session_id"`
	SessionName   string          `json:"session_name"`
	TermStartsOn  *string         `json:"term_starts_on"`
	TermEndsOn    *string         `json:"term_ends_on"`
	BasedOn       *string         `json:"based_on"`
	Instructions  string          `json:"instructions"`
	From          string          `json:"from"` // the routine's dates within the term
	To            string          `json:"to"`
	Sittings      []Sitting       `json:"sittings"`
	Papers        []Paper         `json:"papers"`
	Duties        []Duty          `json:"duties"`
	Classes       []PlanClass     `json:"classes"`
	Rooms         []Room          `json:"rooms"`
	Teachers      []Teacher       `json:"teachers"`
	OffDays       []int           `json:"weekly_off_days"`
	Holidays      []Holiday       `json:"holidays"`
	Dates         []string        `json:"dates"` // open exam days
	Issues        []planner.Issue `json:"issues"`
	Changes       []Change        `json:"changes"`
	PreviousLabel *int            `json:"previous_version"`
}

type VersionInput struct {
	TermID       string `json:"term_id"`
	StartsOn     string `json:"starts_on"`
	EndsOn       string `json:"ends_on"`
	Instructions string `json:"instructions"`
	Duration     int    `json:"duration_minutes"` // the first papers' length, default 180
}

type GenerateResult struct {
	Report  planner.Report `json:"report"`
	Version Version        `json:"version"`
}

// StudentPaper is a paper on a student's own routine.
type StudentPaper struct {
	PaperID       string `json:"paper_id"`
	ExamDate      string `json:"exam_date"`
	StartTime     string `json:"start_time"`
	EndTime       string `json:"end_time"`
	SittingName   string `json:"sitting_name"`
	SittingNameBN string `json:"sitting_name_bn"`
	SubjectCode   string `json:"subject_code"`
	SubjectName   string `json:"subject_name"`
	SubjectNameBN string `json:"subject_name_bn"`
	Kind          string `json:"kind"`
	PaperNo       int    `json:"paper_no"`
	Duration      int    `json:"duration_minutes"`
	Optional      bool   `json:"optional"`
	RoomName      string `json:"room_name"`
	RoomNameBN    string `json:"room_name_bn"`
	Changed       bool   `json:"changed"`
	ClassID       string `json:"-"`
	SubjectID     string `json:"-"`
	SittingID     string `json:"-"`
	RoomID        string `json:"-"`
}

// StudentRoutine is a published term routine as one student sees it.
type StudentRoutine struct {
	VersionID   string         `json:"version_id"`
	TermID      string         `json:"term_id"`
	TermName    string         `json:"term_name"`
	Version     int            `json:"version"`
	PublishedAt string         `json:"published_at"`
	AmendedOn   *string        `json:"amended_on"`
	Reason      *string        `json:"reason"`
	ClassName   string         `json:"class_name"`
	SectionName string         `json:"section_name"`
	Papers      []StudentPaper `json:"papers" gorm:"-"`
}

// TeacherDuty is one invigilation duty in a published routine.
type TeacherDuty struct {
	VersionID     string   `json:"version_id"`
	TermName      string   `json:"term_name"`
	ExamDate      string   `json:"exam_date"`
	StartTime     string   `json:"start_time"`
	EndTime       string   `json:"end_time"`
	SittingName   string   `json:"sitting_name"`
	SittingNameBN string   `json:"sitting_name_bn"`
	RoomName      string   `json:"room_name"`
	RoomNameBN    string   `json:"room_name_bn"`
	Papers        []string `json:"papers" gorm:"-"`
	PapersBN      []string `json:"papers_bn" gorm:"-"`
	Students      int      `json:"students" gorm:"-"`
	RoomID        string   `json:"-"`
	SittingID     string   `json:"-"`
}

// DocumentRequest names a routine PDF.
type DocumentRequest struct {
	Kind      string // class, room, invigilation
	Language  string
	ClassID   string // optional: one class
	SectionID string // optional: one section (students)
	RoomID    string // optional: one room
}

// TermRepository stores term routines.
type TermRepository interface {
	ListRooms(context.Context, bool) ([]Room, error)
	SaveRoom(context.Context, string, string, RoomInput) (Room, error)
	ListTerms(context.Context, string) ([]TermSummary, error)
	CreateVersion(context.Context, string, VersionInput) (Version, error)
	GetVersion(context.Context, string) (Version, error)
	UpdateVersion(context.Context, string, string, VersionInput) (Version, error)
	DeleteVersion(context.Context, string, string) error
	SaveSitting(context.Context, string, string, string, SittingInput, int, int) (Version, error)
	DeleteSitting(context.Context, string, string, string) (Version, error)
	AddPaper(context.Context, string, string, PaperInput) (Version, error)
	PlacePaper(context.Context, string, string, string, Placement) (Version, error)
	DeletePaper(context.Context, string, string, string) (Version, error)
	AddDuty(context.Context, string, string, DutyInput) (Version, error)
	DeleteDuty(context.Context, string, string, string) (Version, error)
	Generate(context.Context, string, string) (GenerateResult, error)
	Move(context.Context, string, string, string, string) (Version, error)
	Amend(context.Context, string, string, string) (Version, error)
	StudentRoutines(context.Context, string) ([]StudentRoutine, string, string, error)
	TeacherDuties(context.Context, string) ([]TeacherDuty, error)
	Document(context.Context, string, DocumentRequest) (documents.RoutineDoc, error)
	StudentDocument(context.Context, string, string, string) (documents.RoutineDoc, error)
}

// RoutineRenderer renders routine PDFs (the documents service).
type RoutineRenderer interface {
	RenderRoutine(context.Context, documents.RoutineDoc) ([]byte, error)
	RoutineAvailable() bool
}

type TermService struct {
	repo TermRepository
	// Documents renders PDFs; nil turns the PDF routes off.
	Documents RoutineRenderer
}

func NewTermService(repo TermRepository) *TermService { return &TermService{repo: repo} }

func validDay(s string) bool {
	d, err := time.Parse(time.DateOnly, s)
	return err == nil && d.Format(time.DateOnly) == s
}
func text(v string, max int) (string, bool) {
	v = strings.TrimSpace(v)
	return v, utf8.ValidString(v) && len(v) <= max
}
func ids(xs ...string) bool {
	for _, x := range xs {
		if !uuid.MatchString(x) {
			return false
		}
	}
	return true
}

func (s *TermService) ListRooms(ctx context.Context, all bool) ([]Room, error) {
	return s.repo.ListRooms(ctx, all)
}
func (s *TermService) SaveRoom(ctx context.Context, actor, id string, in RoomInput) (Room, error) {
	var ok1, ok2 bool
	in.Name, ok1 = text(in.Name, 80)
	in.NameBN, ok2 = text(in.NameBN, 240)
	if in.Kind == "" {
		in.Kind = "classroom"
	}
	if !ids(actor) || (id != "" && !ids(id)) || !ok1 || !ok2 || in.Name == "" || (in.Kind != "classroom" && in.Kind != "lab" && in.Kind != "hall") || (in.Capacity != nil && (*in.Capacity < 1 || *in.Capacity > 5000)) {
		return Room{}, ErrInvalid
	}
	return s.repo.SaveRoom(ctx, actor, id, in)
}
func (s *TermService) ListTerms(ctx context.Context, sessionID string) ([]TermSummary, error) {
	if sessionID != "" && !ids(sessionID) {
		return nil, ErrInvalid
	}
	return s.repo.ListTerms(ctx, sessionID)
}
func (s *TermService) validVersion(in *VersionInput) bool {
	var ok bool
	in.Instructions, ok = text(in.Instructions, 4000)
	if in.Duration == 0 {
		in.Duration = 180
	}
	return ok && (in.StartsOn == "" || validDay(in.StartsOn)) && (in.EndsOn == "" || validDay(in.EndsOn)) && in.Duration >= 15 && in.Duration <= 360
}
func (s *TermService) CreateVersion(ctx context.Context, actor string, in VersionInput) (Version, error) {
	if !ids(actor, in.TermID) || !s.validVersion(&in) {
		return Version{}, ErrInvalid
	}
	return s.repo.CreateVersion(ctx, actor, in)
}
func (s *TermService) GetVersion(ctx context.Context, id string) (Version, error) {
	if !ids(id) {
		return Version{}, ErrInvalid
	}
	return s.repo.GetVersion(ctx, id)
}
func (s *TermService) UpdateVersion(ctx context.Context, actor, id string, in VersionInput) (Version, error) {
	if !ids(actor, id) || !s.validVersion(&in) || in.StartsOn == "" || in.EndsOn == "" {
		return Version{}, ErrInvalid
	}
	return s.repo.UpdateVersion(ctx, actor, id, in)
}
func (s *TermService) DeleteVersion(ctx context.Context, actor, id string) error {
	if !ids(actor, id) {
		return ErrInvalid
	}
	return s.repo.DeleteVersion(ctx, actor, id)
}
func (s *TermService) SaveSitting(ctx context.Context, actor, versionID, id string, in SittingInput) (Version, error) {
	a, b, ok := times(in.StartTime, in.EndTime)
	var ok1, ok2 bool
	in.Name, ok1 = text(in.Name, 60)
	in.NameBN, ok2 = text(in.NameBN, 180)
	if !ids(actor, versionID) || (id != "" && !ids(id)) || !ok || !ok1 || !ok2 || in.Name == "" || b-a < 15 {
		return Version{}, ErrInvalid
	}
	return s.repo.SaveSitting(ctx, actor, versionID, id, in, a, b)
}
func (s *TermService) DeleteSitting(ctx context.Context, actor, versionID, id string) (Version, error) {
	if !ids(actor, versionID, id) {
		return Version{}, ErrInvalid
	}
	return s.repo.DeleteSitting(ctx, actor, versionID, id)
}

var paperKinds = map[string]bool{"theory": true, "practical": true, "viva": true, "oral": true}

func (s *TermService) AddPaper(ctx context.Context, actor, versionID string, in PaperInput) (Version, error) {
	if in.Kind == "" {
		in.Kind = "theory"
	}
	if in.PaperNo == 0 {
		in.PaperNo = 1
	}
	if !ids(actor, versionID, in.ClassID, in.SubjectID) || !paperKinds[in.Kind] || in.PaperNo < 1 || in.PaperNo > 3 || in.Duration < 15 || in.Duration > 360 {
		return Version{}, ErrInvalid
	}
	return s.repo.AddPaper(ctx, actor, versionID, in)
}
func (s *TermService) PlacePaper(ctx context.Context, actor, versionID, id string, in Placement) (Version, error) {
	if !ids(actor, versionID, id) || in.Duration < 15 || in.Duration > 360 || (in.ExamDate == "") != (in.SittingID == "") || (in.ExamDate != "" && (!validDay(in.ExamDate) || !ids(in.SittingID))) || len(in.Rooms) > 50 {
		return Version{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range in.Rooms {
		if !ids(r.SectionID, r.RoomID) || seen[r.SectionID] {
			return Version{}, ErrInvalid
		}
		seen[r.SectionID] = true
	}
	if in.ExamDate == "" && len(in.Rooms) > 0 {
		return Version{}, ErrInvalid
	}
	return s.repo.PlacePaper(ctx, actor, versionID, id, in)
}
func (s *TermService) DeletePaper(ctx context.Context, actor, versionID, id string) (Version, error) {
	if !ids(actor, versionID, id) {
		return Version{}, ErrInvalid
	}
	return s.repo.DeletePaper(ctx, actor, versionID, id)
}
func (s *TermService) AddDuty(ctx context.Context, actor, versionID string, in DutyInput) (Version, error) {
	if !ids(actor, versionID, in.SittingID, in.RoomID, in.EmployeeID) || !validDay(in.ExamDate) {
		return Version{}, ErrInvalid
	}
	return s.repo.AddDuty(ctx, actor, versionID, in)
}
func (s *TermService) DeleteDuty(ctx context.Context, actor, versionID, id string) (Version, error) {
	if !ids(actor, versionID, id) {
		return Version{}, ErrInvalid
	}
	return s.repo.DeleteDuty(ctx, actor, versionID, id)
}
func (s *TermService) Generate(ctx context.Context, actor, versionID string) (GenerateResult, error) {
	if !ids(actor, versionID) {
		return GenerateResult{}, ErrInvalid
	}
	return s.repo.Generate(ctx, actor, versionID)
}

// Move takes a version to its next status: submitted, approved,
// published, or back to draft (returned, with a reason).
func (s *TermService) Move(ctx context.Context, actor, versionID, to, reason string) (Version, error) {
	reason, ok := text(reason, 1000)
	if !ids(actor, versionID) || !ok || (to != "submitted" && to != "approved" && to != "published" && to != "draft") || (to == "draft" && reason == "") {
		return Version{}, ErrInvalid
	}
	return s.repo.Move(ctx, actor, versionID, to, reason)
}
func (s *TermService) Amend(ctx context.Context, actor, versionID, reason string) (Version, error) {
	reason, ok := text(reason, 1000)
	if !ids(actor, versionID) || !ok || reason == "" {
		return Version{}, ErrInvalid
	}
	return s.repo.Amend(ctx, actor, versionID, reason)
}
func (s *TermService) StudentRoutines(ctx context.Context, actor string) ([]StudentRoutine, error) {
	if !ids(actor) {
		return nil, ErrInvalid
	}
	out, _, _, err := s.repo.StudentRoutines(ctx, actor)
	return out, err
}
func (s *TermService) TeacherDuties(ctx context.Context, actor string) ([]TeacherDuty, error) {
	if !ids(actor) {
		return nil, ErrInvalid
	}
	return s.repo.TeacherDuties(ctx, actor)
}

// Available reports whether routine PDFs can be rendered.
func (s *TermService) DocumentsAvailable() bool {
	return s != nil && s.Documents != nil && s.Documents.RoutineAvailable()
}

func (s *TermService) Document(ctx context.Context, versionID string, in DocumentRequest) ([]byte, error) {
	if !s.DocumentsAvailable() {
		return nil, ErrDocuments
	}
	if in.Language == "" {
		in.Language = "en"
	}
	if !ids(versionID) || (in.Kind != "class" && in.Kind != "room" && in.Kind != "invigilation") || (in.Language != "en" && in.Language != "bn") ||
		(in.ClassID != "" && !ids(in.ClassID)) || (in.RoomID != "" && !ids(in.RoomID)) || (in.SectionID != "" && !ids(in.SectionID)) {
		return nil, ErrInvalid
	}
	doc, err := s.repo.Document(ctx, versionID, in)
	if err != nil {
		return nil, err
	}
	return s.Documents.RenderRoutine(ctx, doc)
}

// StudentDocument renders a student's own class routine (their section's
// papers and rooms) from a published version.
func (s *TermService) StudentDocument(ctx context.Context, actor, versionID, language string) ([]byte, error) {
	if !s.DocumentsAvailable() {
		return nil, ErrDocuments
	}
	if language == "" {
		language = "en"
	}
	if !ids(actor, versionID) || (language != "en" && language != "bn") {
		return nil, ErrInvalid
	}
	doc, err := s.repo.StudentDocument(ctx, actor, versionID, language)
	if err != nil {
		return nil, err
	}
	return s.Documents.RenderRoutine(ctx, doc)
}
