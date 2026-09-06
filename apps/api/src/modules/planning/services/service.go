package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ledgermeadow/src/modules/planning/models"
	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	shared "ledgermeadow/src/shared/types"
)

const planningItemLimit = 300

var ErrEngineResponse = errors.New("financial engine returned an invalid planning summary")

type Repository interface {
	Get(context.Context, shared.UserID) (models.Workspace, error)
}

type Engine interface {
	CalculatePlanningSummary(context.Context, *financialenginepb.CalculatePlanningSummaryRequest) (*financialenginepb.CalculatePlanningSummaryResponse, error)
}

type Service struct {
	repository Repository
	engine     Engine
}

func New(repository Repository, engine Engine) *Service {
	return &Service{repository: repository, engine: engine}
}

func (s *Service) Get(ctx context.Context, userID shared.UserID) (models.Workspace, error) {
	workspace, err := s.repository.Get(ctx, userID)
	if err != nil {
		return models.Workspace{}, err
	}
	asOfDate, err := engineDate(workspace.AsOfDate)
	if err != nil {
		return models.Workspace{}, err
	}
	planStart, err := engineDate(workspace.PlanStart)
	if err != nil {
		return models.Workspace{}, err
	}
	planEnd, err := engineDate(workspace.PlanEnd)
	if err != nil {
		return models.Workspace{}, err
	}
	if err := validateWorkspaceCurrencies(workspace); err != nil {
		return models.Workspace{}, err
	}

	workspace.Summaries = make([]models.Summary, 0, 2)
	for _, currency := range []string{"CAD", "USD"} {
		request, ok, err := planningRequest(workspace, currency, asOfDate, planStart, planEnd)
		if err != nil {
			return models.Workspace{}, err
		}
		if !ok {
			continue
		}
		response, err := s.engine.CalculatePlanningSummary(ctx, request)
		if err != nil {
			return models.Workspace{}, fmt.Errorf("calculate %s planning summary: %w", currency, err)
		}
		summary, err := planningSummary(response, request, workspace.PlanStart, workspace.PlanEnd)
		if err != nil {
			return models.Workspace{}, err
		}
		workspace.Summaries = append(workspace.Summaries, summary)
	}
	return workspace, nil
}

func validateWorkspaceCurrencies(workspace models.Workspace) error {
	valid := func(currency string) bool { return currency == "CAD" || currency == "USD" }
	for _, item := range workspace.Bills {
		if !valid(item.Currency) {
			return errors.New("stored planning currency is invalid")
		}
	}
	for _, item := range workspace.Subscriptions {
		if !valid(item.Currency) {
			return errors.New("stored planning currency is invalid")
		}
	}
	for _, item := range workspace.RecurringIncome {
		if !valid(item.Currency) {
			return errors.New("stored planning currency is invalid")
		}
	}
	for _, item := range workspace.Budgets {
		if !valid(item.Currency) {
			return errors.New("stored planning currency is invalid")
		}
	}
	return nil
}

