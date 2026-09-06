package services

import (
	"context"

	"ledgermeadow/src/modules/spaces/models"
	"ledgermeadow/src/modules/spaces/validators"
	shared "ledgermeadow/src/shared/types"
)

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Space, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, userID shared.UserID) ([]models.Space, error) {
	return s.repository.List(ctx, userID)
}

func (s *Service) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	if err := validators.Create(command); err != nil {
		return "", err
	}
	return s.repository.Create(ctx, userID, command)
}
