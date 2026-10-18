package diary

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

// Dhaka is Bangladesh Standard Time (no daylight saving).
var Dhaka = time.FixedZone("BST", 6*60*60)

// MaxPlanAhead lets teachers prepare diaries a week ahead.
const MaxPlanAhead = 7

type Repository interface {
	Assignments(context.Context, string, string) ([]Assignment, error)
	Teaching(context.Context, string, Query) (Page[Diary], error)
	Feed(context.Context, Viewer, Query) (Page[Diary], error)
	Oversight(context.Context, Query) (Page[Diary], error)
	Get(context.Context, Viewer, string) (Diary, error)
	Save(context.Context, string, string, DiaryInput) (Diary, error)
	Publish(context.Context, string, string) (Diary, error)
	Delete(context.Context, string, string) error
	FileAccess(context.Context, Viewer, string, string) error
	MySubmission(context.Context, string, string) (Submission, error)
	Submit(context.Context, string, string, SubmissionInput) (Submission, error)
	Submissions(context.Context, Viewer, string) ([]Submission, error)
	Review(context.Context, string, string, string) (Submission, error)
	SubmissionFileAccess(context.Context, Viewer, string, string) error
	Settings(context.Context) (Settings, error)
	SaveSettings(context.Context, string, Settings) (Settings, error)
	Compliance(context.Context, string) ([]ComplianceRow, error)
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }

func (s *Service) today() string { return s.now().In(Dhaka).Format(time.DateOnly) }

func validID(v string) bool { return uuid.MatchString(v) }
func validDate(v string) bool {
	t, err := time.Parse(time.DateOnly, v)
	return err == nil && t.Year() >= 2000 && t.Year() <= 2200
}
func clean(v string, max int) (string, bool) {
	v = strings.TrimSpace(v)
	return v, utf8.ValidString(v) && len(v) <= max
}
func validFiles(ids []string, max int) bool {
	if len(ids) > max {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !validID(id) || seen[strings.ToLower(id)] {
			return false
		}
		seen[strings.ToLower(id)] = true
	}
	return true
}

func (s *Service) validQuery(q *Query) bool {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	q.Search = strings.TrimSpace(q.Search)
	if q.Status != "" && q.Status != "draft" && q.Status != "published" {
		return false
	}
	return q.Page >= 1 && q.Page <= 10000 && q.PageSize >= 1 && q.PageSize <= 100 && len(q.Search) <= 100 && utf8.ValidString(q.Search) &&
		(q.From == "" || validDate(q.From)) && (q.To == "" || validDate(q.To)) && (q.From == "" || q.To == "" || q.From <= q.To) &&
		(q.SubjectID == "" || validID(q.SubjectID)) && (q.SectionID == "" || validID(q.SectionID))
}

func (s *Service) Assignments(ctx context.Context, actor, date string) ([]Assignment, error) {
	if date == "" {
		date = s.today()
	}
	if !validID(actor) || !validDate(date) {
		return nil, ErrInvalid
	}
	return s.repo.Assignments(ctx, actor, date)
}
func (s *Service) Teaching(ctx context.Context, actor string, q Query) (Page[Diary], error) {
	if !validID(actor) || !s.validQuery(&q) {
		return Page[Diary]{}, ErrInvalid
	}
	return s.repo.Teaching(ctx, actor, q)
}

// Feed lists published diaries for a student's own section or, for a
// guardian, a linked child's section.
func (s *Service) Feed(ctx context.Context, v Viewer, q Query) (Page[Diary], error) {
	if !validID(v.UserID) || (v.Role != "student" && v.Role != "guardian") || (v.Role == "guardian" && !validID(v.StudentID)) || !s.validQuery(&q) {
		return Page[Diary]{}, ErrInvalid
	}
	return s.repo.Feed(ctx, v, q)
}
func (s *Service) Oversight(ctx context.Context, q Query) (Page[Diary], error) {
	if !s.validQuery(&q) {
		return Page[Diary]{}, ErrInvalid
	}
	return s.repo.Oversight(ctx, q)
}
func (s *Service) Get(ctx context.Context, v Viewer, id string) (Diary, error) {
	if !validID(v.UserID) || !validID(id) || (v.StudentID != "" && !validID(v.StudentID)) {
		return Diary{}, ErrNotFound
	}
	return s.repo.Get(ctx, v, id)
}

