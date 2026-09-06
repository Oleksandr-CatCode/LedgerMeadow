package models

import shared "ledgermeadow/src/shared/types"

type Account struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	BalanceMinor shared.MinorUnits `json:"balance_minor"`
	Currency     string            `json:"currency"`
}

type ManualItem struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
	Currency    string            `json:"currency"`
}

type Snapshot struct {
	Date                  string             `json:"date"`
	ValueMinor            shared.MinorUnits  `json:"value_minor"`
	Currency              string             `json:"currency"`
	TotalAssetsMinor      *shared.MinorUnits `json:"total_assets_minor,omitempty"`
	TotalLiabilitiesMinor *shared.MinorUnits `json:"total_liabilities_minor,omitempty"`
}

type NetWorth struct {
	Accounts    []Account    `json:"accounts"`
	Assets      []ManualItem `json:"assets"`
	Liabilities []ManualItem `json:"liabilities"`
	Snapshots   []Snapshot   `json:"snapshots"`
}
