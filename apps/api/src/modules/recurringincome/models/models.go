package models

import (
	"time"

	shared "ledgermeadow/src/shared/types"
)

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

type Update struct {
	Name                string
	ExpectedAmountMinor int64
	Currency            string
	Frequency           string
	NextExpectedAt      time.Time
	CategoryID          *string
	PaymentAccountID    *string
	Status              string
}
