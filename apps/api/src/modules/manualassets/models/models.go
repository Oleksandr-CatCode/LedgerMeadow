package models

import shared "ledgermeadow/src/shared/types"

type Asset struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Type              string            `json:"type"`
	ValueMinor        shared.MinorUnits `json:"value_minor"`
	Currency          string            `json:"currency"`
	IncludeInNetWorth bool              `json:"include_in_net_worth"`
}
type Create struct {
	Name              string
	Type              string
	ValueMinor        int64
	Currency          string
	IncludeInNetWorth bool
}
