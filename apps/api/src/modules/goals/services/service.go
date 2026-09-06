package services

import (
	"context"
	"time"

	"ledgermeadow/src/modules/goals/models"
	"ledgermeadow/src/modules/goals/validators"
	shared "ledgermeadow/src/shared/types"
)

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Goal, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) List(ctx context.Context, userID shared.UserID) ([]models.Goal, error) {
	return s.repository.List(ctx, userID)
}
func (s *Service) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	if err := validators.Create(command, time.Now().UTC().Truncate(24*time.Hour)); err != nil {
		return "", err
	}
	return s.repository.Create(ctx, userID, command)
}
