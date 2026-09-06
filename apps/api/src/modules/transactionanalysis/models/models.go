package models

import "time"

const MaxTransactions = 4096

type Input struct {
	AsOf                time.Time
	Transactions        []Transaction
	Categories          []Category
	CategoryEvidence    []CategoryRecurrenceEvidence
	PrevailingFrequency string
}

type CategoryRecurrenceEvidence struct {
	CategoryID                 string
	SubscriptionCount          uint32
	BillCount                  uint32
	ConfirmedSubscriptionCount uint32
	ConfirmedBillCount         uint32
}

type Category struct {
	ID                string
	Name              string
	Type              string
	ParentName        *string
	RecurringKindHint *string
}

type LearningSignal struct {
	LearningKey                 string
	CategoryID                  string
	PersonalObservationCount    uint32
	GlobalUserCount             uint32
	GlobalTotalContributorCount uint32
}

type Transaction struct {
	ID                       string
	AccountID                string
	Name                     string
	MerchantName             *string
	OriginalDescription      *string
	AmountMinor              int64
	Currency                 string
	Date                     time.Time
	ProviderCategoryPrimary  *string
	ProviderCategoryDetailed *string
	LearningKey              *string
	CategoryID               *string
	CategoryType             *string
	CategorySource           string
	AccountKind              string
}

type LearningKeyAssignment struct {
	TransactionID string `json:"transaction_id"`
	LearningKey   string `json:"learning_key"`
}

type CategoryAssignment struct {
	TransactionID         string `json:"transaction_id"`
	CategoryID            string `json:"category_id"`
	ConfidenceBasisPoints uint32 `json:"confidence_basis_points"`
}

type CategoryReview struct {
	TransactionID         string `json:"transaction_id"`
	SuggestedCategoryID   string `json:"suggested_category_id,omitempty"`
	ConfidenceBasisPoints uint32 `json:"confidence_basis_points"`
	Explanation           string `json:"explanation"`
}

type RecurringCandidate struct {
	DetectionKey          string   `json:"detection_key"`
	Name                  string   `json:"name"`
	Kind                  string   `json:"kind"`
	Currency              string   `json:"currency"`
	Frequency             string   `json:"frequency"`
	ExpectedAmountMinor   int64    `json:"expected_amount_minor"`
	NextExpectedAt        string   `json:"next_expected_at"`
	ConfidenceBasisPoints uint32   `json:"confidence_basis_points"`
	OccurrenceCount       uint32   `json:"occurrence_count"`
	AccountID             string   `json:"account_id"`
	CategoryID            string   `json:"category_id,omitempty"`
	SupportingIDs         []string `json:"supporting_ids"`
	Explanation           string   `json:"explanation"`
	Status                string   `json:"status"`
	ObservedAmountMinor   *int64   `json:"observed_amount_minor,omitempty"`
	AmountObservationIDs  []string `json:"amount_observation_ids,omitempty"`
}

type Result struct {
	LearningKeys    []LearningKeyAssignment
	Categories      []CategoryAssignment
	CategoryReviews []CategoryReview
	Recurring       []RecurringCandidate
}
