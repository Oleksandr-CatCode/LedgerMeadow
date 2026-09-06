package models

import shared "ledgermeadow/src/shared/types"

type MemberSummary struct {
	Currency        string             `json:"currency"`
	PaidMinor       shared.MinorUnits  `json:"paid_minor"`
	OwedMinor       shared.MinorUnits  `json:"owed_minor"`
	DifferenceMinor shared.MinorUnits  `json:"difference_minor"`
	SettlementMinor *shared.MinorUnits `json:"settlement_minor"`
}

type Member struct {
	UserID      string          `json:"user_id"`
	DisplayName string          `json:"display_name"`
	Role        string          `json:"role"`
	Summaries   []MemberSummary `json:"summaries"`
}

type SpendingSummary struct {
	Currency           string            `json:"currency"`
	TotalSpendingMinor shared.MinorUnits `json:"total_spending_minor"`
}

type SharedSpace struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Currency     string            `json:"currency"`
	BalanceMinor shared.MinorUnits `json:"balance_minor"`
}

type SplitParticipant struct {
	UserID      string            `json:"user_id"`
	DisplayName string            `json:"display_name"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
	Status      string            `json:"status"`
}

type SharedTransaction struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	MerchantName     *string            `json:"merchant_name"`
	AmountMinor      shared.MinorUnits  `json:"amount_minor"`
	Currency         string             `json:"currency"`
	Date             string             `json:"date"`
	PayerUserID      string             `json:"payer_user_id"`
	PayerDisplayName string             `json:"payer_display_name"`
	PayerAmountMinor shared.MinorUnits  `json:"payer_amount_minor"`
	Participants     []SplitParticipant `json:"participants"`
}

type Household struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Members            []Member            `json:"members"`
	SpendingSummaries  []SpendingSummary   `json:"spending_summaries"`
	SharedSpaces       []SharedSpace       `json:"shared_spaces"`
	SharedTransactions []SharedTransaction `json:"shared_transactions"`
}