// Save creates or edits the actor's diary. A new diary must be for an
// assigned section and subject on a day inside the session, no more than a
// week ahead; edits change only content, submission settings and files.
func (s *Service) Save(ctx context.Context, actor, id string, in DiaryInput) (Diary, error) {
	var ok [4]bool
	in.LessonTopics, ok[0] = clean(in.LessonTopics, 10000)
	in.ClassNotes, ok[1] = clean(in.ClassNotes, 20000)
	in.Homework, ok[2] = clean(in.Homework, 10000)
	in.Instructions, ok[3] = clean(in.Instructions, 10000)
	for _, v := range ok {
		if !v {
			return Diary{}, ErrInvalid
		}
	}
	if !validID(actor) || (id != "" && !validID(id)) || !validFiles(in.FileIDs, 10) {
		return Diary{}, ErrInvalid
	}
	if id == "" {
		limit, _ := time.Parse(time.DateOnly, s.today())
		if !validID(in.SectionID) || !validID(in.SubjectID) || !validDate(in.DiaryDate) || in.DiaryDate > limit.AddDate(0, 0, MaxPlanAhead).Format(time.DateOnly) {
			return Diary{}, ErrInvalid
		}
	}
	if in.SubmissionDueAt != nil && strings.TrimSpace(*in.SubmissionDueAt) == "" {
		in.SubmissionDueAt = nil
	}
	if in.SubmissionDueAt != nil {
		due, err := time.Parse(time.RFC3339, *in.SubmissionDueAt)
		if err != nil || !in.SubmissionEnabled {
			return Diary{}, ErrInvalid
		}
		v := due.UTC().Format(time.RFC3339)
		in.SubmissionDueAt = &v
	}
	return s.repo.Save(ctx, actor, id, in)
}
func (s *Service) Publish(ctx context.Context, actor, id string) (Diary, error) {
	if !validID(actor) || !validID(id) {
		return Diary{}, ErrInvalid
	}
	return s.repo.Publish(ctx, actor, id)
}
func (s *Service) Delete(ctx context.Context, actor, id string) error {
	if !validID(actor) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.Delete(ctx, actor, id)
}
func (s *Service) FileAccess(ctx context.Context, v Viewer, diaryID, fileID string) error {
	if !validID(v.UserID) || !validID(diaryID) || !validID(fileID) {
		return ErrNotFound
	}
	return s.repo.FileAccess(ctx, v, diaryID, fileID)
}
func (s *Service) MySubmission(ctx context.Context, actor, diaryID string) (Submission, error) {
	if !validID(actor) || !validID(diaryID) {
		return Submission{}, ErrNotFound
	}
	return s.repo.MySubmission(ctx, actor, diaryID)
}

// Submit creates or replaces the student's homework while submission is open
// and before the teacher reviews it.
func (s *Service) Submit(ctx context.Context, actor, diaryID string, in SubmissionInput) (Submission, error) {
	var ok bool
	in.AnswerText, ok = clean(in.AnswerText, 10000)
	if !validID(actor) || !validID(diaryID) || !ok || !validFiles(in.FileIDs, 5) || (in.AnswerText == "" && len(in.FileIDs) == 0) {
		return Submission{}, ErrInvalid
	}
	return s.repo.Submit(ctx, actor, diaryID, in)
}
func (s *Service) Submissions(ctx context.Context, v Viewer, diaryID string) ([]Submission, error) {
	if !validID(v.UserID) || !validID(diaryID) {
		return nil, ErrNotFound
	}
	return s.repo.Submissions(ctx, v, diaryID)
}
func (s *Service) Review(ctx context.Context, actor, submissionID, note string) (Submission, error) {
	note, ok := clean(note, 2000)
	if !validID(actor) || !validID(submissionID) || !ok {
		return Submission{}, ErrInvalid
	}
	return s.repo.Review(ctx, actor, submissionID, note)
}
func (s *Service) SubmissionFileAccess(ctx context.Context, v Viewer, submissionID, fileID string) error {
	if !validID(v.UserID) || !validID(submissionID) || !validID(fileID) {
		return ErrNotFound
	}
	return s.repo.SubmissionFileAccess(ctx, v, submissionID, fileID)
}
func (s *Service) Settings(ctx context.Context) (Settings, error) { return s.repo.Settings(ctx) }
func (s *Service) SaveSettings(ctx context.Context, actor string, in Settings) (Settings, error) {
	t, err := time.Parse("15:04", in.ReminderTime)
	if !validID(actor) || err != nil || t.Format("15:04") != in.ReminderTime {
		return Settings{}, ErrInvalid
	}
	return s.repo.SaveSettings(ctx, actor, in)
}
func (s *Service) Compliance(ctx context.Context, date string) ([]ComplianceRow, error) {
	if date == "" {
		date = s.today()
	}
	if !validDate(date) {
		return nil, ErrInvalid
	}
	return s.repo.Compliance(ctx, date)
}
