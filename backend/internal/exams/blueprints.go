package exams

import "github.com/mdrayhankvr-creator/Fly_Birds/backend/internal/blueprints"

// Blueprints returns the terms and blueprints service over the same
// database, or nil when the exams service has no database.
func (s *Service) Blueprints() *blueprints.Service {
	if r, ok := s.repo.(*GormRepository); ok && r.db != nil {
		return blueprints.NewService(blueprints.NewRepository(r.db))
	}
	return nil
}
