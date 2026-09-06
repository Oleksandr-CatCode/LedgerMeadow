package types

import (
	"time"

	shared "ledgermeadow/src/shared/types"
)

type ConnectionStatus struct {
	ID              shared.BankConnectionID
	Provider        string
	InstitutionName string
	Status          string
	LastSyncAt      *time.Time
	LastErrorCode   *string
}

type Account struct {
	ID                    shared.AccountID
	Provider              string
	Name                  string
	OfficialName          *string
	Mask                  *string
	Type                  string
	Subtype               *string
	BalanceMinor          int64
	AvailableBalanceMinor *int64
	Currency              shared.Currency
}

type AccountTransaction struct {
	ID           shared.TransactionID
	Name         string
	MerchantName *string
	AmountMinor  int64
	Currency     shared.Currency
	Date         time.Time
	IsPending    bool
}

type AccountDetail struct {
	Account
	ConnectionID shared.BankConnectionID
	Transactions []AccountTransaction
}

type OwnedConnection struct {
	Provider       string
	EncryptedToken []byte
}

type NormalizedAccount struct {
	ProviderAccountID     string
	Name                  string
	OfficialName          *string
	Mask                  *string
	Type                  string
	Subtype               *string
	BalanceMinor          int64
	AvailableBalanceMinor *int64
	Currency              shared.Currency
}

type SyncConnection struct {
	ID             shared.BankConnectionID
	UserID         shared.UserID
	EncryptedToken []byte
	Cursor         string
}
