package services

import (
	"context"
	"errors"
	"testing"

	"ledgermeadow/src/modules/planning/models"
	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	shared "ledgermeadow/src/shared/types"
)

func TestGetCalculatesBoundedPlanningSummary(t *testing.T) {
	engine := &fakeEngine{}
	service := New(fakeRepository{workspace: planningWorkspace()}, engine)

	workspace, err := service.Get(context.Background(), shared.UserID("owner"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(engine.requests) != 1 || len(engine.requests[0].Items) != 3 || len(engine.requests[0].Allocations) != 1 {
		t.Fatalf("engine requests = %#v", engine.requests)
	}
	if len(workspace.Summaries) != 1 || workspace.Summaries[0].UnallocatedMinor != 235_000 {
		t.Fatalf("planning summaries = %#v", workspace.Summaries)
	}
	if workspace.Summaries[0].Allocations[0].ID != "budget-1" {
		t.Fatalf("planning allocations = %#v", workspace.Summaries[0].Allocations)
	}
}

func TestGetRejectsUntrustedPlanningAllocation(t *testing.T) {
	engine := &fakeEngine{spoofAllocation: true}
	service := New(fakeRepository{workspace: planningWorkspace()}, engine)

	_, err := service.Get(context.Background(), shared.UserID("owner"))
	if !errors.Is(err, ErrEngineResponse) {
		t.Fatalf("Get() error = %v, want ErrEngineResponse", err)
	}
}

type fakeRepository struct{ workspace models.Workspace }

func (repository fakeRepository) Get(context.Context, shared.UserID) (models.Workspace, error) {
	return repository.workspace, nil
}

type fakeEngine struct {
	requests        []*financialenginepb.CalculatePlanningSummaryRequest
	spoofAllocation bool
}

func (engine *fakeEngine) CalculatePlanningSummary(
	_ context.Context,
	request *financialenginepb.CalculatePlanningSummaryRequest,
) (*financialenginepb.CalculatePlanningSummaryResponse, error) {
	engine.requests = append(engine.requests, request)
	allocationID := request.Allocations[0].Id
	if engine.spoofAllocation {
		allocationID = "foreign-budget"
	}
	return &financialenginepb.CalculatePlanningSummaryResponse{
		Currency:            request.Currency,
		ExpectedIncomeMinor: 550_000, CommittedBillsMinor: 180_000,
		CommittedSubscriptionsMinor: 35_000, PlannedMinor: 100_000,
		UnallocatedMinor: 235_000, RecurringBillsMonthlyMinor: 180_000,
		RecurringSubscriptionsMonthlyMinor: 35_000, RecurringMonthlyMinor: 215_000,
		RecurringSharePercent: 39, Next_7DaysMinor: 35_000, Next_30DaysMinor: 215_000,
		AnnualCostMinor: 2_580_000,
		Allocations: []*financialenginepb.PlanningAllocationAmount{{
			Id: allocationID, Name: request.Allocations[0].Name, AmountMinor: 100_000,
		}},
	}, nil
}

func planningWorkspace() models.Workspace {
	return models.Workspace{
		AsOfDate: "2000-08-24", PlanStart: "2000-09-01", PlanEnd: "2000-09-30",
		RecurringIncome: []models.RecurringIncome{{
			ID: "income-1", Name: "Salary", ExpectedAmountMinor: 550_000, Currency: "CAD",
			Frequency: "MONTHLY", NextExpectedAt: "2000-09-01", Status: "ACTIVE",
		}},
		Bills: []models.Bill{{
			ID: "bill-1", Name: "Rent", ExpectedAmountMinor: 180_000, Currency: "CAD",
			Frequency: "MONTHLY", NextDueAt: "2000-09-01", Status: "ACTIVE",
		}},
		Subscriptions: []models.Subscription{{
			ID: "subscription-1", MerchantName: "Synthetic Subscription Service", ExpectedAmountMinor: 3_500, Currency: "CAD",
			Frequency: "MONTHLY", NextExpectedAt: "2000-08-24", Status: "ACTIVE",
		}},
		Budgets: []models.Budget{{
			ID: "budget-1", Name: "Daily", Period: "MONTHLY", LimitMinor: 100_000, Currency: "CAD",
		}},
	}
}
