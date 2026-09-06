package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

type Summary struct {
	IncomeMinor          shared.MinorUnits `json:"income_minor"`
	FixedOutflowMinor    shared.MinorUnits `json:"fixed_outflow_minor"`
	VariableOutflowMinor shared.MinorUnits `json:"variable_outflow_minor"`
	SubscriptionsMinor   shared.MinorUnits `json:"subscriptions_minor"`
	SavingsMinor         shared.MinorUnits `json:"savings_minor"`
	TotalOutflowMinor    shared.MinorUnits `json:"total_outflow_minor"`
	NetMinor             shared.MinorUnits `json:"net_minor"`
}

type Snapshot struct {
	Currency    string    `json:"currency"`
	PeriodStart string    `json:"period_start"`
	PeriodEnd   string    `json:"period_end"`
	Actual      Summary   `json:"actual"`
	Projected   Summary   `json:"projected"`
	UpdatedAt   time.Time `json:"updated_at"`
}
