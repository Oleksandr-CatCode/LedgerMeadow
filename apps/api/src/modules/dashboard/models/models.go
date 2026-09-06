package models

import (
	shared "ledgermeadow/src/shared/types"
	"time"
)

type Projection struct {
	TotalMinor             shared.MinorUnits `json:"total_minor"`
	AvailableMinor         shared.MinorUnits `json:"available_minor"`
	ProtectedMinor         shared.MinorUnits `json:"protected_minor"`
	ProjectedMonthEndMinor shared.MinorUnits `json:"projected_month_end_minor"`
	Currency               string            `json:"currency"`
	Status                 string            `json:"status"`
	Breakdown              []BreakdownItem   `json:"breakdown"`
	UpdatedAt              time.Time         `json:"updated_at"`
}
type BreakdownItem struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Kind        string            `json:"kind"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
	EffectMinor shared.MinorUnits `json:"effect_minor"`
}
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
type Upcoming struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	AmountMinor shared.MinorUnits `json:"amount_minor"`
	Currency    string            `json:"currency"`
	Date        string            `json:"date"`
}
type Budget struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	LimitMinor shared.MinorUnits `json:"limit_minor"`
	SpentMinor shared.MinorUnits `json:"spent_minor"`
	Currency   string            `json:"currency"`
}
type MonthlyPlan struct {
	PlannedMinor shared.MinorUnits `json:"planned_minor"`
	SpentMinor   shared.MinorUnits `json:"spent_minor"`
	Currency     string            `json:"currency"`
}
type Transaction struct {
	ID           string            `json:"id"`
	AccountID    string            `json:"account_id"`
	AccountName  string            `json:"account_name"`
	Name         string            `json:"name"`
	MerchantName *string           `json:"merchant_name"`
	AmountMinor  shared.MinorUnits `json:"amount_minor"`
	Currency     string            `json:"currency"`
	Date         string            `json:"date"`
	Status       string            `json:"status"`
	CategoryName *string           `json:"category_name"`
	SpaceName    *string           `json:"space_name"`
	ReviewStatus string            `json:"review_status"`
	Visibility   string            `json:"visibility"`
}
type Dashboard struct {
	Projections        []Projection  `json:"projections"`
	Spaces             []Space       `json:"spaces"`
	Budgets            []Budget      `json:"budgets"`
	MonthlyPlans       []MonthlyPlan `json:"monthly_plans"`
	Upcoming           []Upcoming    `json:"upcoming"`
	AttentionCount     int           `json:"attention_count"`
	RecentTransactions []Transaction `json:"recent_transactions"`
}
