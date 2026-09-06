package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	analyticsmodels "ledgermeadow/src/modules/analytics/models"
	"ledgermeadow/src/modules/materialization/models"
	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	shared "ledgermeadow/src/shared/types"
)

func TestMaterializePersistsEveryDashboardSnapshot(t *testing.T) {
	asOf := time.Date(2026, time.August, 23, 0, 0, 0, 0, time.UTC)
	repository := &fakeRepository{input: models.Input{
		AsOfDate: asOf, PeriodStart: time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd: time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC),
		Currencies: []models.CurrencyInput{{
			Currency: "CAD", LiquidBalanceMinor: 50_000,
			Assets: []models.Item{{ID: "account:1", Label: "Cash", AmountMinor: 50_000}},
			ProjectionSources: []models.ProjectionSource{{
				ID: "bill:1", Name: "Rent", Kind: "BILL", AmountMinor: -10_000,
				FirstDate: time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC), Frequency: "MONTHLY",
			}},
			Actual: models.CashFlowComponents{
				IncomeMinor: 20_000, IncomeCount: 1, VariableOutflowMinor: 5_000, OutflowCount: 2,
			},
			Analytics: map[string][]models.AnalyticsBucket{
				"SPENDING":   {{ID: "uncategorized", Label: "Uncategorized", AmountMinor: 5_000, ItemCount: 2}},
				"CATEGORIES": {{ID: "uncategorized", Label: "Uncategorized", AmountMinor: 5_000, ItemCount: 2}},
			},
		}},
	}}
	service := New(repository, fakeEngine{})
	service.now = func() time.Time { return asOf }

	if err := service.Materialize(context.Background(), shared.UserID("00000000-0000-0000-0000-000000000001")); err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if !repository.persisted {
		t.Fatal("Materialize() did not persist its result")
	}
	if len(repository.result.Currencies) != 1 {
		t.Fatalf("currency result count = %d, want 1", len(repository.result.Currencies))
	}
	result := repository.result.Currencies[0]
	if result.ProjectionStatus != "ON_TRACK" || result.ProjectedMonthEndMinor != 40_000 {
		t.Fatalf("projection result = status %q, month end %d", result.ProjectionStatus, result.ProjectedMonthEndMinor)
	}
	if len(result.Analytics) != len(analyticsViews) {
		t.Fatalf("analytics view count = %d, want %d", len(result.Analytics), len(analyticsViews))
	}
	var spending []analyticsmodels.BreakdownItem
	if err := json.Unmarshal(result.Analytics[0].BreakdownJSON, &spending); err != nil {
		t.Fatalf("decode persisted analytics = %v", err)
	}
	if len(spending) != 1 || int64(spending[0].AmountMinor) != 5_000 {
		t.Fatalf("persisted spending breakdown = %#v", spending)
	}
}

func TestPayCycleProtectionCoversProjectionHorizon(t *testing.T) {
	// These invented events span both sides of a synthetic pay date.
	events := []*financialenginepb.TimelineEvent{
		{SourceId: "synthetic-projection-before", Name: "Synthetic Earlier Subscription", Kind: financialenginepb.ProjectionKind_PROJECTION_KIND_SUBSCRIPTION, AmountMinor: -7, Date: &financialenginepb.Date{Year: 2000, Month: 1, Day: 2}},
		{SourceId: "synthetic-projection-bill", Name: "Synthetic Scheduled Bill", Kind: financialenginepb.ProjectionKind_PROJECTION_KIND_BILL, AmountMinor: -11, Date: &financialenginepb.Date{Year: 2000, Month: 1, Day: 3}},
		{SourceId: "synthetic-projection-payday", Name: "Synthetic Pay Event", Kind: financialenginepb.ProjectionKind_PROJECTION_KIND_SALARY, AmountMinor: 101, Date: &financialenginepb.Date{Year: 2000, Month: 1, Day: 4}},
		{SourceId: "synthetic-projection-after", Name: "Synthetic Later Subscription", Kind: financialenginepb.ProjectionKind_PROJECTION_KIND_SUBSCRIPTION, AmountMinor: -13, Date: &financialenginepb.Date{Year: 2000, Month: 1, Day: 5}},
	}

	items, expectedIncome, err := payCycleProtection(events)
	if err != nil {
		t.Fatalf("payCycleProtection() error = %v", err)
	}
	if expectedIncome != 101 {
		t.Fatalf("expected income = %d", expectedIncome)
	}
	if len(items) != 3 {
		t.Fatalf("protected item count = %d", len(items))
	}
	if items[0].Id != "synthetic-projection-before" || items[1].Id != "synthetic-projection-bill" || items[2].Id != "synthetic-projection-after" {
		t.Fatalf("protected items = %#v", items)
	}
}

