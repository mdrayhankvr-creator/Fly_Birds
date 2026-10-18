// Package notify keeps in-app notifications, per-user preferences and a mock
// SMS log. Dispatch derives notifications from facts other modules already
// store (published notices, absences, results, overdue fees, diaries), so it
// can run repeatedly without creating duplicates.
package notify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid notification input")
	ErrNotFound = errors.New("notification not found")
)

var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var Categories = []string{"notice", "absence", "result", "fee", "diary"}

// Dhaka is Bangladesh Standard Time (no daylight saving).
var Dhaka = time.FixedZone("BST", 6*60*60)

type Notification struct {
	ID        string `json:"id"`
	Category  string `json:"category"`
	TitleEN   string `json:"title_en"`
	TitleBN   string `json:"title_bn"`
	BodyEN    string `json:"body_en"`
	BodyBN    string `json:"body_bn"`
	Link      string `json:"link"`
	Urgent    bool   `json:"urgent"`
	Read      bool   `json:"read"`
	CreatedAt string `json:"created_at"`
}
type Inbox struct {
	Items    []Notification `json:"items"`
	Total    int64          `json:"total"`
	Unread   int64          `json:"unread"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
type Preference struct {
	Category string `json:"category"`
	InApp    bool   `json:"in_app"`
	SMS      bool   `json:"sms"`
}
type SMS struct {
	ID            string  `json:"id"`
	RecipientName string  `json:"recipient_name"`
	Phone         string  `json:"phone"`
	Category      string  `json:"category"`
	Body          string  `json:"body"`
	Status        string  `json:"status"`
	Provider      string  `json:"provider"`
	ProviderRef   string  `json:"provider_ref"`
	Error         string  `json:"error"`
	Attempts      int     `json:"attempts"`
	CreatedAt     string  `json:"created_at"`
	ProcessedAt   *string `json:"processed_at"`
}
type SMSPage struct {
	Items    []SMS            `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
	Counts   map[string]int64 `json:"counts"`
}

// Result reports one dispatch run. Created counts are new or revived
// notifications; SMS counts describe the mock provider, never real delivery.
type Result struct {
	Skipped     bool             `json:"skipped"`
	Disabled    bool             `json:"disabled"`
	Created     map[string]int64 `json:"created"`
	Withdrawn   int64            `json:"withdrawn"`
	SMSQueued   int64            `json:"sms_queued"`
	SMSSentMock int64            `json:"sms_sent_mock"`
	SMSFailed   int64            `json:"sms_failed"`
	SMSRetrying int64            `json:"sms_retrying"`
}

// SMSProvider is the boundary for a real gateway. Send returns the provider's
// message reference; acceptance is not delivery.
type SMSProvider interface {
	Name() string
	Send(ctx context.Context, phone, body string) (string, error)
}

// MockSMS accepts every message locally and never contacts a network or
// spends credit. Its references start with MOCK-.
type MockSMS struct{}

func (MockSMS) Name() string { return "mock" }
func (MockSMS) Send(_ context.Context, phone, body string) (string, error) {
	if phone == "" || body == "" {
		return "", ErrInvalid
	}
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "MOCK-" + hex.EncodeToString(raw), nil
}

type Repository interface {
	Inbox(context.Context, string, bool, int, int) (Inbox, error)
	MarkRead(context.Context, string, string) error
	MarkAllRead(context.Context, string) (int64, error)
	Preferences(context.Context, string) ([]Preference, error)
	SavePreferences(context.Context, string, []Preference) error
	SMSLog(context.Context, string, int, int) (SMSPage, error)
	Dispatch(context.Context, time.Time) (Result, error)
	ClaimSMS(context.Context, int) ([]QueuedSMS, error)
	FinishSMS(context.Context, string, string, string, string) error
}

type QueuedSMS struct {
	ID       string
	Phone    string
	Body     string
	Attempts int
}

// MaxSMSAttempts is how often a failing message is retried before it fails.
const MaxSMSAttempts = 3

type Service struct {
	repo     Repository
	provider SMSProvider
	now      func() time.Time
}

func NewService(repo Repository, provider SMSProvider) *Service {
	if provider == nil {
		provider = MockSMS{}
	}
	return &Service{repo: repo, provider: provider, now: time.Now}
}

func validPage(page, size int) bool { return page >= 1 && page <= 10000 && size >= 1 && size <= 100 }

func (s *Service) Inbox(ctx context.Context, user string, unreadOnly bool, page, size int) (Inbox, error) {
	if !uuid.MatchString(user) || !validPage(page, size) {
		return Inbox{}, ErrInvalid
	}
	return s.repo.Inbox(ctx, user, unreadOnly, page, size)
}
func (s *Service) MarkRead(ctx context.Context, user, id string) error {
	if !uuid.MatchString(user) || !uuid.MatchString(id) {
		return ErrNotFound
	}
	return s.repo.MarkRead(ctx, user, id)
}
func (s *Service) MarkAllRead(ctx context.Context, user string) (int64, error) {
	if !uuid.MatchString(user) {
		return 0, ErrInvalid
	}
	return s.repo.MarkAllRead(ctx, user)
}
func (s *Service) Preferences(ctx context.Context, user string) ([]Preference, error) {
	if !uuid.MatchString(user) {
		return nil, ErrInvalid
	}
	return s.repo.Preferences(ctx, user)
}
func (s *Service) SavePreferences(ctx context.Context, user string, prefs []Preference) ([]Preference, error) {
	if !uuid.MatchString(user) || len(prefs) == 0 || len(prefs) > len(Categories) {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range prefs {
		known := false
		for _, c := range Categories {
			known = known || c == p.Category
		}
		if !known || seen[p.Category] {
			return nil, ErrInvalid
		}
		seen[p.Category] = true
	}
	if err := s.repo.SavePreferences(ctx, user, prefs); err != nil {
		return nil, err
	}
	return s.repo.Preferences(ctx, user)
}
func (s *Service) SMSLog(ctx context.Context, status string, page, size int) (SMSPage, error) {
	if !validPage(page, size) || (status != "" && status != "queued" && status != "sent_mock" && status != "failed" && status != "cancelled") {
		return SMSPage{}, ErrInvalid
	}
	return s.repo.SMSLog(ctx, status, page, size)
}

// Dispatch creates due notifications and SMS rows, withdraws ones whose cause
// was corrected, then hands queued SMS to the provider. Concurrent runs are
// skipped rather than duplicated.
func (s *Service) Dispatch(ctx context.Context) (Result, error) {
	res, err := s.repo.Dispatch(ctx, s.now())
	if err != nil || res.Skipped || res.Disabled {
		return res, err
	}
	queued, err := s.repo.ClaimSMS(ctx, 200)
	if err != nil {
		return res, err
	}
	for _, m := range queued {
		ref, sendErr := s.provider.Send(ctx, m.Phone, m.Body)
		switch {
		case sendErr == nil:
			err = s.repo.FinishSMS(ctx, m.ID, "sent_mock", ref, "")
			res.SMSSentMock++
		case m.Attempts+1 >= MaxSMSAttempts:
			err = s.repo.FinishSMS(ctx, m.ID, "failed", "", sendErr.Error())
			res.SMSFailed++
		default:
			err = s.repo.FinishSMS(ctx, m.ID, "queued", "", sendErr.Error())
			res.SMSRetrying++
		}
		if err != nil {
			return res, err
		}
	}
	return res, nil
}
