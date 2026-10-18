package diary

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type recordingRepo struct {
	Repository
	saved []DiaryInput
}

func (r *recordingRepo) Save(_ context.Context, _, _ string, in DiaryInput) (Diary, error) {
	r.saved = append(r.saved, in)
	return Diary{}, nil
}
func (r *recordingRepo) Submit(context.Context, string, string, SubmissionInput) (Submission, error) {
	return Submission{}, nil
}

const (
	actor   = "11111111-1111-1111-1111-111111111111"
	section = "22222222-2222-2222-2222-222222222222"
	subject = "33333333-3333-3333-3333-333333333333"
	file    = "44444444-4444-4444-4444-444444444444"
)

func TestDiaryValidation(t *testing.T) {
	repo := &recordingRepo{}
	s := NewService(repo)
	s.now = func() time.Time { return time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC) } // 18 Oct in Dhaka
	due := "2026-10-20T10:00:00+06:00"
	bad := []DiaryInput{
		{SectionID: section, SubjectID: subject, DiaryDate: "2026-10-26"},
		{SectionID: section, SubjectID: subject, DiaryDate: "2026-02-30"},
		{SectionID: "x", SubjectID: subject, DiaryDate: "2026-10-18"},
		{SectionID: section, SubjectID: subject, DiaryDate: "2026-10-18", SubmissionDueAt: &due},
		{SectionID: section, SubjectID: subject, DiaryDate: "2026-10-18", FileIDs: []string{file, strings.ToUpper(file)}},
		{SectionID: section, SubjectID: subject, DiaryDate: "2026-10-18", ClassNotes: strings.Repeat("x", 20001)},
	}
	for i, in := range bad {
		if _, err := s.Save(context.Background(), actor, "", in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("case %d accepted: %v", i, err)
		}
	}
	if _, err := s.Save(context.Background(), actor, "", DiaryInput{SectionID: section, SubjectID: subject, DiaryDate: "2026-10-25", Homework: "  Read  ", SubmissionEnabled: true, SubmissionDueAt: &due}); err != nil {
		t.Fatal(err)
	}
	if got := repo.saved[0]; got.Homework != "Read" || *got.SubmissionDueAt != "2026-10-20T04:00:00Z" {
		t.Fatalf("normalized %+v", got)
	}
	if _, err := s.Submit(context.Background(), actor, section, SubmissionInput{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty submission accepted")
	}
	if _, err := s.Submit(context.Background(), actor, section, SubmissionInput{FileIDs: []string{file, section, subject, actor, "55555555-5555-5555-5555-555555555555", "66666666-6666-6666-6666-666666666666"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("six homework files accepted")
	}
	if _, err := s.SaveSettings(context.Background(), actor, Settings{ReminderTime: "7:5"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("bad reminder time accepted")
	}
}
