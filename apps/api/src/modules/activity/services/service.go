package services

import (
	"context"

	"ledgermeadow/src/modules/activity/models"
	shared "ledgermeadow/src/shared/types"
)

type Repository interface {
	Get(context.Context, shared.UserID) (models.Summary, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) Get(ctx context.Context, userID shared.UserID) (models.Summary, error) {
	return s.repository.Get(ctx, userID)
}