type fakeRepository struct {
	input     models.Input
	result    models.Result
	persisted bool
}

func (r *fakeRepository) Load(context.Context, shared.UserID, time.Time) (models.Input, error) {
	return r.input, nil
}

func (r *fakeRepository) Persist(_ context.Context, _ shared.UserID, result models.Result) error {
	r.result = result
	r.persisted = true
	return nil
}

type fakeEngine struct{}

func (fakeEngine) BuildProjection(
	_ context.Context,
	request *financialenginepb.BuildProjectionRequest,
) (*financialenginepb.BuildProjectionResponse, error) {
	source := request.Sources[0]
	return &financialenginepb.BuildProjectionResponse{
		Currency: request.Currency, EndingBalanceMinor: 40_000, MinimumBalanceMinor: 40_000,
		MinimumBalanceDate: source.FirstDate, Status: financialenginepb.ProjectionStatus_PROJECTION_STATUS_ON_TRACK,
		Events: []*financialenginepb.TimelineEvent{{
			SourceId: source.SourceId, Occurrence: 1, Date: source.FirstDate, Kind: source.Kind,
			Name: source.Name, AmountMinor: source.AmountMinor,
			BalanceBeforeMinor: request.StartingBalanceMinor, BalanceAfterMinor: 40_000,
		}},
	}, nil
}

func (fakeEngine) CalculateAvailableToSpend(
	_ context.Context,
	request *financialenginepb.CalculateAvailableToSpendRequest,
) (*financialenginepb.CalculateAvailableToSpendResponse, error) {
	return &financialenginepb.CalculateAvailableToSpendResponse{
		Currency: request.Currency, TotalMinor: request.LiquidBalanceMinor,
		ProtectedMinor: 10_000, AvailableMinor: 40_000,
		Status: request.ProjectionStatus,
		Breakdown: []*financialenginepb.AvailableComponent{
			{Id: "liquid_balance", Label: "Liquid balance", Kind: financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_LIQUID_BALANCE, AmountMinor: 50_000, EffectMinor: 50_000},
			{Id: "bill:1", Label: "Rent", Kind: financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_UPCOMING_BILL, AmountMinor: 10_000, EffectMinor: -10_000},
		},
	}, nil
}

func (fakeEngine) CalculateCashFlow(
	_ context.Context,
	request *financialenginepb.CalculateCashFlowRequest,
) (*financialenginepb.CalculateCashFlowResponse, error) {
	summarize := func(value *financialenginepb.CashFlowComponents) *financialenginepb.CashFlowSummary {
		outflow := value.FixedOutflowMinor + value.VariableOutflowMinor + value.SubscriptionsMinor + value.SavingsMinor
		return &financialenginepb.CashFlowSummary{
			IncomeMinor: value.IncomeMinor, FixedOutflowMinor: value.FixedOutflowMinor,
			VariableOutflowMinor: value.VariableOutflowMinor, SubscriptionsMinor: value.SubscriptionsMinor,
			SavingsMinor: value.SavingsMinor, TotalOutflowMinor: outflow, NetMinor: value.IncomeMinor - outflow,
		}
	}
	return &financialenginepb.CalculateCashFlowResponse{
		Currency: request.Currency, Actual: summarize(request.Actual), Projected: summarize(request.Projected),
	}, nil
}

func (fakeEngine) CalculateAnalyticsBreakdown(
	_ context.Context,
	request *financialenginepb.CalculateAnalyticsBreakdownRequest,
) (*financialenginepb.CalculateAnalyticsBreakdownResponse, error) {
	response := &financialenginepb.CalculateAnalyticsBreakdownResponse{Currency: request.Currency}
	for _, bucket := range request.Buckets {
		response.TotalMinor += bucket.AmountMinor
		response.Buckets = append(response.Buckets, &financialenginepb.AnalyticsBreakdownItem{
			Id: bucket.Id, Label: bucket.Label, AmountMinor: bucket.AmountMinor,
			ItemCount: bucket.ItemCount, ShareBasisPoints: 10_000,
		})
	}
	return response, nil
}

func (fakeEngine) CalculateNetWorth(
	_ context.Context,
	request *financialenginepb.CalculateNetWorthRequest,
) (*financialenginepb.CalculateNetWorthResponse, error) {
	return &financialenginepb.CalculateNetWorthResponse{
		Currency: request.Currency, TotalAssetsMinor: 50_000, NetWorthMinor: 50_000,
	}, nil
}
