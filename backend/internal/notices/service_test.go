package notices

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingRepo struct {
	Repository
	notices []NoticeInput
	events  []EventInput
}

func (r *recordingRepo) Save(_ context.Context, _, _ string, in NoticeInput) (Notice, error) {
	r.notices = append(r.notices, in)
	return Notice{}, nil
}
func (r *recordingRepo) SaveEvent(_ context.Context, _, _ string, in EventInput) (Event, error) {
	r.events = append(r.events, in)
	return Event{}, nil
}
func (r *recordingRepo) Publish(context.Context, string, string, *time.Time) (Notice, error) {
	return Notice{}, nil
}

const actor = "11111111-1111-1111-1111-111111111111"

func TestNoticeValidation(t *testing.T) {
	repo := &recordingRepo{}
	s := NewService(repo)
	class := "22222222-2222-2222-2222-222222222222"
	bad := []NoticeInput{
		{TitleEN: "", BodyEN: "x", AudienceRoles: []string{"student"}},
		{TitleEN: "T", AudienceRoles: []string{"student"}},
		{TitleEN: "T", BodyEN: "x", AudienceRoles: nil},
		{TitleEN: "T", BodyEN: "x", AudienceRoles: []string{"admin"}},
		{TitleEN: "T", BodyEN: "x", AudienceRoles: []string{"student"}, Priority: "critical"},
		{TitleEN: "T", BodyEN: "x", AudienceRoles: []string{"student"}, Targets: []Target{{ClassID: class}, {ClassID: strings.ToUpper(class)}}},
		{TitleEN: "T", BodyEN: "x", AudienceRoles: []string{"student"}, ExpiresAt: ptr("tomorrow")},
	}
	for i, in := range bad {
		if _, err := s.Save(context.Background(), actor, "", in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	if _, err := s.Save(context.Background(), actor, "", NoticeInput{TitleEN: " Exam ", BodyBN: "বার্তা", AudienceRoles: []string{"student", "guardian", "student"}, ExpiresAt: ptr("2026-12-01T10:00:00+06:00")}); err != nil {
		t.Fatal(err)
	}
	got := repo.notices[0]
	if got.TitleEN != "Exam" || strings.Join(got.AudienceRoles, ",") != "guardian,student" || got.Priority != "normal" || *got.ExpiresAt != "2026-12-01T04:00:00Z" {
		t.Fatalf("normalized %+v", got)
	}
	if _, err := s.Publish(context.Background(), actor, class, PublishInput{PublishAt: ptr("2026-13-01")}); !errors.Is(err, ErrInvalid) {
		t.Fatal("bad publish time accepted")
	}
}

func TestEventValidation(t *testing.T) {
	repo := &recordingRepo{}
	s := NewService(repo)
	base := EventInput{TitleEN: "Exam", Kind: "exam", StartsOn: "2026-11-02", EndsOn: "2026-11-02", AudienceRoles: []string{"student"}}
	mutate := func(f func(*EventInput)) EventInput { e := base; f(&e); return e }
	bad := []EventInput{
		mutate(func(e *EventInput) { e.Kind = "party" }),
		mutate(func(e *EventInput) { e.EndsOn = "2026-11-01" }),
		mutate(func(e *EventInput) { e.StartTime = ptr("10:00") }),
		mutate(func(e *EventInput) { e.StartTime, e.EndTime = ptr("10:00"), ptr("09:59") }),
		mutate(func(e *EventInput) { e.StartTime, e.EndTime = ptr("25:00"), ptr("26:00") }),
		mutate(func(e *EventInput) { e.EndsOn = "2028-01-01" }),
	}
	for i, in := range bad {
		if _, err := s.SaveEvent(context.Background(), actor, "", in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("event case %d accepted", i)
		}
	}
	if _, err := s.SaveEvent(context.Background(), actor, "", mutate(func(e *EventInput) { e.EndsOn = "2026-11-03"; e.StartTime, e.EndTime = ptr("15:00"), ptr("09:00") })); err != nil {
		t.Fatal("multi-day timed event rejected")
	}
	if _, err := s.ListEvents(context.Background(), actor, Query{From: "2026-01-01", To: "2027-03-01"}, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("over-long range accepted")
	}
	if !strings.Contains(AudiencePredicate("n", "notice_targets", "notice_id"), "g.user_id = u.id") {
		t.Fatal("guardian targeting missing")
	}
}

func ptr(s string) *string { return &s }
