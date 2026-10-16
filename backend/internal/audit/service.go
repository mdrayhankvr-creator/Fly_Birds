package audit

import "context"

type Reader interface {
	List(context.Context, Query) (Page, error)
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func (s *Service) List(ctx context.Context, query Query) (Page, error) {
	if query.Page < 1 || query.Page > 100000 || query.PageSize < 1 || query.PageSize > 100 ||
		(query.Action != "" && !safeFilter.MatchString(query.Action)) ||
		(query.TargetType != "" && !safeFilter.MatchString(query.TargetType)) ||
		(query.Actor != "" && !uuid.MatchString(query.Actor)) {
		return Page{}, ErrInvalid
	}
	return s.reader.List(ctx, query)
}
