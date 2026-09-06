package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	banking "ledgermeadow/src/modules/banking/types"
	"ledgermeadow/src/modules/platform/plaid"
	transactions "ledgermeadow/src/modules/transactions/types"
	shared "ledgermeadow/src/shared/types"
)

const maxSyncPages = 100

type SyncProvider interface {
	GetAccounts(ctx context.Context, accessToken string) ([]plaid.Account, error)
	SyncTransactions(ctx context.Context, accessToken string, cursor string, count int) (plaid.SyncPage, error)
}

type SyncCipher interface {
	Decrypt(ciphertext []byte) (string, error)
}

type SyncRepository interface {
	LoadForSync(ctx context.Context, connectionID shared.BankConnectionID) (banking.SyncConnection, error)
	MarkSyncing(ctx context.Context, connectionID shared.BankConnectionID) error
	MarkReady(ctx context.Context, connectionID shared.BankConnectionID) error
	MarkError(ctx context.Context, connectionID shared.BankConnectionID, errorCode string) error
	UpsertAccounts(ctx context.Context, connection banking.SyncConnection, accounts []banking.NormalizedAccount) (map[string]shared.AccountID, error)
}

type TransactionSyncRepository interface {
	ApplySyncPage(
		ctx context.Context,
		connection banking.SyncConnection,
		upserts []transactions.NormalizedTransaction,
		removedProviderIDs []string,
		nextCursor string,
	) error
}

type SyncService struct {
	provider     SyncProvider
	cipher       SyncCipher
	banking      SyncRepository
	transactions TransactionSyncRepository
}

type normalizedSyncPage struct {
	upserts []transactions.NormalizedTransaction
	removed []string
	cursor  string
}

func NewSyncService(
	provider SyncProvider,
	cipher SyncCipher,
	bankingRepository SyncRepository,
	transactionRepository TransactionSyncRepository,
) *SyncService {
	return &SyncService{
		provider: provider, cipher: cipher,
		banking: bankingRepository, transactions: transactionRepository,
	}
}

func (s *SyncService) Synchronize(ctx context.Context, connectionID shared.BankConnectionID) (syncErr error) {
	connection, err := s.banking.LoadForSync(ctx, connectionID)
	if err != nil {
		return err
	}
	if err := s.banking.MarkSyncing(ctx, connectionID); err != nil {
		return fmt.Errorf("mark connection syncing: %w", err)
	}
	defer func() {
		if syncErr != nil {
			_ = s.banking.MarkError(context.WithoutCancel(ctx), connectionID, safeSyncErrorCode(syncErr))
		}
	}()

	accessToken, err := s.cipher.Decrypt(connection.EncryptedToken)
	if err != nil {
		return fmt.Errorf("decrypt provider token: %w", err)
	}
	providerAccounts, err := s.provider.GetAccounts(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("get provider accounts: %w", err)
	}
	normalizedAccounts, err := normalizeAccounts(providerAccounts)
	if err != nil {
		return err
	}
	accountIDs, err := s.banking.UpsertAccounts(ctx, connection, normalizedAccounts)
	if err != nil {
		return fmt.Errorf("persist accounts: %w", err)
	}

	cursor := connection.Cursor
	pages := make([]normalizedSyncPage, 0, 4)
	complete := false
	for pageNumber := 0; pageNumber < maxSyncPages; pageNumber++ {
		page, err := s.provider.SyncTransactions(ctx, accessToken, cursor, 500)
		if err != nil {
			return fmt.Errorf("sync provider transactions: %w", err)
		}
		upserts, err := normalizeTransactions(append(page.Added, page.Modified...), accountIDs)
		if err != nil {
			return err
		}
		removed := make([]string, 0, len(page.Removed))
		for _, item := range page.Removed {
			if item.TransactionID != "" {
				removed = append(removed, item.TransactionID)
			}
		}
		pages = append(pages, normalizedSyncPage{upserts: upserts, removed: removed, cursor: page.NextCursor})
		cursor = page.NextCursor
		if !page.HasMore {
			complete = true
			break
		}
	}
	if !complete {
		return errors.New("provider transaction sync exceeded the page bound")
	}
	for _, page := range pages {
		if err := s.transactions.ApplySyncPage(ctx, connection, page.upserts, page.removed, page.cursor); err != nil {
			return err
		}
	}
	if err := s.banking.MarkReady(ctx, connectionID); err != nil {
		return fmt.Errorf("mark connection ready: %w", err)
	}
	return nil
}

