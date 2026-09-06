package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

type BreakdownItem struct {
	ID               string            `json:"id"`
	Label            string            `json:"label"`
	AmountMinor      shared.MinorUnits `json:"amount_minor"`
	ItemCount        uint32            `json:"item_count"`
	ShareBasisPoints uint32            `json:"share_basis_points"`
}

type Snapshot struct {
	ViewType    string            `json:"view_type"`
	Currency    string            `json:"currency"`
	PeriodStart string            `json:"period_start"`
	PeriodEnd   string            `json:"period_end"`
	TotalMinor  shared.MinorUnits `json:"total_minor"`
	Breakdown   []BreakdownItem   `json:"breakdown"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
