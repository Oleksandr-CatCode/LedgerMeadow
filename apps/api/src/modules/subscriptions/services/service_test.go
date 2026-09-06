package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	"ledgermeadow/src/modules/subscriptions/models"
	shared "ledgermeadow/src/shared/types"
)

func TestDetailReturnsBoundedNormalizedHistoryWithoutInternalLinkage(t *testing.T) {
	source := models.DetailSource{
		Subscription: models.Subscription{
			ID:                  "00000000-0000-4000-8000-000000000042",
			MerchantName:        "Synthetic Subscription Service",
			ExpectedAmountMinor: 123,
			Currency:            "CAD",
			Frequency:           "MONTHLY",
			NextExpectedAt:      "2000-06-01",
			Status:              "ACTIVE",
			Source:              "DETECTED",
		},
		Payments: []models.PaymentSource{
			{PaidAt: "2000-05-01", AmountMinor: -123},
			{PaidAt: "2000-04-01", AmountMinor: -123},
			{PaidAt: "2000-03-01", AmountMinor: -123},
			{PaidAt: "2000-02-01", AmountMinor: -123},
			{PaidAt: "2000-01-01", AmountMinor: -123},
		},
	}
	service := New(detailRepository{source: source}, detailEngine{})

	detail, err := service.Detail(context.Background(), shared.UserID("user-id"), source.ID)
	if err != nil {
		t.Fatalf("load detail: %v", err)
	}
	if detail.AnnualCostMinor != 1_476 || detail.PaidThisYearMinor != 615 {
		t.Fatalf("subscription totals = %d/%d", detail.AnnualCostMinor, detail.PaidThisYearMinor)
	}
	if len(detail.PaymentHistory) != models.PaymentHistoryLimit {
		t.Fatalf("history length = %d, want %d", len(detail.PaymentHistory), models.PaymentHistoryLimit)
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	if strings.Contains(string(encoded), "detection_key") || strings.Contains(string(encoded), "-123") {
		t.Fatalf("detail exposed internal linkage or raw signed amounts: %s", encoded)
	}
}

func TestDetailRejectsInvalidID(t *testing.T) {
	service := New(detailRepository{}, detailEngine{})

	if _, err := service.Detail(context.Background(), shared.UserID("user-id"), "not-a-uuid"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("detail error = %v, want ErrInvalidID", err)
	}
}

type detailRepository struct {
	source models.DetailSource
}

func (r detailRepository) List(context.Context, shared.UserID) ([]models.Subscription, error) {
	return nil, nil
}

func (r detailRepository) Detail(context.Context, shared.UserID, string) (models.DetailSource, error) {
	return r.source, nil
}

func (r detailRepository) Create(context.Context, shared.UserID, models.Create) (string, error) {
	return "", nil
}

func (r detailRepository) Update(context.Context, shared.UserID, string, models.Update) error {
	return nil
}

func (r detailRepository) Reclassify(context.Context, shared.UserID, string) error {
	return nil
}

type detailEngine struct{}

func (detailEngine) CalculateSubscriptionSummary(
	_ context.Context,
	request *financialenginepb.CalculateSubscriptionSummaryRequest,
) (*financialenginepb.CalculateSubscriptionSummaryResponse, error) {
	amounts := make([]int64, len(request.TransactionAmountsMinor))
	for index := range request.TransactionAmountsMinor {
		amounts[index] = 123
	}
	return &financialenginepb.CalculateSubscriptionSummaryResponse{
		Currency:            request.Currency,
		AnnualCostMinor:     1_476,
		PaidThisYearMinor:   615,
		PaymentAmountsMinor: amounts,
	}, nil
}
