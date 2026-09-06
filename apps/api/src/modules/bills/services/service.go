package services

import (
	"ledgermeadow/src/modules/bills/models"
	"ledgermeadow/src/modules/bills/validators"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
	"context"
	"errors"
)

var ErrInvalidID = errors.New("invalid bill id")

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Bill, error)
	Detail(context.Context, shared.UserID, string) (models.Bill, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
	Reclassify(context.Context, shared.UserID, string) error
}

func (s *Service) Detail(c context.Context, u shared.UserID, id string) (models.Bill, error) {
	if !validate.UUID(id) {
		return models.Bill{}, ErrInvalidID
	}
	return s.repository.Detail(c, u, id)
}
func (s *Service) Update(c context.Context, u shared.UserID, id string, v models.Update) error {
	if !validate.UUID(id) {
		return ErrInvalidID
	}
	if err := validators.Update(v); err != nil {
		return err
	}
	if v.Reclassify != "" {
		return s.repository.Reclassify(c, u, id)
	}
	return s.repository.Update(c, u, id, v)
}

type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) List(c context.Context, u shared.UserID) ([]models.Bill, error) {
	return s.repository.List(c, u)
}
func (s *Service) Create(c context.Context, u shared.UserID, v models.Create) (string, error) {
	if err := validators.Create(v); err != nil {
		return "", err
	}
	return s.repository.Create(c, u, v)
}
