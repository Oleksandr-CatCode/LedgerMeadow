package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	analyticsmodels "ledgermeadow/src/modules/analytics/models"
	cashflowmodels "ledgermeadow/src/modules/cashflow/models"
	"ledgermeadow/src/modules/materialization/models"
	financialenginepb "ledgermeadow/src/modules/platform/financialengine/pb"
	timelinemodels "ledgermeadow/src/modules/timeline/models"
	shared "ledgermeadow/src/shared/types"
)

var ErrEngineResponse = errors.New("financial engine returned an invalid response")

var analyticsViews = [...]string{"SPENDING", "INCOME", "CASH_FLOW", "CATEGORIES", "MERCHANTS", "RECURRING"}

const timelineHorizonDays = 20

type Repository interface {
	Load(ctx context.Context, userID shared.UserID, now time.Time) (models.Input, error)
	Persist(ctx context.Context, userID shared.UserID, result models.Result) error
}

type Engine interface {
	BuildProjection(context.Context, *financialenginepb.BuildProjectionRequest) (*financialenginepb.BuildProjectionResponse, error)
	CalculateAvailableToSpend(context.Context, *financialenginepb.CalculateAvailableToSpendRequest) (*financialenginepb.CalculateAvailableToSpendResponse, error)
	CalculateCashFlow(context.Context, *financialenginepb.CalculateCashFlowRequest) (*financialenginepb.CalculateCashFlowResponse, error)
	CalculateAnalyticsBreakdown(context.Context, *financialenginepb.CalculateAnalyticsBreakdownRequest) (*financialenginepb.CalculateAnalyticsBreakdownResponse, error)
	CalculateNetWorth(context.Context, *financialenginepb.CalculateNetWorthRequest) (*financialenginepb.CalculateNetWorthResponse, error)
}

type Service struct {
	repository Repository
	engine     Engine
	now        func() time.Time
}

func New(repository Repository, engine Engine) *Service {
	return &Service{repository: repository, engine: engine, now: time.Now}
}

func (s *Service) Materialize(ctx context.Context, userID shared.UserID) error {
	input, err := s.repository.Load(ctx, userID, s.now())
	if err != nil {
		return err
	}
	result := models.Result{
		AsOfDate: input.AsOfDate, PeriodStart: input.PeriodStart, PeriodEnd: input.PeriodEnd,
		Currencies: make([]models.CurrencyResult, 0, len(input.Currencies)),
	}
	for _, currencyInput := range input.Currencies {
		currencyResult, err := s.calculateCurrency(ctx, input, currencyInput)
		if err != nil {
			return fmt.Errorf("calculate %s financial materialization: %w", currencyInput.Currency, err)
		}
		result.Currencies = append(result.Currencies, currencyResult)
	}
	return s.repository.Persist(ctx, userID, result)
}

