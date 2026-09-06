package services

import (
	"ledgermeadow/src/modules/notifications/models"
	"ledgermeadow/src/modules/notifications/validators"
	shared "ledgermeadow/src/shared/types"
	"context"
)

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Notification, error)
	Preferences(context.Context, shared.UserID) ([]models.Preference, error)
	UpdatePreference(context.Context, shared.UserID, models.Preference) error
	MarkRead(context.Context, shared.UserID, string) error
}
type Service struct{ repository Repository }

func New(r Repository) *Service { return &Service{repository: r} }
func (s *Service) List(c context.Context, u shared.UserID) ([]models.Notification, error) {
	return s.repository.List(c, u)
}
func (s *Service) Preferences(c context.Context, u shared.UserID) ([]models.Preference, error) {
	return s.repository.Preferences(c, u)
}
func (s *Service) UpdatePreference(c context.Context, u shared.UserID, p models.Preference) error {
	if e := validators.Preference(p.Type); e != nil {
		return e
	}
	return s.repository.UpdatePreference(c, u, p)
}
func (s *Service) MarkRead(c context.Context, u shared.UserID, id string) error {
	if e := validators.NotificationID(id); e != nil {
		return e
	}
	return s.repository.MarkRead(c, u, id)
}
