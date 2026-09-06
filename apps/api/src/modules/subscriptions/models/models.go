package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

const DetailPaymentLimit = 100
const PaymentHistoryLimit = 4

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

type PaymentSource struct {
	PaidAt      string
	AmountMinor int64
}

type DetailSource struct {
	Subscription
	Payments []PaymentSource
}

type Payment struct {
	PaidAt      string            `json:"paid_at"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
}

type Detail struct {
	Subscription
	AnnualCostMinor   shared.MinorUnits `json:"annual_cost_minor"`
	PaidThisYearMinor shared.MinorUnits `json:"paid_this_year_minor"`
	PaymentHistory    []Payment         `json:"payment_history"`
}

type Create struct {
	MerchantName        string
	ExpectedAmountMinor int64
	Currency            string
	Frequency           string
	NextExpectedAt      time.Time
	CategoryID          *string
	SpaceID             *string
	PaymentAccountID    *string
}

type Update struct {
	Reclassify          string
	MerchantName        string
	ExpectedAmountMinor int64
	Currency            string
	Frequency           string
	NextExpectedAt      time.Time
	CategoryID          *string
	SpaceID             *string
	PaymentAccountID    *string
	Status              string
}
