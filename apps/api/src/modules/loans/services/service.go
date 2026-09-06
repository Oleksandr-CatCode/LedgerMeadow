package services

import (
	"ledgermeadow/src/modules/loans/models"
	"ledgermeadow/src/modules/loans/validators"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
	"context"
	"errors"
)

var ErrInvalidID = errors.New("invalid loan id")
var ErrEngineUnavailable = errors.New("financial engine unavailable")

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Loan, error)
	Detail(context.Context, shared.UserID, string) (models.Detail, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
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

func (s *Service) Create(c context.Context, u shared.UserID, command models.Create) (string, error) {
	if err := validators.Create(command); err != nil {
		return "", err
	}
	return s.repository.Create(c, u, command)
}

type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) List(c context.Context, u shared.UserID) ([]models.Loan, error) {
	return s.repository.List(c, u)
}
func (s *Service) Detail(c context.Context, u shared.UserID, id string) (models.Detail, error) {
	if !validate.UUID(id) {
		return models.Detail{}, ErrInvalidID
	}
	return s.repository.Detail(c, u, id)
}
func (s *Service) Scenario(ctx context.Context, userID shared.UserID, id string, input models.ScenarioInput) (models.Scenario, error) {
	if !validate.UUID(id) {
		return models.Scenario{}, ErrInvalidID
	}
	if err := validators.Scenario(input); err != nil {
		return models.Scenario{}, err
	}
	if _, err := s.repository.Detail(ctx, userID, id); err != nil {
		return models.Scenario{}, err
	}
	return models.Scenario{}, ErrEngineUnavailable
}