func planningRequest(
	workspace models.Workspace,
	currency string,
	asOfDate *financialenginepb.Date,
	planStart *financialenginepb.Date,
	planEnd *financialenginepb.Date,
) (*financialenginepb.CalculatePlanningSummaryRequest, bool, error) {
	engineCurrency, ok := financialenginepb.Currency_value["CURRENCY_"+currency]
	if !ok || engineCurrency == int32(financialenginepb.Currency_CURRENCY_UNSPECIFIED) {
		return nil, false, errors.New("stored planning currency is invalid")
	}
	items := make([]*financialenginepb.PlanningItem, 0, planningItemLimit)
	for _, income := range workspace.RecurringIncome {
		if income.Currency != currency || income.Status != "ACTIVE" || income.ExpectedAmountMinor == 0 {
			continue
		}
		item, err := planningItem(
			"income-"+income.ID,
			income.Name,
			financialenginepb.PlanningItemKind_PLANNING_ITEM_KIND_INCOME,
			income.ExpectedAmountMinor,
			income.NextExpectedAt,
			income.Frequency,
		)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	for _, bill := range workspace.Bills {
		if bill.Currency != currency || bill.Status != "ACTIVE" || bill.ExpectedAmountMinor == 0 {
			continue
		}
		item, err := planningItem(
			"bill-"+bill.ID,
			bill.Name,
			financialenginepb.PlanningItemKind_PLANNING_ITEM_KIND_BILL,
			bill.ExpectedAmountMinor,
			bill.NextDueAt,
			bill.Frequency,
		)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	for _, subscription := range workspace.Subscriptions {
		if subscription.Currency != currency || subscription.Status != "ACTIVE" || subscription.ExpectedAmountMinor == 0 {
			continue
		}
		item, err := planningItem(
			"subscription-"+subscription.ID,
			subscription.MerchantName,
			financialenginepb.PlanningItemKind_PLANNING_ITEM_KIND_SUBSCRIPTION,
			subscription.ExpectedAmountMinor,
			subscription.NextExpectedAt,
			subscription.Frequency,
		)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if len(items) > planningItemLimit {
		return nil, false, errors.New("planning calculation input exceeds bound")
	}

	allocations := make([]*financialenginepb.PlanningAllocation, 0, len(workspace.Budgets))
	for _, budget := range workspace.Budgets {
		if budget.Currency != currency {
			continue
		}
		frequency, err := engineFrequency(budget.Period)
		if err != nil {
			return nil, false, err
		}
		allocations = append(allocations, &financialenginepb.PlanningAllocation{
			Id: budget.ID, Name: budget.Name, AmountMinor: int64(budget.LimitMinor), Frequency: frequency,
		})
	}
	if len(items) == 0 && len(allocations) == 0 {
		return nil, false, nil
	}
	return &financialenginepb.CalculatePlanningSummaryRequest{
		Currency:      financialenginepb.Currency(engineCurrency),
		AsOfDate:      asOfDate,
		PlanStartDate: planStart,
		PlanEndDate:   planEnd,
		Items:         items,
		Allocations:   allocations,
	}, true, nil
}

func planningItem(
	id string,
	name string,
	kind financialenginepb.PlanningItemKind,
	amount shared.MinorUnits,
	nextDate string,
	frequency string,
) (*financialenginepb.PlanningItem, error) {
	date, err := engineDate(nextDate)
	if err != nil {
		return nil, err
	}
	engineFrequency, err := engineFrequency(frequency)
	if err != nil {
		return nil, err
	}
	return &financialenginepb.PlanningItem{
		Id: id, Name: name, Kind: kind, ExpectedAmountMinor: int64(amount), NextDate: date, Frequency: engineFrequency,
	}, nil
}

func engineFrequency(value string) (financialenginepb.Frequency, error) {
	if value == "ANNUAL" {
		value = "ANNUALLY"
	}
	frequency, ok := financialenginepb.Frequency_value["FREQUENCY_"+value]
	if !ok || frequency == int32(financialenginepb.Frequency_FREQUENCY_UNSPECIFIED) {
		return 0, errors.New("stored planning frequency is invalid")
	}
	return financialenginepb.Frequency(frequency), nil
}

func engineDate(value string) (*financialenginepb.Date, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, errors.New("stored planning date is invalid")
	}
	return &financialenginepb.Date{Year: int32(date.Year()), Month: uint32(date.Month()), Day: uint32(date.Day())}, nil
}

func planningSummary(
	response *financialenginepb.CalculatePlanningSummaryResponse,
	request *financialenginepb.CalculatePlanningSummaryRequest,
	planStart string,
	planEnd string,
) (models.Summary, error) {
	if response == nil || response.Currency != request.Currency || response.ExpectedIncomeMinor < 0 ||
		response.CommittedBillsMinor < 0 || response.CommittedSubscriptionsMinor < 0 ||
		response.PlannedMinor < 0 || response.RecurringBillsMonthlyMinor < 0 ||
		response.RecurringSubscriptionsMonthlyMinor < 0 || response.RecurringMonthlyMinor < 0 ||
		response.Next_7DaysMinor < 0 || response.Next_30DaysMinor < 0 || response.AnnualCostMinor < 0 ||
		len(response.Allocations) != len(request.Allocations) {
		return models.Summary{}, ErrEngineResponse
	}
	expectedAllocations := make(map[string]string, len(request.Allocations))
	for _, allocation := range request.Allocations {
		expectedAllocations[allocation.Id] = allocation.Name
	}
	allocations := make([]models.PlanAllocation, len(response.Allocations))
	seen := make(map[string]struct{}, len(response.Allocations))
	for index, allocation := range response.Allocations {
		expectedName, ok := expectedAllocations[allocation.Id]
		if !ok || expectedName != allocation.Name || allocation.AmountMinor < 0 {
			return models.Summary{}, ErrEngineResponse
		}
		if _, duplicate := seen[allocation.Id]; duplicate {
			return models.Summary{}, ErrEngineResponse
		}
		seen[allocation.Id] = struct{}{}
		allocations[index] = models.PlanAllocation{
			ID: allocation.Id, Name: allocation.Name, AmountMinor: shared.MinorUnits(allocation.AmountMinor),
		}
	}
	return models.Summary{
		Currency:  financialenginepb.Currency_name[int32(response.Currency)][len("CURRENCY_"):],
		PlanStart: planStart, PlanEnd: planEnd,
		ExpectedIncomeMinor:                shared.MinorUnits(response.ExpectedIncomeMinor),
		CommittedBillsMinor:                shared.MinorUnits(response.CommittedBillsMinor),
		CommittedSubscriptionsMinor:        shared.MinorUnits(response.CommittedSubscriptionsMinor),
		PlannedMinor:                       shared.MinorUnits(response.PlannedMinor),
		UnallocatedMinor:                   shared.MinorUnits(response.UnallocatedMinor),
		RecurringBillsMonthlyMinor:         shared.MinorUnits(response.RecurringBillsMonthlyMinor),
		RecurringSubscriptionsMonthlyMinor: shared.MinorUnits(response.RecurringSubscriptionsMonthlyMinor),
		RecurringMonthlyMinor:              shared.MinorUnits(response.RecurringMonthlyMinor),
		RecurringSharePercent:              response.RecurringSharePercent,
		Next7DaysMinor:                     shared.MinorUnits(response.Next_7DaysMinor),
		Next30DaysMinor:                    shared.MinorUnits(response.Next_30DaysMinor),
		AnnualCostMinor:                    shared.MinorUnits(response.AnnualCostMinor),
		Allocations:                        allocations,
	}, nil
}