func normalizeAccounts(accounts []plaid.Account) ([]banking.NormalizedAccount, error) {
	normalized := make([]banking.NormalizedAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.AccountID == "" || account.Name == "" || account.Balances.Current == nil ||
			account.Balances.ISOCurrencyCode == nil {
			return nil, errors.New("provider account is missing required normalized fields")
		}
		balance, err := shared.ParseProviderAccountBalance(string(*account.Balances.Current), *account.Balances.ISOCurrencyCode)
		if err != nil {
			return nil, err
		}
		if account.Type == "credit" || account.Type == "loan" {
			if balance.AmountMinor == math.MinInt64 {
				return nil, errors.New("liability balance cannot be negated")
			}
			balance.AmountMinor = -balance.AmountMinor
		}
		var available *int64
		if account.Balances.Available != nil {
			money, err := shared.ParseProviderAccountBalance(string(*account.Balances.Available), *account.Balances.ISOCurrencyCode)
			if err != nil {
				return nil, err
			}
			available = &money.AmountMinor
		}
		normalized = append(normalized, banking.NormalizedAccount{
			ProviderAccountID: account.AccountID, Name: account.Name,
			OfficialName: account.OfficialName, Mask: account.Mask, Type: account.Type,
			Subtype: account.Subtype, BalanceMinor: balance.AmountMinor,
			AvailableBalanceMinor: available, Currency: balance.Currency,
		})
	}
	return normalized, nil
}

func normalizeTransactions(
	providerTransactions []plaid.Transaction,
	accountIDs map[string]shared.AccountID,
) ([]transactions.NormalizedTransaction, error) {
	normalized := make([]transactions.NormalizedTransaction, 0, len(providerTransactions))
	for _, transaction := range providerTransactions {
		accountID, ok := accountIDs[transaction.AccountID]
		if !ok || transaction.TransactionID == "" || transaction.ISOCurrencyCode == nil {
			return nil, errors.New("provider transaction references unknown or incomplete data")
		}
		money, err := shared.ParseProviderDecimal(string(transaction.Amount), *transaction.ISOCurrencyCode)
		if err != nil {
			return nil, err
		}
		if money.AmountMinor == math.MinInt64 {
			return nil, errors.New("provider transaction amount cannot be negated")
		}
		money.AmountMinor = -money.AmountMinor
		transactionDate, err := time.Parse("2006-01-02", transaction.Date)
		if err != nil {
			return nil, errors.New("provider transaction date is invalid")
		}
		var authorizedDate *time.Time
		if transaction.AuthorizedDate != nil {
			parsed, err := time.Parse("2006-01-02", *transaction.AuthorizedDate)
			if err != nil {
				return nil, errors.New("provider authorized date is invalid")
			}
			authorizedDate = &parsed
		}
		name := strings.TrimSpace(transaction.Name)
		if name == "" || len(name) > 500 {
			return nil, errors.New("provider transaction name is invalid")
		}
		normalized = append(normalized, transactions.NormalizedTransaction{
			ProviderTransactionID:        transaction.TransactionID,
			PendingProviderTransactionID: transaction.PendingTransactionID,
			AccountID:                    accountID, Name: name, MerchantName: transaction.MerchantName,
			AmountMinor: money.AmountMinor, Currency: money.Currency,
			Date: transactionDate, AuthorizedDate: authorizedDate, IsPending: transaction.Pending,
			ProviderCategoryPrimary:  providerCategoryPrimary(transaction.PersonalFinanceCategory),
			ProviderCategoryDetailed: providerCategoryDetailed(transaction.PersonalFinanceCategory),
		})
	}
	return normalized, nil
}

func providerCategoryPrimary(category *plaid.PersonalFinanceCategory) *string {
	if category == nil || strings.TrimSpace(category.Primary) == "" {
		return nil
	}
	value := strings.TrimSpace(category.Primary)
	return &value
}

func providerCategoryDetailed(category *plaid.PersonalFinanceCategory) *string {
	if category == nil || strings.TrimSpace(category.Detailed) == "" {
		return nil
	}
	value := strings.TrimSpace(category.Detailed)
	return &value
}

func safeSyncErrorCode(err error) string {
	if errors.Is(err, shared.ErrUnsupportedCurrency) {
		return "UNSUPPORTED_CURRENCY"
	}
	return "SYNC_FAILED"
}
