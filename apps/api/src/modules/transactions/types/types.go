package types

import (
	"time"

	shared "ledgermeadow/src/shared/types"
)

type Transaction struct {
	ID             shared.TransactionID
	AccountID      shared.AccountID
	AccountName    string
	Name           string
	MerchantName   *string
	AmountMinor    int64
	Currency       shared.Currency
	Date           time.Time
	IsPending      bool
	CategoryID     *string
	CategoryName   *string
	CategorySource string
	IsRecurring    bool
	SpaceID        *string
	SpaceName      *string
	ReviewStatus   string
	Visibility     string
}

type Allocation struct {
	SpaceID     string
	SpaceName   string
	AmountMinor int64
}

type Detail struct {
	Transaction
	OriginalDescription *string
	AuthorizedDate      *time.Time
	Allocations         []Allocation
}

type Update struct {
	CategoryID   *string
	SpaceID      *string
	ReviewStatus *string
	Visibility   *string
}

type ListFilters struct {
	AccountID    *string
	CategoryID   *string
	SpaceID      *string
	ReviewStatus *string
	CurrentMonth bool
	Query        *string
}

type BulkUpdate struct {
	TransactionIDs []shared.TransactionID
	Update         Update
}

type NormalizedTransaction struct {
	ProviderTransactionID        string
	PendingProviderTransactionID *string
	AccountID                    shared.AccountID
	Name                         string
	MerchantName                 *string
	AmountMinor                  int64
	Currency                     shared.Currency
	Date                         time.Time
	AuthorizedDate               *time.Time
	IsPending                    bool
	ProviderCategoryPrimary      *string
	ProviderCategoryDetailed     *string
}
