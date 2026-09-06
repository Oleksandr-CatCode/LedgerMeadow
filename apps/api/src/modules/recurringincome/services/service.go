package services

import (
	"context"
	"errors"

	"ledgermeadow/src/modules/recurringincome/models"
	"ledgermeadow/src/modules/recurringincome/validators"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalidID = errors.New("invalid recurring income id")

type Repository interface {
	List(context.Context, shared.UserID) ([]models.RecurringIncome, error)
	Detail(context.Context, shared.UserID, string) (models.RecurringIncome, error)
	Update(context.Context, shared.UserID, string, models.Update) error
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, userID shared.UserID) ([]models.RecurringIncome, error) {
	return s.repository.List(ctx, userID)
}

func (s *Service) Detail(ctx context.Context, userID shared.UserID, id string) (models.RecurringIncome, error) {
	if !validate.UUID(id) {
		return models.RecurringIncome{}, ErrInvalidID
	}
	return s.repository.Detail(ctx, userID, id)
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
