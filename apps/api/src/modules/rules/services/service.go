package services

import (
	"context"
	"errors"

	"ledgermeadow/src/modules/rules/models"
	"ledgermeadow/src/modules/rules/validators"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalidID = errors.New("invalid rule id")
var ErrInvalidOrder = errors.New("invalid rule order")

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Rule, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
	Delete(context.Context, shared.UserID, string) error
	Duplicate(context.Context, shared.UserID, string) (string, error)
	Reorder(context.Context, shared.UserID, []string) error
}

func (s *Service) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	if !validate.UUID(id) {
		return ErrInvalidID
	}
	if err := validators.Update(command); err != nil {
		return err
	}
	return s.repository.Update(ctx, userID, id, command)
}

func (s *Service) Delete(ctx context.Context, userID shared.UserID, id string) error {
	if !validate.UUID(id) {
		return ErrInvalidID
	}
	return s.repository.Delete(ctx, userID, id)
}

func (s *Service) Duplicate(ctx context.Context, userID shared.UserID, id string) (string, error) {
	if !validate.UUID(id) {
		return "", ErrInvalidID
	}
	return s.repository.Duplicate(ctx, userID, id)
}

func (s *Service) Reorder(ctx context.Context, userID shared.UserID, orderedIDs []string) error {
	if len(orderedIDs) < 1 || len(orderedIDs) > 100 {
		return ErrInvalidOrder
	}
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if !validate.UUID(id) {
			return ErrInvalidOrder
		}
		if _, exists := seen[id]; exists {
			return ErrInvalidOrder
		}
		seen[id] = struct{}{}
	}
	return s.repository.Reorder(ctx, userID, orderedIDs)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) List(ctx context.Context, userID shared.UserID) ([]models.Rule, error) {
	return s.repository.List(ctx, userID)
}
func (s *Service) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	if err := validators.Create(command); err != nil {
		return "", err
	}
	return s.repository.Create(ctx, userID, command)
}
