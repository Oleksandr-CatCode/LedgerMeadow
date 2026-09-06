package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

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

type Update struct {
	Reclassify          string
	Name                string
	AmountType          string
	ExpectedAmountMinor int64
	Currency            string
	Frequency           string
	NextDueAt           time.Time
	CategoryID          *string
	SpaceID             *string
	Status              string
}
type Create struct {
	Name                string
	AmountType          string
	ExpectedAmountMinor int64
	Currency            string
	Frequency           string
	NextDueAt           time.Time
	CategoryID          *string
	SpaceID             *string
}
