package dashboard

import (
	"context"

	"github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/auth"
)

type Reader interface {
	Get(context.Context, auth.User) (Data, error)
}

type Service struct{ reader Reader }

func NewService(reader Reader) *Service { return &Service{reader: reader} }

func (s *Service) Get(ctx context.Context, user auth.User) (Data, error) {
	if !auth.ValidRole(user.Role) || !user.IsActive {
		return Data{}, ErrForbidden
	}
	return s.reader.Get(ctx, user)
}