func (s *Service) calculateCurrency(
	ctx context.Context,
	input models.Input,
	currencyInput models.CurrencyInput,
) (models.CurrencyResult, error) {
	currency, err := engineCurrency(currencyInput.Currency)
	if err != nil {
		return models.CurrencyResult{}, err
	}
	projectionRequest := &financialenginepb.BuildProjectionRequest{
		Currency:             currency,
		StartingBalanceMinor: currencyInput.LiquidBalanceMinor,
		StartDate:            engineDate(input.AsOfDate),
		EndDate:              engineDate(input.PeriodEnd),
		SafetyFloorMinor:     0,
		MaxEvents:            models.MaxProjectionEvents,
		Sources:              make([]*financialenginepb.ProjectionSource, 0, len(currencyInput.ProjectionSources)),
	}
	for _, source := range currencyInput.ProjectionSources {
		kind, frequency, err := projectionEnums(source.Kind, source.Frequency)
		if err != nil {
			return models.CurrencyResult{}, err
		}
		projectionRequest.Sources = append(projectionRequest.Sources, &financialenginepb.ProjectionSource{
			SourceId: source.ID, Name: source.Name, Kind: kind, AmountMinor: source.AmountMinor,
			FirstDate: engineDate(source.FirstDate), Frequency: frequency,
		})
	}
	projection, err := s.engine.BuildProjection(ctx, projectionRequest)
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("build projection: %w", err)
	}
	if projection == nil || projection.Currency != currency || len(projection.Events) > models.MaxProjectionEvents {
		return models.CurrencyResult{}, fmt.Errorf("month projection: %w", ErrEngineResponse)
	}
	protectionRequest := *projectionRequest
	protectionRequest.PayCycleHorizon = true
	protectionProjection, err := s.engine.BuildProjection(ctx, &protectionRequest)
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("build pay-cycle projection: %w", err)
	}
	if protectionProjection == nil || protectionProjection.Currency != currency || protectionProjection.MinimumBalanceDate == nil || len(protectionProjection.Events) > models.MaxProjectionEvents {
		return models.CurrencyResult{}, fmt.Errorf("pay-cycle projection: %w", ErrEngineResponse)
	}
	timelineEndDate := input.AsOfDate.AddDate(0, 0, timelineHorizonDays)
	timelineRequest := *projectionRequest
	timelineRequest.EndDate = engineDate(timelineEndDate)
	timeline, err := s.engine.BuildProjection(ctx, &timelineRequest)
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("build timeline projection: %w", err)
	}
	if timeline == nil || timeline.Currency != currency || timeline.MinimumBalanceDate == nil || len(timeline.Events) > models.MaxProjectionEvents {
		return models.CurrencyResult{}, fmt.Errorf("timeline projection: %w", ErrEngineResponse)
	}

	projected := models.CashFlowComponents{}
	recurring := make(map[string]*financialenginepb.AnalyticsBucket)
	for _, event := range projection.Events {
		if event == nil || event.Date == nil || event.SourceId == "" || event.Name == "" || event.AmountMinor == 0 {
			return models.CurrencyResult{}, ErrEngineResponse
		}
		if event.Kind == financialenginepb.ProjectionKind_PROJECTION_KIND_SALARY {
			if event.AmountMinor < 0 {
				return models.CurrencyResult{}, ErrEngineResponse
			}
			projected.IncomeMinor, err = checkedAdd(projected.IncomeMinor, event.AmountMinor)
			if err != nil {
				return models.CurrencyResult{}, err
			}
			continue
		}
		if event.AmountMinor > 0 {
			return models.CurrencyResult{}, ErrEngineResponse
		}
		amount, err := checkedNegate(event.AmountMinor)
		if err != nil {
			return models.CurrencyResult{}, err
		}
		isRecurringObligation := false
		switch event.Kind {
		case financialenginepb.ProjectionKind_PROJECTION_KIND_BILL:
			projected.FixedOutflowMinor, err = checkedAdd(projected.FixedOutflowMinor, amount)
			isRecurringObligation = true
		case financialenginepb.ProjectionKind_PROJECTION_KIND_LOAN_PAYMENT:
			projected.FixedOutflowMinor, err = checkedAdd(projected.FixedOutflowMinor, amount)
		case financialenginepb.ProjectionKind_PROJECTION_KIND_SUBSCRIPTION:
			projected.SubscriptionsMinor, err = checkedAdd(projected.SubscriptionsMinor, amount)
			isRecurringObligation = true
		default:
			return models.CurrencyResult{}, ErrEngineResponse
		}
		if isRecurringObligation {
			bucket := recurring[event.SourceId]
			if bucket == nil {
				bucket = &financialenginepb.AnalyticsBucket{Id: event.SourceId, Label: event.Name}
				recurring[event.SourceId] = bucket
			}
			bucket.AmountMinor, err = checkedAdd(bucket.AmountMinor, amount)
			if bucket.ItemCount == math.MaxUint32 {
				return models.CurrencyResult{}, errors.New("recurring analytics item count overflow")
			}
			bucket.ItemCount++
		}
		if err != nil {
			return models.CurrencyResult{}, err
		}
	}
	protected, eligibleExpectedIncome, err := payCycleProtection(protectionProjection.Events)
	if err != nil {
		return models.CurrencyResult{}, err
	}
	for _, item := range currencyInput.ProtectedSpaces {
		protected = append(protected, &financialenginepb.ProtectedItem{
			Id: item.ID, Label: item.Label,
			Kind: financialenginepb.ProtectedKind_PROTECTED_KIND_PROTECTED_SPACE, AmountMinor: item.AmountMinor,
		})
	}
	if len(protected)+len(currencyInput.PlannedAllocations) > models.MaxBreakdownItems {
		return models.CurrencyResult{}, errors.New("available-to-spend input exceeds bound")
	}
	planned := make([]*financialenginepb.PlannedAllocation, 0, len(currencyInput.PlannedAllocations))
	for _, item := range currencyInput.PlannedAllocations {
		planned = append(planned, &financialenginepb.PlannedAllocation{Id: item.ID, Label: item.Label, AmountMinor: item.AmountMinor})
	}
	available, err := s.engine.CalculateAvailableToSpend(ctx, &financialenginepb.CalculateAvailableToSpendRequest{
		Currency: currency, LiquidBalanceMinor: currencyInput.LiquidBalanceMinor,
		EligibleExpectedIncomeMinor: eligibleExpectedIncome,
		ProtectedItems:              protected, PlannedAllocations: planned,
		ProjectionStatus:             protectionProjection.Status,
		MinimumProjectedBalanceMinor: protectionProjection.MinimumBalanceMinor,
	})
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("calculate available to spend: %w", err)
	}
	if available == nil || available.Currency != currency {
		return models.CurrencyResult{}, fmt.Errorf("available to spend: %w", ErrEngineResponse)
	}
	breakdownJSON, err := availableBreakdownJSON(
		available,
		currencyInput.LiquidBalanceMinor,
		eligibleExpectedIncome,
		protectionProjection.MinimumBalanceMinor,
		protected,
		planned,
	)
	if err != nil {
		return models.CurrencyResult{}, err
	}

	cashFlow, err := s.engine.CalculateCashFlow(ctx, &financialenginepb.CalculateCashFlowRequest{
		Currency: currency, Actual: cashFlowComponents(currencyInput.Actual), Projected: cashFlowComponents(projected),
	})
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("calculate cash flow: %w", err)
	}
	if cashFlow == nil || cashFlow.Currency != currency || cashFlow.Actual == nil || cashFlow.Projected == nil {
		return models.CurrencyResult{}, fmt.Errorf("cash flow: %w", ErrEngineResponse)
	}
	actualJSON, err := json.Marshal(cashFlowSummary(cashFlow.Actual))
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("encode actual cash flow: %w", err)
	}
	projectedJSON, err := json.Marshal(cashFlowSummary(cashFlow.Projected))
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("encode projected cash flow: %w", err)
	}
	timelineJSON, err := timelineJSON(timeline.Events)
	if err != nil {
		return models.CurrencyResult{}, err
	}

	assets := valuations(currencyInput.Assets)
	liabilities := valuations(currencyInput.Liabilities)
	netWorth, err := s.engine.CalculateNetWorth(ctx, &financialenginepb.CalculateNetWorthRequest{
		Currency: currency, Assets: assets, Liabilities: liabilities,
	})
	if err != nil {
		return models.CurrencyResult{}, fmt.Errorf("calculate net worth: %w", err)
	}
	if netWorth == nil || netWorth.Currency != currency {
		return models.CurrencyResult{}, fmt.Errorf("net worth: %w", ErrEngineResponse)
	}

	analyticsInputs := currencyInput.Analytics
	analyticsInputs["CASH_FLOW"] = cashFlowBuckets(cashFlow.Actual, currencyInput.Actual)
	recurringBuckets := make([]models.AnalyticsBucket, 0, len(recurring))
	for _, bucket := range recurring {
		recurringBuckets = append(recurringBuckets, models.AnalyticsBucket{
			ID: bucket.Id, Label: bucket.Label, AmountMinor: bucket.AmountMinor, ItemCount: bucket.ItemCount,
		})
	}
	analyticsInputs["RECURRING"] = recurringBuckets
	analyticsResults := make([]models.AnalyticsResult, 0, len(analyticsViews))
	for _, view := range analyticsViews {
		buckets := analyticsInputs[view]
		if len(buckets) > models.MaxAnalyticsBuckets {
			return models.CurrencyResult{}, errors.New("analytics input exceeds bound")
		}
		requestBuckets := make([]*financialenginepb.AnalyticsBucket, 0, len(buckets))
		for _, bucket := range buckets {
			requestBuckets = append(requestBuckets, &financialenginepb.AnalyticsBucket{
				Id: bucket.ID, Label: bucket.Label, AmountMinor: bucket.AmountMinor, ItemCount: bucket.ItemCount,
			})
		}
		breakdown, err := s.engine.CalculateAnalyticsBreakdown(ctx, &financialenginepb.CalculateAnalyticsBreakdownRequest{
			Currency: currency, Buckets: requestBuckets,
		})
		if err != nil {
			return models.CurrencyResult{}, fmt.Errorf("calculate %s analytics: %w", view, err)
		}
		if breakdown == nil || breakdown.Currency != currency || len(breakdown.Buckets) > models.MaxAnalyticsBuckets {
			return models.CurrencyResult{}, ErrEngineResponse
		}
		encoded, err := analyticsJSON(breakdown.Buckets)
		if err != nil {
			return models.CurrencyResult{}, err
		}
		analyticsResults = append(analyticsResults, models.AnalyticsResult{
			ViewType: view, TotalMinor: breakdown.TotalMinor, BreakdownJSON: encoded,
		})
	}
	status, err := projectionStatus(available.Status)
	if err != nil {
		return models.CurrencyResult{}, err
	}
	return models.CurrencyResult{
		Currency: currencyInput.Currency, TotalMinor: available.TotalMinor,
		AvailableMinor: available.AvailableMinor, ProtectedMinor: available.ProtectedMinor,
		ProjectedMonthEndMinor: projection.EndingBalanceMinor, ProjectionStatus: status, BreakdownJSON: breakdownJSON,
		TotalAssetsMinor: netWorth.TotalAssetsMinor, TotalLiabilitiesMinor: netWorth.TotalLiabilitiesMinor,
		NetWorthMinor: netWorth.NetWorthMinor, TimelineEndDate: timelineEndDate,
		TimelineStartingMinor: currencyInput.LiquidBalanceMinor, TimelineEndingMinor: timeline.EndingBalanceMinor,
		TimelineMinimumMinor: timeline.MinimumBalanceMinor, TimelineMinimumDate: time.Date(
			int(timeline.MinimumBalanceDate.Year), time.Month(timeline.MinimumBalanceDate.Month), int(timeline.MinimumBalanceDate.Day),
			0, 0, 0, 0, input.AsOfDate.Location(),
		), ActualCashFlowJSON: actualJSON,
		ProjectedCashFlowJSON: projectedJSON, TimelineJSON: timelineJSON, Analytics: analyticsResults,
	}, nil
}

