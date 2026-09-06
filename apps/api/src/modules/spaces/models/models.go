package models

import shared "ledgermeadow/src/shared/types"

type Space struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Type                   string            `json:"type"`
	Currency               string            `json:"currency"`
	MonthlyAllocationMinor shared.MinorUnits `json:"monthly_allocation_minor"`
	BalanceMinor           shared.MinorUnits `json:"balance_minor"`
	Protected              bool              `json:"protected"`
	Visibility             string            `json:"visibility"`
}

type Create struct {
	Name                   string
	Type                   string
	Currency               string
	MonthlyAllocationMinor int64
	Protected              bool
	Visibility             string
}
