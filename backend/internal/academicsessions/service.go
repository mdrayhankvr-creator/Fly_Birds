package academicsessions

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid academic session")
	ErrDuplicate = errors.New("academic session name already exists")
	ErrNotFound  = errors.New("academic session not found")
	ErrForbidden = errors.New("academic session operation denied")
)

var sessionName = regexp.MustCompile(`^([0-9]{4})(?:[-–]([0-9]{4}))?$`)
var uuid = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Session struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Input struct {
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}
type Query struct {
	Search   string
	Page     int
	PageSize int
}
type Page struct {
	Items    []Session `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
type Repository interface {
	Create(context.Context, string, Input, string) (Session, error)
	List(context.Context, Query) (Page, error)
	Get(context.Context, string) (Session, error)
	Update(context.Context, string, string, Input) (Session, error)
	SetActive(context.Context, string, string, bool) (Session, error)
	Current(context.Context) (Session, error)
}
type Service struct{ repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func normalize(input Input) (Input, string, error) {
	input.Name = strings.TrimSpace(input.Name)
	match := sessionName.FindStringSubmatch(input.Name)
	if match == nil {
		return Input{}, "", ErrInvalid
	}
	year, _ := strconv.Atoi(match[1])
	if year < 1900 || year > 2200 {
		return Input{}, "", ErrInvalid
	}
	if match[2] != "" {
		endYear, _ := strconv.Atoi(match[2])
		if endYear != year+1 {
			return Input{}, "", ErrInvalid
		}
	}
	start, err := time.Parse("2006-01-02", input.StartDate)
	if err != nil || start.Format("2006-01-02") != input.StartDate {
		return Input{}, "", ErrInvalid
	}
	end, err := time.Parse("2006-01-02", input.EndDate)
	if err != nil || end.Format("2006-01-02") != input.EndDate || !end.After(start) {
		return Input{}, "", ErrInvalid
	}
	// Treat a hyphen and an en dash as the same name while preserving the label entered.
	return input, strings.ReplaceAll(input.Name, "–", "-"), nil
}
func (s *Service) Create(ctx context.Context, actor string, input Input) (Session, error) {
	input, key, err := normalize(input)
	if !uuid.MatchString(actor) || err != nil {
		return Session{}, ErrInvalid
	}
	return s.repo.Create(ctx, actor, input, key)
}
func (s *Service) List(ctx context.Context, q Query) (Page, error) {
	q.Search = strings.TrimSpace(q.Search)
	if q.Page < 1 || q.Page > 100000 || q.PageSize < 1 || q.PageSize > 100 || len(q.Search) > 100 {
		return Page{}, ErrInvalid
	}
	return s.repo.List(ctx, q)
}
func (s *Service) Get(ctx context.Context, id string) (Session, error) {
	if !uuid.MatchString(id) {
		return Session{}, ErrInvalid
	}
	return s.repo.Get(ctx, id)
}
func (s *Service) Update(ctx context.Context, actor, id string, input Input) (Session, error) {
	input, _, err := normalize(input)
	if !uuid.MatchString(actor) || !uuid.MatchString(id) || err != nil {
		return Session{}, ErrInvalid
	}
	return s.repo.Update(ctx, actor, id, input)
}
func (s *Service) SetActive(ctx context.Context, actor, id string, active bool) (Session, error) {
	if !uuid.MatchString(actor) || !uuid.MatchString(id) {
		return Session{}, ErrInvalid
	}
	return s.repo.SetActive(ctx, actor, id, active)
}
func (s *Service) Current(ctx context.Context) (Session, error) { return s.repo.Current(ctx) }