func payCycleProtection(events []*financialenginepb.TimelineEvent) ([]*financialenginepb.ProtectedItem, int64, error) {
	items := make([]*financialenginepb.ProtectedItem, 0, len(events))
	positions := make(map[string]int, len(events))
	var expectedIncome int64
	for _, event := range events {
		if event == nil || event.Date == nil || event.SourceId == "" || event.Name == "" || event.AmountMinor == 0 {
			return nil, 0, ErrEngineResponse
		}
		if event.Kind == financialenginepb.ProjectionKind_PROJECTION_KIND_SALARY {
			if event.AmountMinor < 0 {
				return nil, 0, ErrEngineResponse
			}
			var err error
			expectedIncome, err = checkedAdd(expectedIncome, event.AmountMinor)
			if err != nil {
				return nil, 0, err
			}
			continue
		}
		amount, err := checkedNegate(event.AmountMinor)
		if err != nil || amount < 0 {
			return nil, 0, ErrEngineResponse
		}
		kind := financialenginepb.ProtectedKind_PROTECTED_KIND_UPCOMING_BILL
		switch event.Kind {
		case financialenginepb.ProjectionKind_PROJECTION_KIND_BILL,
			financialenginepb.ProjectionKind_PROJECTION_KIND_LOAN_PAYMENT:
		case financialenginepb.ProjectionKind_PROJECTION_KIND_SUBSCRIPTION:
			kind = financialenginepb.ProtectedKind_PROTECTED_KIND_SUBSCRIPTION
		default:
			return nil, 0, ErrEngineResponse
		}
		if position, ok := positions[event.SourceId]; ok {
			item := items[position]
			if item.Label != event.Name || item.Kind != kind {
				return nil, 0, ErrEngineResponse
			}
			item.AmountMinor, err = checkedAdd(item.AmountMinor, amount)
			if err != nil {
				return nil, 0, err
			}
			continue
		}
		positions[event.SourceId] = len(items)
		items = append(items, &financialenginepb.ProtectedItem{
			Id: event.SourceId, Label: event.Name, Kind: kind, AmountMinor: amount,
		})
	}
	return items, expectedIncome, nil
}

