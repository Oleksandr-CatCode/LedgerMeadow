package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

type Event struct {
	SourceID           string            `json:"source_id"`
	Occurrence         uint32            `json:"occurrence"`
	Date               string            `json:"date"`
	Kind               string            `json:"kind"`
	Name               string            `json:"name"`
	AmountMinor        shared.MinorUnits `json:"amount_minor"`
	BalanceBeforeMinor shared.MinorUnits `json:"balance_before_minor"`
	BalanceAfterMinor  shared.MinorUnits `json:"balance_after_minor"`
}

type Snapshot struct {
	Currency             string             `json:"currency"`
	AsOfDate             string             `json:"as_of_date"`
	ProjectionEndDate    *string            `json:"projection_end_date,omitempty"`
	StartingBalanceMinor *shared.MinorUnits `json:"starting_balance_minor,omitempty"`
	EndingBalanceMinor   *shared.MinorUnits `json:"ending_balance_minor,omitempty"`
	MinimumBalanceMinor  *shared.MinorUnits `json:"minimum_balance_minor,omitempty"`
	MinimumBalanceDate   *string            `json:"minimum_balance_date,omitempty"`
	Points               []Event            `json:"points"`
	UpdatedAt            time.Time          `json:"updated_at"`
}
