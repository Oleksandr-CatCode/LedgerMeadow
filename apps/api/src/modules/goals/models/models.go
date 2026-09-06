package models

import (
	"time"

	shared "ledgermeadow/src/shared/types"
)

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

type Create struct {
	Name         string
	TargetMinor  int64
	CurrentMinor int64
	Currency     string
	TargetDate   *time.Time
	SpaceID      *string
}
