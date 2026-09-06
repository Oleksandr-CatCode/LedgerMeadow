package models

import shared "ledgermeadow/src/shared/types"

type Liability struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	BalanceMinor      shared.MinorUnits `json:"balance_minor"`
	Currency          string            `json:"currency"`
	IncludeInNetWorth bool              `json:"include_in_net_worth"`
}

type Create struct {
	Name              string
	Type              string
	BalanceMinor      int64
	Currency          string
	IncludeInNetWorth bool
}
