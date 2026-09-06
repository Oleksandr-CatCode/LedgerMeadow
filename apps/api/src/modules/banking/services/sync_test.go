package services

import (
	"context"
	"errors"
	"testing"

	banking "ledgermeadow/src/modules/banking/types"
	"ledgermeadow/src/modules/platform/plaid"
	transactions "ledgermeadow/src/modules/transactions/types"
	shared "ledgermeadow/src/shared/types"
)

func TestNormalizePlaidSigns(t *testing.T) {
	t.Parallel()

	currency := "CAD"
	current := plaid.Decimal("100.25")
	investmentCurrent := plaid.Decimal("123.4567")
	accounts, err := normalizeAccounts([]plaid.Account{{
		AccountID: "account-1", Name: "Credit", Type: "credit",
		Balances: plaid.Balances{Current: &current, ISOCurrencyCode: &currency},
	}, {
		AccountID: "account-2", Name: "Investment", Type: "investment",
		Balances: plaid.Balances{Current: &investmentCurrent, ISOCurrencyCode: &currency},
	}})
	if err != nil {
		t.Fatalf("normalizeAccounts() error = %v", err)
	}
	if accounts[0].BalanceMinor != -10025 {
		t.Fatalf("credit balance = %d, want -10025", accounts[0].BalanceMinor)
	}
	if accounts[1].BalanceMinor != 12346 {
		t.Fatalf("investment balance = %d, want rounded 12346", accounts[1].BalanceMinor)
	}

	accountIDs := map[string]shared.AccountID{"account-1": "internal-account"}
	transactions, err := normalizeTransactions([]plaid.Transaction{{
		TransactionID: "transaction-1", AccountID: "account-1", Name: "Coffee",
		Amount: plaid.Decimal("4.50"), ISOCurrencyCode: &currency, Date: "2026-08-19",
	}}, accountIDs)
	if err != nil {
		t.Fatalf("normalizeTransactions() error = %v", err)
	}
	if transactions[0].AmountMinor != -450 {
		t.Fatalf("transaction amount = %d, want -450", transactions[0].AmountMinor)
	}
}

func TestSynchronizeDoesNotPersistIntermediateCursorWhenProviderPaginationFails(t *testing.T) {
	t.Parallel()

	provider := &paginationFailureProvider{}
	bankRepository := &syncBankRepository{}
	transactionRepository := &recordingTransactionRepository{}
	service := NewSyncService(provider, staticSyncCipher{}, bankRepository, transactionRepository)

	err := service.Synchronize(context.Background(), "connection-1")
	if err == nil {
		t.Fatal("Synchronize() error = nil, want provider pagination failure")
	}
	if transactionRepository.applyCalls != 0 {
		t.Fatalf("ApplySyncPage() calls = %d, want 0 before a stable page sequence", transactionRepository.applyCalls)
	}
}

func TestSynchronizeDoesNotPersistProviderErrorDetails(t *testing.T) {
	t.Parallel()

	provider := &accountFailureProvider{}
	bankRepository := &syncBankRepository{}
	service := NewSyncService(provider, staticSyncCipher{}, bankRepository, &recordingTransactionRepository{})

	if err := service.Synchronize(context.Background(), "connection-1"); err == nil {
		t.Fatal("Synchronize() error = nil, want provider failure")
	}
	if bankRepository.errorCode != "SYNC_FAILED" {
		t.Fatal("provider error details reached the persisted, client-visible sync status")
	}
}

type accountFailureProvider struct {
	paginationFailureProvider
}

func (p *accountFailureProvider) GetAccounts(context.Context, string) ([]plaid.Account, error) {
	return nil, &plaid.APIError{Code: "synthetic-private-provider-detail"}
}

type paginationFailureProvider struct {
	syncCalls int
}

func (p *paginationFailureProvider) GetAccounts(context.Context, string) ([]plaid.Account, error) {
	return nil, nil
}

func (p *paginationFailureProvider) SyncTransactions(context.Context, string, string, int) (plaid.SyncPage, error) {
	p.syncCalls++
	if p.syncCalls == 1 {
		return plaid.SyncPage{NextCursor: "intermediate-cursor", HasMore: true}, nil
	}
	return plaid.SyncPage{}, errors.New("pagination changed")
}

type staticSyncCipher struct{}

func (staticSyncCipher) Decrypt([]byte) (string, error) {
	return "provider-access-token", nil
}

type syncBankRepository struct {
	errorCode string
}

func (r *syncBankRepository) LoadForSync(context.Context, shared.BankConnectionID) (banking.SyncConnection, error) {
	return banking.SyncConnection{ID: "connection-1", UserID: "user-1", EncryptedToken: []byte("encrypted")}, nil
}

func (r *syncBankRepository) MarkSyncing(context.Context, shared.BankConnectionID) error { return nil }
func (r *syncBankRepository) MarkReady(context.Context, shared.BankConnectionID) error   { return nil }
func (r *syncBankRepository) MarkError(_ context.Context, _ shared.BankConnectionID, code string) error {
	r.errorCode = code
	return nil
}
func (r *syncBankRepository) UpsertAccounts(
	context.Context,
	banking.SyncConnection,
	[]banking.NormalizedAccount,
) (map[string]shared.AccountID, error) {
	return map[string]shared.AccountID{}, nil
}

type recordingTransactionRepository struct {
	applyCalls int
}

func (r *recordingTransactionRepository) ApplySyncPage(
	context.Context,
	banking.SyncConnection,
	[]transactions.NormalizedTransaction,
	[]string,
	string,
) error {
	r.applyCalls++
	return nil
}
