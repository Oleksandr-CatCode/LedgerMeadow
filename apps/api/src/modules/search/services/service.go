package services

import (
	"ledgermeadow/src/modules/search/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"strings"
)

var ErrInvalidQuery = errors.New("invalid search query")

type Repository interface {
	Prefix(context.Context, shared.UserID, string) ([]models.Result, error)
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) Prefix(c context.Context, u shared.UserID, q string) ([]models.Result, error) {
	n := len(strings.TrimSpace(q))
	if n < 2 || n > 80 {
		return nil, ErrInvalidQuery
	}
	return s.repository.Prefix(c, u, q)
}
