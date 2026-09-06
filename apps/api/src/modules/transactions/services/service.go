package services

import (
	"context"
	"errors"

	transactionrepo "ledgermeadow/src/modules/transactions/repository"
	transactions "ledgermeadow/src/modules/transactions/types"
	"ledgermeadow/src/modules/transactions/validators"
	shared "ledgermeadow/src/shared/types"
)

type Repository interface {
	List(ctx context.Context, userID shared.UserID, cursor string, limit int, filters transactions.ListFilters) (transactionrepo.Page, error)
	Detail(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID) (transactions.Detail, error)
	Update(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID, update transactions.Update) error
	BulkUpdate(ctx context.Context, userID shared.UserID, command transactions.BulkUpdate) error
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, userID shared.UserID, cursor string, limit int, filters transactions.ListFilters) (transactionrepo.Page, error) {
	if err := validators.Filters(filters); err != nil {
		return transactionrepo.Page{}, err
	}
	return s.repository.List(ctx, userID, cursor, limit, filters)
}

func (s *Service) BulkUpdate(ctx context.Context, userID shared.UserID, command transactions.BulkUpdate) error {
	if err := validators.Bulk(command); err != nil {
		return err
	}
	return s.repository.BulkUpdate(ctx, userID, command)
}

func (s *Service) Detail(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID) (transactions.Detail, error) {
	if err := validators.ID(string(transactionID)); err != nil {
		return transactions.Detail{}, errors.Join(validators.ErrInvalidUpdate, err)
	}
	return s.repository.Detail(ctx, userID, transactionID)
}

func (s *Service) Update(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
	update transactions.Update,
) error {
	if err := validators.ID(string(transactionID)); err != nil {
		return err
	}
	if err := validators.Update(update); err != nil {
		return err
	}
	return s.repository.Update(ctx, userID, transactionID, update)
}
