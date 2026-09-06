package services

import (
	"context"

	"ledgermeadow/src/modules/manualliabilities/models"
	"ledgermeadow/src/modules/manualliabilities/validators"
	shared "ledgermeadow/src/shared/types"
)

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Liability, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, userID shared.UserID) ([]models.Liability, error) {
	return s.repository.List(ctx, userID)
}

func (s *Service) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	if err := validators.Create(command); err != nil {
		return "", err
	}
	return s.repository.Create(ctx, userID, command)
}
