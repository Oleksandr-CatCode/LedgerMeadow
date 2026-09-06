package models

import shared "ledgermeadow/src/shared/types"

type Bill struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	AmountType          string            `json:"amount_type"`
	ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
	Currency            string            `json:"currency"`
	Frequency           string            `json:"frequency"`
	NextDueAt           string            `json:"next_due_at"`
	CategoryID          *string           `json:"category_id"`
	CategoryName        *string           `json:"category_name"`
	SpaceID             *string           `json:"space_id"`
	SpaceName           *string           `json:"space_name"`
	Source              string            `json:"source"`
	Status              string            `json:"status"`
	OccurrenceCount     *int              `json:"occurrence_count"`
}
type Subscription struct {
	ID                  string            `json:"id"`
	MerchantName        string            `json:"merchant_name"`
	ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
	Currency            string            `json:"currency"`
	Frequency           string            `json:"frequency"`
	NextExpectedAt      string            `json:"next_expected_at"`
	CategoryID          *string           `json:"category_id"`
	CategoryName        *string           `json:"category_name"`
	SpaceID             *string           `json:"space_id"`
	SpaceName           *string           `json:"space_name"`
	PaymentAccountID    *string           `json:"payment_account_id"`
	PaymentAccountName  *string           `json:"payment_account_name"`
	Status              string            `json:"status"`
	Source              string            `json:"source"`
	OccurrenceCount     *int              `json:"occurrence_count"`
}
type RecurringIncome struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
	Currency            string            `json:"currency"`
	Frequency           string            `json:"frequency"`
	NextExpectedAt      string            `json:"next_expected_at"`
	CategoryID          *string           `json:"category_id"`
	CategoryName        *string           `json:"category_name"`
	PaymentAccountID    *string           `json:"payment_account_id"`
	PaymentAccountName  *string           `json:"payment_account_name"`
	Status              string            `json:"status"`
	Source              string            `json:"source"`
	OccurrenceCount     int               `json:"occurrence_count"`
}
type Budget struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	CategoryID        *string           `json:"category_id"`
	SpaceID           *string           `json:"space_id"`
	Period            string            `json:"period"`
	LimitMinor        shared.MinorUnits `json:"limit_minor"`
	SpentMinor        shared.MinorUnits `json:"spent_minor"`
	Currency          string            `json:"currency"`
	WarningThreshold  int               `json:"warning_threshold"`
	CriticalThreshold int               `json:"critical_threshold"`
	CarryoverEnabled  bool              `json:"carryover_enabled"`
}
type Goal struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	TargetMinor  shared.MinorUnits `json:"target_minor"`
	CurrentMinor shared.MinorUnits `json:"current_minor"`
	Currency     string            `json:"currency"`
	TargetDate   *string           `json:"target_date"`
	SpaceID      *string           `json:"space_id"`
	SpaceName    *string           `json:"space_name"`
	Status       string            `json:"status"`
}

type PlanAllocation struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
}

type Summary struct {
	Currency                           string            `json:"currency"`
	PlanStart                          string            `json:"plan_start"`
	PlanEnd                            string            `json:"plan_end"`
	ExpectedIncomeMinor                shared.MinorUnits `json:"expected_income_minor"`
	CommittedBillsMinor                shared.MinorUnits `json:"committed_bills_minor"`
	CommittedSubscriptionsMinor        shared.MinorUnits `json:"committed_subscriptions_minor"`
	PlannedMinor                       shared.MinorUnits `json:"planned_minor"`
	UnallocatedMinor                   shared.MinorUnits `json:"unallocated_minor"`
	RecurringBillsMonthlyMinor         shared.MinorUnits `json:"recurring_bills_monthly_minor"`
	RecurringSubscriptionsMonthlyMinor shared.MinorUnits `json:"recurring_subscriptions_monthly_minor"`
	RecurringMonthlyMinor              shared.MinorUnits `json:"recurring_monthly_minor"`
	RecurringSharePercent              uint32            `json:"recurring_share_percent"`
	Next7DaysMinor                     shared.MinorUnits `json:"next_7_days_minor"`
	Next30DaysMinor                    shared.MinorUnits `json:"next_30_days_minor"`
	AnnualCostMinor                    shared.MinorUnits `json:"annual_cost_minor"`
	Allocations                        []PlanAllocation  `json:"allocations"`
}

type Workspace struct {
	Bills           []Bill            `json:"bills"`
	Subscriptions   []Subscription    `json:"subscriptions"`
	RecurringIncome []RecurringIncome `json:"recurring_income"`
	Budgets         []Budget          `json:"budgets"`
	Goals           []Goal            `json:"goals"`
	Summaries       []Summary         `json:"summaries"`
	AsOfDate        string            `json:"-"`
	PlanStart       string            `json:"-"`
	PlanEnd         string            `json:"-"`
}
