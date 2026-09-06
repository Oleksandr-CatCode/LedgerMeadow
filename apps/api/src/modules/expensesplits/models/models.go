package models

import shared "ledgermeadow/src/shared/types"

type Participant struct {
	UserID      string            `json:"user_id"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
	Status      string            `json:"status"`
}

type Replace struct {
	PayerAmountMinor int64
	Participants     []Participant
}

type Split struct {
	TransactionID    string            `json:"transaction_id"`
	PayerUserID      string            `json:"payer_user_id"`
	Currency         string            `json:"currency"`
	TotalMinor       shared.MinorUnits `json:"total_minor"`
	PayerAmountMinor shared.MinorUnits `json:"payer_amount_minor"`
	Participants     []Participant     `json:"participants"`
}
