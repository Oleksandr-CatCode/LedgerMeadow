package services

import (
	"ledgermeadow/src/modules/networth/models"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	Get(context.Context, shared.UserID) (models.NetWorth, error)
}
type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Get(ctx context.Context, userID shared.UserID) (models.NetWorth, error) {
	return s.repository.Get(ctx, userID)
}
