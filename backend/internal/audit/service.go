package audit

import (
	"context"
	"time"
)

type Reader interface {
	List(context.Context, Query) (Page, error)
	Actions(context.Context) ([]ActionCount, error)
	Denied(context.Context, *string, string, string, string) error
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func (s *Service) List(ctx context.Context, query Query) (Page, error) {
	if query.Page < 1 || query.Page > 100000 || query.PageSize < 1 || query.PageSize > 100 ||
		(query.Action != "" && !safeFilter.MatchString(query.Action)) ||
		(query.TargetType != "" && !safeFilter.MatchString(query.TargetType)) ||
		(query.Actor != "" && !uuid.MatchString(query.Actor)) ||
		(query.Outcome != "" && query.Outcome != "success" && query.Outcome != "denied") ||
		len(query.TargetID) > 200 || len(query.Search) > 100 || !validDay(query.From) || !validDay(query.To) ||
		(query.From != "" && query.To != "" && query.From > query.To) {
		return Page{}, ErrInvalid
	}
	return s.reader.List(ctx, query)
}

func (s *Service) Actions(ctx context.Context) ([]ActionCount, error) { return s.reader.Actions(ctx) }

// Denied records a refused administrative request without failing it.
func (s *Service) Denied(ctx context.Context, actor *string, method, route, reason string) {
	_ = s.reader.Denied(ctx, actor, method, route, reason)
}

func validDay(v string) bool {
	if v == "" {
		return true
	}
	t, err := time.Parse(time.DateOnly, v)
	return err == nil && t.Format(time.DateOnly) == v
}
