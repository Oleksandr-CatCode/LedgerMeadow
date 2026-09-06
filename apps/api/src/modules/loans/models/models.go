package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

type Loan struct {
	ID                      string            `json:"id"`
	Name                    string            `json:"name"`
	PrincipalRemainingMinor shared.MinorUnits `json:"principal_remaining_minor"`
	Currency                string            `json:"currency"`
	InterestRateBasisPoints int               `json:"interest_rate_basis_points"`
	MonthlyPaymentMinor     shared.MinorUnits `json:"monthly_payment_minor"`
	NextPaymentAt           *string           `json:"next_payment_at"`
}

type Payment struct {
	ID             string            `json:"id"`
	PaidAt         string            `json:"paid_at"`
	AmountMinor    shared.MinorUnits `json:"amount_minor"`
	PrincipalMinor shared.MinorUnits `json:"principal_minor"`
	InterestMinor  shared.MinorUnits `json:"interest_minor"`
}

type Detail struct {
	Loan
	Payments []Payment `json:"payments"`
}

type Create struct {
	Name                    string
	PrincipalRemainingMinor int64
	Currency                string
	InterestRateBasisPoints int
	MonthlyPaymentMinor     int64
	NextPaymentAt           *time.Time
}

type Update = Create

type ScenarioInput struct {
	ExtraMonthlyPaymentMinor int64 `json:"-"`
	IncludeSchedule          bool  `json:"include_schedule"`
}

type ScenarioPayment struct {
	PaymentNumber           uint32            `json:"payment_number"`
	PaymentDate             string            `json:"payment_date"`
	PaymentMinor            shared.MinorUnits `json:"payment_minor"`
	PrincipalMinor          shared.MinorUnits `json:"principal_minor"`
	InterestMinor           shared.MinorUnits `json:"interest_minor"`
	RemainingPrincipalMinor shared.MinorUnits `json:"remaining_principal_minor"`
}

type AmortizationSummary struct {
	MonthlyPaymentMinor shared.MinorUnits `json:"monthly_payment_minor"`
	PayoffMonths        uint32            `json:"payoff_months"`
	PayoffDate          *string           `json:"payoff_date"`
	TotalInterestMinor  shared.MinorUnits `json:"total_interest_minor"`
	TotalPaidMinor      shared.MinorUnits `json:"total_paid_minor"`
	Schedule            []ScenarioPayment `json:"schedule"`
}

type Scenario struct {
	Currency           string              `json:"currency"`
	Base               AmortizationSummary `json:"base"`
	Scenario           AmortizationSummary `json:"scenario"`
	MonthsSaved        uint32              `json:"months_saved"`
	InterestSavedMinor shared.MinorUnits   `json:"interest_saved_minor"`
}
