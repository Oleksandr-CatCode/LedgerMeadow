package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/transactionimports/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAccountConflict = errors.New("multiple imported accounts match this file")
var ErrCurrentBalanceRequired = errors.New("current balance is required for a new imported account")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Match(ctx context.Context, userID shared.UserID, file models.ParsedFile) (models.AccountMatch, error) {
	accountType, _ := importedAccountFields(file)
	return findImportedAccount(ctx, r.db, userID, file, accountType, false)
}

func (r *Repository) Import(ctx context.Context, userID shared.UserID, command models.ImportCommand) (models.ImportResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("begin RBC CSV import: %w", err)
	}
	defer tx.Rollback(ctx)

	lockKey := fmt.Sprintf("%s:%s:%s:%s", userID, command.File.AccountType, command.File.Mask, command.File.Currency)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return models.ImportResult{}, fmt.Errorf("lock imported account identity: %w", err)
	}

	accountType, subtype := importedAccountFields(command.File)
	match, err := findImportedAccount(ctx, tx, userID, command.File, accountType, true)
	if err != nil {
		return models.ImportResult{}, err
	}
	accountID := string(match.AccountID)
	connectionID := string(match.ConnectionID)
	if !match.Exists {
		if command.CurrentBalanceMinor == nil {
			return models.ImportResult{}, ErrCurrentBalanceRequired
		}
		storedBalance := storedBalance(command.File.AccountType, *command.CurrentBalanceMinor)
		err = tx.QueryRow(ctx, `
			INSERT INTO bank_connections (
				user_id, provider, provider_item_id, institution_name,
				access_token_ciphertext, status, last_sync_at
			) VALUES ($1, 'FILE_IMPORT', $2, 'RBC CSV import', NULL, 'READY', now())
			RETURNING id::text
		`, string(userID), command.ConnectionOpaqueID).Scan(&connectionID)
		if err != nil {
			return models.ImportResult{}, fmt.Errorf("create imported bank connection: %w", err)
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO accounts (
				user_id, bank_connection_id, provider, provider_account_id, name,
				mask, account_type, account_subtype, balance_minor, currency
			) VALUES ($1, $2, 'FILE_IMPORT', $3, $4, $5, $6, $7, $8, $9)
			RETURNING id::text
		`, string(userID), connectionID, command.AccountOpaqueID, command.AccountName,
			command.File.Mask, accountType, subtype, storedBalance, string(command.File.Currency)).Scan(&accountID)
		if err != nil {
			return models.ImportResult{}, fmt.Errorf("create imported account: %w", err)
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE bank_connections
			SET status = 'READY', last_sync_at = now(), updated_at = now()
			WHERE id = $1 AND user_id = $2 AND provider = 'FILE_IMPORT'
		`, connectionID, string(userID)); err != nil {
			return models.ImportResult{}, fmt.Errorf("update imported connection: %w", err)
		}
	}

	inserted := 0
	insertedDelta := int64(0)
	for _, transaction := range command.File.Transactions {
		providerID := transactionProviderID(userID, accountID, transaction.Fingerprint)
		result, err := tx.Exec(ctx, `
			INSERT INTO transactions (
				user_id, bank_connection_id, account_id, provider,
				provider_transaction_id, name, merchant_name, original_description,
				amount_minor, currency, transaction_date, is_pending
			) VALUES ($1, $2, $3, 'FILE_IMPORT', $4, $5, $6, $7, $8, $9, $10, false)
			ON CONFLICT (provider, provider_transaction_id) DO NOTHING
		`, string(userID), connectionID, accountID, providerID, transaction.Name,
			transaction.MerchantName, transaction.OriginalDescription, transaction.AmountMinor,
			string(command.File.Currency), transaction.Date)
		if err != nil {
			return models.ImportResult{}, fmt.Errorf("insert imported transaction: %w", err)
		}
		if result.RowsAffected() == 1 {
			if (transaction.AmountMinor > 0 && insertedDelta > math.MaxInt64-transaction.AmountMinor) ||
				(transaction.AmountMinor < 0 && insertedDelta < math.MinInt64-transaction.AmountMinor) {
				return models.ImportResult{}, errors.New("imported transaction total exceeds balance range")
			}
			inserted++
			insertedDelta += transaction.AmountMinor
		}
	}

	if match.Exists {
		nextBalance := match.BalanceMinor
		if command.CurrentBalanceMinor != nil {
			nextBalance = storedBalance(command.File.AccountType, *command.CurrentBalanceMinor)
		} else {
			if (insertedDelta > 0 && nextBalance > math.MaxInt64-insertedDelta) ||
				(insertedDelta < 0 && nextBalance < math.MinInt64-insertedDelta) {
				return models.ImportResult{}, errors.New("imported account balance exceeds range")
			}
			nextBalance += insertedDelta
		}
		if _, err := tx.Exec(ctx, `
			UPDATE accounts SET balance_minor = $3, updated_at = now()
			WHERE id = $1 AND user_id = $2
		`, accountID, string(userID), nextBalance); err != nil {
			return models.ImportResult{}, fmt.Errorf("update imported account balance: %w", err)
		}
	}

	metadata, err := json.Marshal(map[string]any{
		"imported_count":  inserted,
		"duplicate_count": len(command.File.Transactions) - inserted,
		"date_from":       command.File.DateFrom.Format("2006-01-02"),
		"date_to":         command.File.DateTo.Format("2006-01-02"),
	})
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("encode import audit metadata: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id, metadata)
		VALUES ($1, 'RBC_CSV_IMPORTED', $2, $3, $4)
	`, string(userID), string(entities.Account), accountID, metadata); err != nil {
		return models.ImportResult{}, fmt.Errorf("audit RBC CSV import: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		) VALUES ($1, $2, $1, 'TRANSACTION_ANALYSIS_REQUESTED', $3, '{}'::jsonb)
	`, string(userID), string(entities.User), "transaction-analysis:file-import:"+command.BatchOpaqueID); err != nil {
		return models.ImportResult{}, fmt.Errorf("enqueue imported transaction analysis: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return models.ImportResult{}, fmt.Errorf("commit RBC CSV import: %w", err)
	}
	return models.ImportResult{
		AccountID: shared.AccountID(accountID), ConnectionID: shared.BankConnectionID(connectionID),
		ImportedCount: inserted, DuplicateCount: len(command.File.Transactions) - inserted,
	}, nil
}

type accountQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func findImportedAccount(ctx context.Context, queryer accountQueryer, userID shared.UserID, file models.ParsedFile, accountType string, forUpdate bool) (models.AccountMatch, error) {
	query := `
		SELECT a.id::text, a.bank_connection_id::text, a.name, a.balance_minor
		FROM accounts a
		JOIN bank_connections bc ON bc.id = a.bank_connection_id
		WHERE a.user_id = $1 AND a.provider = 'FILE_IMPORT'
		  AND bc.provider = 'FILE_IMPORT' AND bc.status <> 'DISCONNECTED'
		  AND a.account_type = $2 AND a.mask = $3 AND a.currency = $4
		ORDER BY a.id
		LIMIT 2
	`
	if forUpdate {
		query += ` FOR UPDATE OF a, bc`
	}
	rows, err := queryer.Query(ctx, query, string(userID), accountType, file.Mask, string(file.Currency))
	if err != nil {
		return models.AccountMatch{}, fmt.Errorf("find imported account: %w", err)
	}
	defer rows.Close()
	var match models.AccountMatch
	var accountID, connectionID string
	count := 0
	for rows.Next() {
		count++
		if err := rows.Scan(&accountID, &connectionID, &match.AccountName, &match.BalanceMinor); err != nil {
			return models.AccountMatch{}, fmt.Errorf("scan imported account: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return models.AccountMatch{}, fmt.Errorf("iterate imported accounts: %w", err)
	}
	if count > 1 {
		return models.AccountMatch{}, ErrAccountConflict
	}
	match.Exists = count == 1
	match.AccountID = shared.AccountID(accountID)
	match.ConnectionID = shared.BankConnectionID(connectionID)
	return match, nil
}

func importedAccountFields(file models.ParsedFile) (string, string) {
	if file.AccountType == models.AccountTypeVisa {
		return "credit", "credit card"
	}
	return "depository", "checking"
}

func storedBalance(accountType string, currentBalanceMinor int64) int64 {
	if accountType == models.AccountTypeVisa {
		return -currentBalanceMinor
	}
	return currentBalanceMinor
}

func transactionProviderID(userID shared.UserID, accountID string, fingerprint string) string {
	digest := sha256.Sum256([]byte(string(userID) + ":" + accountID + ":" + fingerprint))
	return "rbc_csv_" + hex.EncodeToString(digest[:])
}
