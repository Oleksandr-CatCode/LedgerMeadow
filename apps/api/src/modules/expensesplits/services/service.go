package services

import (
	"context"

	"ledgermeadow/src/modules/expensesplits/models"
	"ledgermeadow/src/modules/expensesplits/validators"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
)

type Repository interface {
	Get(context.Context, shared.UserID, shared.TransactionID) (models.Split, error)
	Replace(context.Context, shared.UserID, shared.TransactionID, models.Replace) (models.Split, error)
	Clear(context.Context, shared.UserID, shared.TransactionID) error
}

func (s *Service) Get(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (models.Split, error) {
	if !validate.UUID(string(transactionID)) {
		return models.Split{}, validators.ErrInvalid
	}
	return s.repository.Get(ctx, userID, transactionID)
}

type Service struct {
	repository Repository
}

func New(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Replace(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
	command models.Replace,
) (models.Split, error) {
	if !validate.UUID(string(transactionID)) {
		return models.Split{}, validators.ErrInvalid
	}
	if err := validators.Replace(command); err != nil {
		return models.Split{}, err
	}
	return s.repository.Replace(ctx, userID, transactionID, command)
}

func (s *Service) Clear(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) error {
	if !validate.UUID(string(transactionID)) {
		return validators.ErrInvalid
	}
	return s.repository.Clear(ctx, userID, transactionID)
}
