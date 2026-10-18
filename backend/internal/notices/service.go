package notices

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Repository interface {
	ListManaged(context.Context, Query) (Page[Notice], error)
	Feed(context.Context, string, Query) (Page[Notice], error)
	Get(context.Context, string, string, bool) (Notice, error)
	Save(context.Context, string, string, NoticeInput) (Notice, error)
	Publish(context.Context, string, string, *time.Time) (Notice, error)
	Archive(context.Context, string, string) (Notice, error)
	Delete(context.Context, string, string) error
	AttachmentFile(context.Context, string, string, string, bool) (string, error)
	ListEvents(context.Context, string, Query, bool) ([]Event, error)
	SaveEvent(context.Context, string, string, EventInput) (Event, error)
	SetEventStatus(context.Context, string, string, string) (Event, error)
	DeleteEvent(context.Context, string, string) error
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service { return &Service{repo: repo, now: time.Now} }

func clean(v string, max int, required bool) (string, bool) {
	v = strings.TrimSpace(v)
	return v, utf8.ValidString(v) && len(v) <= max && (!required || v != "")
}
func validID(v string) bool { return uuid.MatchString(v) }

func normalizeRoles(roles []string) ([]string, bool) {
	out := []string{}
	for _, r := range roles {
		if !slices.Contains(AudienceRoles, r) {
			return nil, false
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out, len(out) > 0
}

func validTargets(targets []Target) bool {
	if len(targets) > 50 {
		return false
	}
	seen := map[string]bool{}
	for _, t := range targets {
		key := strings.ToLower(t.ClassID)
		if t.SectionID != nil {
			if !validID(*t.SectionID) {
				return false
			}
			key += ":" + strings.ToLower(*t.SectionID)
		}
		if !validID(t.ClassID) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func validQuery(q *Query) bool {
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	q.Search = strings.TrimSpace(q.Search)
	switch q.Status {
	case "", "draft", "published", "archived", "scheduled", "expired", "cancelled":
	default:
		return false
	}
	switch q.Priority {
	case "", "normal", "important", "urgent":
	default:
		return false
	}
	return q.Page >= 1 && q.Page <= 10000 && q.PageSize >= 1 && q.PageSize <= 100 && len(q.Search) <= 100 && utf8.ValidString(q.Search)
}

func parseTime(v *string) (*time.Time, bool) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*v))
	if err != nil || t.Year() < 2000 || t.Year() > 2200 {
		return nil, false
	}
	return &t, true
}

func (s *Service) ListManaged(ctx context.Context, q Query) (Page[Notice], error) {
	if !validQuery(&q) {
		return Page[Notice]{}, ErrInvalid
	}
	return s.repo.ListManaged(ctx, q)
}

// Feed lists live notices addressed to the user, urgent first.
func (s *Service) Feed(ctx context.Context, actor string, q Query) (Page[Notice], error) {
	if !validID(actor) || !validQuery(&q) {
		return Page[Notice]{}, ErrInvalid
	}
	return s.repo.Feed(ctx, actor, q)
}

func (s *Service) Get(ctx context.Context, actor, id string, manager bool) (Notice, error) {
	if !validID(actor) || !validID(id) {
		return Notice{}, ErrInvalid
	}
	return s.repo.Get(ctx, actor, id, manager)
}

func (s *Service) Save(ctx context.Context, actor, id string, in NoticeInput) (Notice, error) {
	var ok [5]bool
	in.TitleEN, ok[0] = clean(in.TitleEN, 200, true)
	in.TitleBN, ok[1] = clean(in.TitleBN, 600, false)
	in.BodyEN, ok[2] = clean(in.BodyEN, 20000, false)
	in.BodyBN, ok[3] = clean(in.BodyBN, 60000, false)
	in.AudienceRoles, ok[4] = normalizeRoles(in.AudienceRoles)
	if in.Priority == "" {
		in.Priority = "normal"
	}
	expires, okExpiry := parseTime(in.ExpiresAt)
	if !validID(actor) || (id != "" && !validID(id)) || slices.Contains(ok[:], false) || !okExpiry || (in.BodyEN == "" && in.BodyBN == "") || !validTargets(in.Targets) || len(in.FileIDs) > 10 {
		return Notice{}, ErrInvalid
	}
	if in.Priority != "normal" && in.Priority != "important" && in.Priority != "urgent" {
		return Notice{}, ErrInvalid
	}
	if expires != nil {
		v := expires.UTC().Format(time.RFC3339)
		in.ExpiresAt = &v
	} else {
		in.ExpiresAt = nil
	}
	for _, f := range in.FileIDs {
		if !validID(f) {
			return Notice{}, ErrInvalid
		}
	}
	return s.repo.Save(ctx, actor, id, in)
}

// Publish makes a notice visible at publish_at. When omitted the database
// clock is used, the same clock feeds and dispatch compare against, so a
// notice published "now" is live immediately even if server clocks differ.
// A future time schedules it.
func (s *Service) Publish(ctx context.Context, actor, id string, in PublishInput) (Notice, error) {
	at, ok := parseTime(in.PublishAt)
	if !validID(actor) || !validID(id) || !ok {
		return Notice{}, ErrInvalid
	}
	return s.repo.Publish(ctx, actor, id, at)
}

func (s *Service) Archive(ctx context.Context, actor, id string) (Notice, error) {
	if !validID(actor) || !validID(id) {
		return Notice{}, ErrInvalid
	}
	return s.repo.Archive(ctx, actor, id)
}

func (s *Service) Delete(ctx context.Context, actor, id string) error {
	if !validID(actor) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.Delete(ctx, actor, id)
}

// AttachmentFile authorizes an attachment download and returns its file id.
func (s *Service) AttachmentFile(ctx context.Context, actor, noticeID, fileID string, manager bool) (string, error) {
	if !validID(actor) || !validID(noticeID) || !validID(fileID) {
		return "", ErrNotFound
	}
	return s.repo.AttachmentFile(ctx, actor, noticeID, fileID, manager)
}

func validDay(v string) bool {
	t, err := time.Parse(time.DateOnly, v)
	return err == nil && t.Year() >= 2000 && t.Year() <= 2200
}

// ListEvents returns events overlapping [from, to]; ranges are capped at 400
// days. Managers also see drafts.
func (s *Service) ListEvents(ctx context.Context, actor string, q Query, manager bool) ([]Event, error) {
	if !validID(actor) || !validDay(q.From) || !validDay(q.To) || q.From > q.To {
		return nil, ErrInvalid
	}
	from, _ := time.Parse(time.DateOnly, q.From)
	to, _ := time.Parse(time.DateOnly, q.To)
	if to.Sub(from) > 400*24*time.Hour || (q.Status != "" && q.Status != "draft" && q.Status != "published" && q.Status != "cancelled") {
		return nil, ErrInvalid
	}
	return s.repo.ListEvents(ctx, actor, q, manager)
}

var hhmm = func(v string) bool { t, err := time.Parse("15:04", v); return err == nil && t.Format("15:04") == v }

func (s *Service) SaveEvent(ctx context.Context, actor, id string, in EventInput) (Event, error) {
	var ok [6]bool
	in.TitleEN, ok[0] = clean(in.TitleEN, 200, true)
	in.TitleBN, ok[1] = clean(in.TitleBN, 600, false)
	in.DescriptionEN, ok[2] = clean(in.DescriptionEN, 5000, false)
	in.DescriptionBN, ok[3] = clean(in.DescriptionBN, 15000, false)
	in.Location, ok[4] = clean(in.Location, 300, false)
	in.AudienceRoles, ok[5] = normalizeRoles(in.AudienceRoles)
	if !validID(actor) || (id != "" && !validID(id)) || slices.Contains(ok[:], false) || !validDay(in.StartsOn) || !validDay(in.EndsOn) || in.EndsOn < in.StartsOn || !validTargets(in.Targets) {
		return Event{}, ErrInvalid
	}
	switch in.Kind {
	case "holiday", "exam", "meeting", "event", "other":
	default:
		return Event{}, ErrInvalid
	}
	start, _ := time.Parse(time.DateOnly, in.StartsOn)
	end, _ := time.Parse(time.DateOnly, in.EndsOn)
	if end.Sub(start) > 366*24*time.Hour {
		return Event{}, ErrInvalid
	}
	empty := func(p *string) bool { return p == nil || *p == "" }
	if empty(in.StartTime) != empty(in.EndTime) {
		return Event{}, ErrInvalid
	}
	if empty(in.StartTime) {
		in.StartTime, in.EndTime = nil, nil
	} else if !hhmm(*in.StartTime) || !hhmm(*in.EndTime) || (in.StartsOn == in.EndsOn && *in.EndTime <= *in.StartTime) {
		return Event{}, ErrInvalid
	}
	return s.repo.SaveEvent(ctx, actor, id, in)
}

func (s *Service) SetEventStatus(ctx context.Context, actor, id, status string) (Event, error) {
	if !validID(actor) || !validID(id) || (status != "published" && status != "cancelled") {
		return Event{}, ErrInvalid
	}
	return s.repo.SetEventStatus(ctx, actor, id, status)
}

func (s *Service) DeleteEvent(ctx context.Context, actor, id string) error {
	if !validID(actor) || !validID(id) {
		return ErrInvalid
	}
	return s.repo.DeleteEvent(ctx, actor, id)
}
