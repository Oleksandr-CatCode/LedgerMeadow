package services

import (
	"ledgermeadow/src/modules/household/models"
	"ledgermeadow/src/modules/household/validators"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	Get(context.Context, shared.UserID) (*models.Household, error)
	Create(context.Context, shared.UserID, string) (string, error)
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) Get(c context.Context, u shared.UserID) (*models.Household, error) {
	return s.repository.Get(c, u)
}
func (s *Service) Create(c context.Context, u shared.UserID, n string) (string, error) {
	if e := validators.Name(n); e != nil {
		return "", e
	}
	return s.repository.Create(c, u, n)
}
