package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepo struct {
	Repository
	queued   []QueuedSMS
	finished map[string]string
}

func (f *fakeRepo) Dispatch(context.Context, time.Time) (Result, error) {
	return Result{Created: map[string]int64{}}, nil
}
func (f *fakeRepo) ClaimSMS(context.Context, int) ([]QueuedSMS, error) { return f.queued, nil }
func (f *fakeRepo) FinishSMS(_ context.Context, id, status, ref, _ string) error {
	f.finished[id] = status + "|" + ref
	return nil
}

type failing struct{}

func (failing) Name() string { return "failing" }
func (failing) Send(context.Context, string, string) (string, error) {
	return "", errors.New("gateway down")
}

func TestMockSMSNeverClaimsDelivery(t *testing.T) {
	ref, err := MockSMS{}.Send(context.Background(), "01711000000", "hello")
	if err != nil || !strings.HasPrefix(ref, "MOCK-") || len(ref) != 17 {
		t.Fatalf("ref %q %v", ref, err)
	}
	if _, err := (MockSMS{}).Send(context.Background(), "", "x"); err == nil {
		t.Fatal("empty phone accepted")
	}
	repo := &fakeRepo{queued: []QueuedSMS{{ID: "a", Phone: "01711000000", Body: "x"}}, finished: map[string]string{}}
	res, err := NewService(repo, nil).Dispatch(context.Background())
	if err != nil || res.SMSSentMock != 1 || !strings.HasPrefix(repo.finished["a"], "sent_mock|MOCK-") {
		t.Fatalf("mock dispatch %+v %v %v", res, err, repo.finished)
	}
}

func TestFailingProviderRetriesThenFails(t *testing.T) {
	repo := &fakeRepo{queued: []QueuedSMS{{ID: "first", Attempts: 0}, {ID: "last", Attempts: MaxSMSAttempts - 1}}, finished: map[string]string{}}
	res, err := NewService(repo, failing{}).Dispatch(context.Background())
	if err != nil || res.SMSRetrying != 1 || res.SMSFailed != 1 || repo.finished["first"] != "queued|" || repo.finished["last"] != "failed|" {
		t.Fatalf("retry %+v %v %v", res, err, repo.finished)
	}
}

func TestPreferenceValidation(t *testing.T) {
	s := NewService(&fakeRepo{}, nil)
	user := "11111111-1111-1111-1111-111111111111"
	for _, prefs := range [][]Preference{nil, {{Category: "spam"}}, {{Category: "fee"}, {Category: "fee"}}} {
		if _, err := s.SavePreferences(context.Background(), user, prefs); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %+v", prefs)
		}
	}
	if _, err := s.SMSLog(context.Background(), "delivered", 1, 20); !errors.Is(err, ErrInvalid) {
		t.Fatal("delivered is not a mock SMS status")
	}
}
