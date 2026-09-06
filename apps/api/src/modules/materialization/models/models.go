package models

import "time"

const (
	MaxAnalyticsBuckets  = 512
	MaxBreakdownItems    = 512
	MaxProjectionEvents  = 512
	MaxProjectionSources = 512
	MaxValuations        = 512
)

type Input struct {
	AsOfDate    time.Time
	PeriodStart time.Time
	PeriodEnd   time.Time
	Currencies  []CurrencyInput
}

type CurrencyInput struct {
	Currency           string
	LiquidBalanceMinor int64
	Assets             []Item
	Liabilities        []Item
	ProtectedSpaces    []Item
	PlannedAllocations []Item
	ProjectionSources  []ProjectionSource
	Actual             CashFlowComponents
	Analytics          map[string][]AnalyticsBucket
}

type Item struct {
	ID          string
	Label       string
	AmountMinor int64
}

type ProjectionSource struct {
	ID          string
	Name        string
	Kind        string
	AmountMinor int64
	FirstDate   time.Time
	Frequency   string
}

type CashFlowComponents struct {
	IncomeMinor          int64
	IncomeCount          uint32
	FixedOutflowMinor    int64
	VariableOutflowMinor int64
	SubscriptionsMinor   int64
	OutflowCount         uint32
	SavingsMinor         int64
}

type AnalyticsBucket struct {
	ID          string
	Label       string
	AmountMinor int64
	ItemCount   uint32
}

type Result struct {
	AsOfDate    time.Time
	PeriodStart time.Time
	PeriodEnd   time.Time
	Currencies  []CurrencyResult
}

type CurrencyResult struct {
	Currency               string
	TotalMinor             int64
	AvailableMinor         int64
	ProtectedMinor         int64
	ProjectedMonthEndMinor int64
	ProjectionStatus       string
	BreakdownJSON          []byte
	TotalAssetsMinor       int64
	TotalLiabilitiesMinor  int64
	NetWorthMinor          int64
	TimelineEndDate        time.Time
	TimelineStartingMinor  int64
	TimelineEndingMinor    int64
	TimelineMinimumMinor   int64
	TimelineMinimumDate    time.Time
	ActualCashFlowJSON     []byte
	ProjectedCashFlowJSON  []byte
	TimelineJSON           []byte
	Analytics              []AnalyticsResult
}

type AnalyticsResult struct {
	ViewType      string
	TotalMinor    int64
	BreakdownJSON []byte
}
