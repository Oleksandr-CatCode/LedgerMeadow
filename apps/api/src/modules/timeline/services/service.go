package services

import (
	"ledgermeadow/src/modules/timeline/models"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	Get(context.Context, shared.UserID) ([]models.Snapshot, error)
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) Get(c context.Context, u shared.UserID) ([]models.Snapshot, error) {
	return s.repository.Get(c, u)
}
