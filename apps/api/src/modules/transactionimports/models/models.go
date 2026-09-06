package models

import (
	"time"

	shared "ledgermeadow/src/shared/types"
)

const (
	AccountTypeVisa     = "VISA"
	AccountTypeChequing = "CHEQUING"
)

type Transaction struct {
	Date                time.Time
	ChequeNumber        string
	Name                string
	MerchantName        *string
	OriginalDescription string
	AmountMinor         int64
	Fingerprint         string
}

type ParsedFile struct {
	AccountType        string
	Mask               string
	Currency           shared.Currency
	Transactions       []Transaction
	DateFrom           time.Time
	DateTo             time.Time
	PositiveTotalMinor int64
	OutflowTotalMinor  int64
}

type Preview struct {
	ParsedFile
	AccountName     string
	ExistingAccount bool
}

type AccountMatch struct {
	Exists       bool
	AccountID    shared.AccountID
	ConnectionID shared.BankConnectionID
	AccountName  string
	BalanceMinor int64
}

type ImportCommand struct {
	AccountName         string
	CurrentBalanceMinor *int64
	ConnectionOpaqueID  string
	AccountOpaqueID     string
	BatchOpaqueID       string
	File                ParsedFile
}

type ImportResult struct {
	AccountID      shared.AccountID
	ConnectionID   shared.BankConnectionID
	ImportedCount  int
	DuplicateCount int
}