type availableBreakdownItem struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Kind        string            `json:"kind"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
	EffectMinor shared.MinorUnits `json:"effect_minor"`
}

func availableBreakdownJSON(
	response *financialenginepb.CalculateAvailableToSpendResponse,
	liquidBalance int64,
	expectedIncome int64,
	minimumProjectedBalance int64,
	protected []*financialenginepb.ProtectedItem,
	planned []*financialenginepb.PlannedAllocation,
) ([]byte, error) {
	expectedTotal, err := checkedAdd(liquidBalance, expectedIncome)
	if err != nil {
		return nil, err
	}
	var expectedProtected int64
	var scheduledObligations int64
	for _, item := range protected {
		expectedProtected, err = checkedAdd(expectedProtected, item.AmountMinor)
		if err != nil {
			return nil, err
		}
		if item.Kind == financialenginepb.ProtectedKind_PROTECTED_KIND_UPCOMING_BILL ||
			item.Kind == financialenginepb.ProtectedKind_PROTECTED_KIND_SUBSCRIPTION {
			scheduledObligations, err = checkedAdd(scheduledObligations, item.AmountMinor)
			if err != nil {
				return nil, err
			}
		}
	}
	negatedScheduledObligations, err := checkedNegate(scheduledObligations)
	if err != nil {
		return nil, err
	}
	projectedEndingBalance, err := checkedAdd(expectedTotal, negatedScheduledObligations)
	if err != nil || minimumProjectedBalance > projectedEndingBalance {
		return nil, ErrEngineResponse
	}
	negatedMinimumProjectedBalance, err := checkedNegate(minimumProjectedBalance)
	if err != nil {
		return nil, ErrEngineResponse
	}
	payCycleReserve, err := checkedAdd(projectedEndingBalance, negatedMinimumProjectedBalance)
	if err != nil {
		return nil, err
	}
	expectedProtected, err = checkedAdd(expectedProtected, payCycleReserve)
	if err != nil {
		return nil, err
	}
	var expectedPlanned int64
	for _, item := range planned {
		expectedPlanned, err = checkedAdd(expectedPlanned, item.AmountMinor)
		if err != nil {
			return nil, err
		}
	}
	expectedAvailable, err := checkedAdd(expectedTotal, -expectedProtected)
	if err == nil {
		expectedAvailable, err = checkedAdd(expectedAvailable, -expectedPlanned)
	}
	if err != nil || response.TotalMinor != expectedTotal || response.ProtectedMinor != expectedProtected ||
		response.PlannedAllocationsMinor != expectedPlanned || response.AvailableMinor != expectedAvailable ||
		len(response.Breakdown) > models.MaxBreakdownItems+4 {
		return nil, ErrEngineResponse
	}
	items := make([]availableBreakdownItem, 0, len(response.Breakdown))
	var effectTotal int64
	for _, component := range response.Breakdown {
		if component == nil || component.Id == "" || component.Label == "" {
			return nil, ErrEngineResponse
		}
		kind, err := availableComponentKind(component.Kind)
		if err != nil {
			return nil, err
		}
		effectTotal, err = checkedAdd(effectTotal, component.EffectMinor)
		if err != nil {
			return nil, err
		}
		items = append(items, availableBreakdownItem{
			ID: component.Id, Label: component.Label, Kind: kind,
			AmountMinor: shared.MinorUnits(component.AmountMinor), EffectMinor: shared.MinorUnits(component.EffectMinor),
		})
	}
	if effectTotal != response.AvailableMinor {
		return nil, ErrEngineResponse
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode available-to-spend breakdown: %w", err)
	}
	return encoded, nil
}

func availableComponentKind(kind financialenginepb.AvailableComponentKind) (string, error) {
	switch kind {
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_LIQUID_BALANCE:
		return "LIQUID_BALANCE", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_EXPECTED_INCOME:
		return "EXPECTED_INCOME", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_UPCOMING_BILL:
		return "UPCOMING_BILL", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_SUBSCRIPTION:
		return "SUBSCRIPTION", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_PROTECTED_SPACE:
		return "PROTECTED_SPACE", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_REQUIRED_GOAL_CONTRIBUTION:
		return "REQUIRED_GOAL_CONTRIBUTION", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_SAFETY_BUFFER:
		return "SAFETY_BUFFER", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_PLANNED_ALLOCATION:
		return "PLANNED_ALLOCATION", nil
	case financialenginepb.AvailableComponentKind_AVAILABLE_COMPONENT_KIND_PAY_CYCLE_RESERVE:
		return "PAY_CYCLE_RESERVE", nil
	default:
		return "", ErrEngineResponse
	}
}

func engineCurrency(currency string) (financialenginepb.Currency, error) {
	switch currency {
	case "CAD":
		return financialenginepb.Currency_CURRENCY_CAD, nil
	case "USD":
		return financialenginepb.Currency_CURRENCY_USD, nil
	default:
		return financialenginepb.Currency_CURRENCY_UNSPECIFIED, errors.New("unsupported financial engine currency")
	}
}

func engineDate(value time.Time) *financialenginepb.Date {
	return &financialenginepb.Date{Year: int32(value.Year()), Month: uint32(value.Month()), Day: uint32(value.Day())}
}

func projectionEnums(kind string, frequency string) (financialenginepb.ProjectionKind, financialenginepb.Frequency, error) {
	kinds := map[string]financialenginepb.ProjectionKind{
		"SALARY":       financialenginepb.ProjectionKind_PROJECTION_KIND_SALARY,
		"BILL":         financialenginepb.ProjectionKind_PROJECTION_KIND_BILL,
		"SUBSCRIPTION": financialenginepb.ProjectionKind_PROJECTION_KIND_SUBSCRIPTION,
		"LOAN_PAYMENT": financialenginepb.ProjectionKind_PROJECTION_KIND_LOAN_PAYMENT,
	}
	frequencies := map[string]financialenginepb.Frequency{
		"WEEKLY":    financialenginepb.Frequency_FREQUENCY_WEEKLY,
		"BIWEEKLY":  financialenginepb.Frequency_FREQUENCY_BIWEEKLY,
		"MONTHLY":   financialenginepb.Frequency_FREQUENCY_MONTHLY,
		"QUARTERLY": financialenginepb.Frequency_FREQUENCY_QUARTERLY,
		"ANNUALLY":  financialenginepb.Frequency_FREQUENCY_ANNUALLY,
	}
	projectionKind, kindOK := kinds[kind]
	projectionFrequency, frequencyOK := frequencies[frequency]
	if !kindOK || !frequencyOK {
		return 0, 0, errors.New("stored projection source is invalid")
	}
	return projectionKind, projectionFrequency, nil
}

func cashFlowComponents(value models.CashFlowComponents) *financialenginepb.CashFlowComponents {
	return &financialenginepb.CashFlowComponents{
		IncomeMinor: value.IncomeMinor, FixedOutflowMinor: value.FixedOutflowMinor,
		VariableOutflowMinor: value.VariableOutflowMinor,
		SubscriptionsMinor:   value.SubscriptionsMinor, SavingsMinor: value.SavingsMinor,
	}
}

func cashFlowSummary(value *financialenginepb.CashFlowSummary) cashflowmodels.Summary {
	return cashflowmodels.Summary{
		IncomeMinor: shared.MinorUnits(value.IncomeMinor), FixedOutflowMinor: shared.MinorUnits(value.FixedOutflowMinor),
		VariableOutflowMinor: shared.MinorUnits(value.VariableOutflowMinor),
		SubscriptionsMinor:   shared.MinorUnits(value.SubscriptionsMinor), SavingsMinor: shared.MinorUnits(value.SavingsMinor),
		TotalOutflowMinor: shared.MinorUnits(value.TotalOutflowMinor), NetMinor: shared.MinorUnits(value.NetMinor),
	}
}

func valuations(items []models.Item) []*financialenginepb.Valuation {
	result := make([]*financialenginepb.Valuation, 0, len(items))
	for _, item := range items {
		result = append(result, &financialenginepb.Valuation{Id: item.ID, Label: item.Label, AmountMinor: item.AmountMinor})
	}
	return result
}

func cashFlowBuckets(
	actual *financialenginepb.CashFlowSummary,
	components models.CashFlowComponents,
) []models.AnalyticsBucket {
	buckets := make([]models.AnalyticsBucket, 0, 2)
	if actual.IncomeMinor > 0 {
		buckets = append(buckets, models.AnalyticsBucket{
			ID: "income", Label: "Income", AmountMinor: actual.IncomeMinor, ItemCount: components.IncomeCount,
		})
	}
	if actual.TotalOutflowMinor > 0 {
		buckets = append(buckets, models.AnalyticsBucket{
			ID: "outflow", Label: "Outflow", AmountMinor: actual.TotalOutflowMinor, ItemCount: components.OutflowCount,
		})
	}
	return buckets
}

func timelineJSON(events []*financialenginepb.TimelineEvent) ([]byte, error) {
	points := make([]timelinemodels.Event, 0, len(events))
	for _, event := range events {
		if event == nil || event.Date == nil {
			return nil, ErrEngineResponse
		}
		kind, err := projectionKind(event.Kind)
		if err != nil {
			return nil, err
		}
		points = append(points, timelinemodels.Event{
			SourceID: event.SourceId, Occurrence: event.Occurrence,
			Date: fmt.Sprintf("%04d-%02d-%02d", event.Date.Year, event.Date.Month, event.Date.Day), Kind: kind, Name: event.Name,
			AmountMinor: shared.MinorUnits(event.AmountMinor), BalanceBeforeMinor: shared.MinorUnits(event.BalanceBeforeMinor),
			BalanceAfterMinor: shared.MinorUnits(event.BalanceAfterMinor),
		})
	}
	encoded, err := json.Marshal(points)
	if err != nil {
		return nil, fmt.Errorf("encode projection timeline: %w", err)
	}
	return encoded, nil
}

func analyticsJSON(buckets []*financialenginepb.AnalyticsBreakdownItem) ([]byte, error) {
	items := make([]analyticsmodels.BreakdownItem, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket == nil || bucket.Id == "" || bucket.Label == "" || bucket.AmountMinor < 0 || bucket.ShareBasisPoints > 10_000 {
			return nil, ErrEngineResponse
		}
		items = append(items, analyticsmodels.BreakdownItem{
			ID: bucket.Id, Label: bucket.Label, AmountMinor: shared.MinorUnits(bucket.AmountMinor),
			ItemCount: bucket.ItemCount, ShareBasisPoints: bucket.ShareBasisPoints,
		})
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("encode analytics breakdown: %w", err)
	}
	return encoded, nil
}

func projectionKind(kind financialenginepb.ProjectionKind) (string, error) {
	switch kind {
	case financialenginepb.ProjectionKind_PROJECTION_KIND_SALARY:
		return "SALARY", nil
	case financialenginepb.ProjectionKind_PROJECTION_KIND_BILL:
		return "BILL", nil
	case financialenginepb.ProjectionKind_PROJECTION_KIND_SUBSCRIPTION:
		return "SUBSCRIPTION", nil
	case financialenginepb.ProjectionKind_PROJECTION_KIND_LOAN_PAYMENT:
		return "LOAN_PAYMENT", nil
	case financialenginepb.ProjectionKind_PROJECTION_KIND_PLANNED_TRANSFER:
		return "PLANNED_TRANSFER", nil
	case financialenginepb.ProjectionKind_PROJECTION_KIND_GOAL_CONTRIBUTION:
		return "GOAL_CONTRIBUTION", nil
	case financialenginepb.ProjectionKind_PROJECTION_KIND_OTHER:
		return "OTHER", nil
	default:
		return "", ErrEngineResponse
	}
}

func projectionStatus(status financialenginepb.ProjectionStatus) (string, error) {
	switch status {
	case financialenginepb.ProjectionStatus_PROJECTION_STATUS_ON_TRACK:
		return "ON_TRACK", nil
	case financialenginepb.ProjectionStatus_PROJECTION_STATUS_WATCH:
		return "WATCH", nil
	case financialenginepb.ProjectionStatus_PROJECTION_STATUS_AT_RISK:
		return "AT_RISK", nil
	default:
		return "", ErrEngineResponse
	}
}

func checkedAdd(left int64, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, errors.New("financial materialization arithmetic overflow")
	}
	return left + right, nil
}

func checkedNegate(value int64) (int64, error) {
	if value == math.MinInt64 {
		return 0, errors.New("financial materialization arithmetic overflow")
	}
	return -value, nil
}
