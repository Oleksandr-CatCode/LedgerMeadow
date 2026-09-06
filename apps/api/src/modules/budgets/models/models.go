package models

import shared "ledgermeadow/src/shared/types"

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
type Create struct {
	Name              string
	CategoryID        *string
	SpaceID           *string
	Period            string
	LimitMinor        int64
	Currency          string
	WarningThreshold  int
	CriticalThreshold int
	CarryoverEnabled  bool
}
