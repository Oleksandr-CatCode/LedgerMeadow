package services

import (
	"ledgermeadow/src/modules/inbox/models"
	"ledgermeadow/src/modules/inbox/validators"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	List(context.Context, shared.UserID, string, int) (models.Page, error)
	Resolve(context.Context, shared.UserID, string, string) error
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) List(c context.Context, u shared.UserID, k string, l int) (models.Page, error) {
	return s.repository.List(c, u, k, l)
}
func (s *Service) Resolve(c context.Context, u shared.UserID, id, resolution string) error {
	if e := validators.Resolve(id, resolution); e != nil {
		return e
	}
	return s.repository.Resolve(c, u, id, resolution)
}
