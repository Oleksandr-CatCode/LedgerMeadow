package services

import (
	"ledgermeadow/src/modules/dashboard/models"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	Get(context.Context, shared.UserID) (models.Dashboard, error)
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) Get(c context.Context, u shared.UserID) (models.Dashboard, error) {
	dashboard, err := s.repository.Get(c, u)
	if err != nil {
		return models.Dashboard{}, err
	}
	if dashboard.Projections == nil {
		dashboard.Projections = make([]models.Projection, 0)
	}
	if dashboard.Spaces == nil {
		dashboard.Spaces = make([]models.Space, 0)
	}
	if dashboard.Budgets == nil {
		dashboard.Budgets = make([]models.Budget, 0)
	}
	if dashboard.MonthlyPlans == nil {
		dashboard.MonthlyPlans = make([]models.MonthlyPlan, 0)
	}
	if dashboard.Upcoming == nil {
		dashboard.Upcoming = make([]models.Upcoming, 0)
	}
	if dashboard.RecentTransactions == nil {
		dashboard.RecentTransactions = make([]models.Transaction, 0)
	}
	return dashboard, nil
}
