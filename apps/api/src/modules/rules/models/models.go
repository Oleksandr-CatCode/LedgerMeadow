package models

import (
	"encoding/json"
	"time"

	shared "ledgermeadow/src/shared/types"
)

type Rule struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Priority   int               `json:"priority"`
	Enabled    bool              `json:"enabled"`
	Conditions json.RawMessage   `json:"conditions"`
	Actions    json.RawMessage   `json:"actions"`
	MatchCount shared.MinorUnits `json:"match_count"`
	LastRunAt  *time.Time        `json:"last_run_at"`
}

type Condition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type Action struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

type Create struct {
	Name       string
	Enabled    bool
	Conditions []Condition
	Actions    []Action
}

type Update struct {
	Name       *string
	Enabled    *bool
	Conditions *[]Condition
	Actions    *[]Action
}
