package services

import (
	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	"ledgermeadow/src/modules/subscriptions/models"
	"ledgermeadow/src/modules/subscriptions/validators"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
	"context"
	"errors"
	"fmt"
)

var ErrInvalidID = errors.New("invalid subscription id")

type Repository interface {
	List(context.Context, shared.UserID) ([]models.Subscription, error)
	Detail(context.Context, shared.UserID, string) (models.DetailSource, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
	Reclassify(context.Context, shared.UserID, string) error
}

type Engine interface {
	CalculateSubscriptionSummary(context.Context, *financialenginepb.CalculateSubscriptionSummaryRequest) (*financialenginepb.CalculateSubscriptionSummaryResponse, error)
}

func (s *Service) Detail(ctx context.Context, userID shared.UserID, id string) (models.Detail, error) {
	if !validate.UUID(id) {
		return models.Detail{}, ErrInvalidID
	}
	source, err := s.repository.Detail(ctx, userID, id)
	if err != nil {
		return models.Detail{}, err
	}
	currency, ok := financialenginepb.Currency_value["CURRENCY_"+source.Currency]
	if !ok || currency == int32(financialenginepb.Currency_CURRENCY_UNSPECIFIED) {
		return models.Detail{}, errors.New("stored subscription currency is invalid")
	}
	frequency, ok := financialenginepb.Frequency_value["FREQUENCY_"+source.Frequency]
	if !ok || frequency == int32(financialenginepb.Frequency_FREQUENCY_UNSPECIFIED) {
		return models.Detail{}, errors.New("stored subscription frequency is invalid")
	}
	if len(source.Payments) > models.DetailPaymentLimit {
		return models.Detail{}, errors.New("subscription payment input exceeds bound")
	}
	amounts := make([]int64, len(source.Payments))
	for index, payment := range source.Payments {
		amounts[index] = payment.AmountMinor
	}
	response, err := s.engine.CalculateSubscriptionSummary(ctx, &financialenginepb.CalculateSubscriptionSummaryRequest{
		Currency:                financialenginepb.Currency(currency),
		ExpectedAmountMinor:     int64(source.ExpectedAmountMinor),
		Frequency:               financialenginepb.Frequency(frequency),
		TransactionAmountsMinor: amounts,
	})
	if err != nil {
		return models.Detail{}, fmt.Errorf("calculate subscription summary: %w", err)
	}
	if response == nil || response.Currency != financialenginepb.Currency(currency) || response.AnnualCostMinor < 0 || response.PaidThisYearMinor < 0 || len(response.PaymentAmountsMinor) != len(source.Payments) {
		return models.Detail{}, errors.New("financial engine returned an invalid subscription summary")
	}
	historyCount := min(len(source.Payments), models.PaymentHistoryLimit)
	history := make([]models.Payment, historyCount)
	for index := range historyCount {
		if response.PaymentAmountsMinor[index] < 0 {
			return models.Detail{}, errors.New("financial engine returned an invalid subscription payment")
		}
		history[index] = models.Payment{
			PaidAt:      source.Payments[index].PaidAt,
			AmountMinor: shared.MinorUnits(response.PaymentAmountsMinor[index]),
		}
	}
	return models.Detail{
		Subscription:      source.Subscription,
		AnnualCostMinor:   shared.MinorUnits(response.AnnualCostMinor),
		PaidThisYearMinor: shared.MinorUnits(response.PaidThisYearMinor),
		PaymentHistory:    history,
	}, nil
}

func (s *Service) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	if !validate.UUID(id) {
		return ErrInvalidID
	}
	if err := validators.Update(command); err != nil {
		return err
	}
	if command.Reclassify != "" {
		return s.repository.Reclassify(ctx, userID, id)
	}
	return s.repository.Update(ctx, userID, id, command)
}

type Service struct {
	repository Repository
	engine     Engine
}

func New(r Repository, engine Engine) *Service { return &Service{repository: r, engine: engine} }
func (s *Service) List(c context.Context, u shared.UserID) ([]models.Subscription, error) {
	return s.repository.List(c, u)
}
func (s *Service) Create(c context.Context, u shared.UserID, v models.Create) (string, error) {
	if e := validators.Create(v); e != nil {
		return "", e
	}
	return s.repository.Create(c, u, v)
}
