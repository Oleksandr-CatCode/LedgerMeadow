package services

import (
	"ledgermeadow/src/modules/budgets/models"
	"ledgermeadow/src/modules/budgets/validators"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Budget, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) List(c context.Context, u shared.UserID) ([]models.Budget, error) {
	return s.repository.List(c, u)
}
func (s *Service) Create(c context.Context, u shared.UserID, v models.Create) (string, error) {
	if e := validators.Create(v); e != nil {
		return "", e
	}
	return s.repository.Create(c, u, v)
}
