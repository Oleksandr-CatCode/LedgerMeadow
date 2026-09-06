package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"ledgermeadow/src/entities"
	banking "ledgermeadow/src/modules/banking/types"
	categoryrepo "ledgermeadow/src/modules/categories/repository"
	transactions "ledgermeadow/src/modules/transactions/types"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

var ErrInvalidCursor = errors.New("invalid transaction cursor")
var ErrNotFound = errors.New("transaction not found")
var ErrRelatedEntityNotFound = errors.New("related transaction entity not found")
var ErrHouseholdRequired = errors.New("household membership is required")
var ErrExpenseSplitExists = errors.New("remove the expense split before making the transaction private")

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ApplySyncPage(
	ctx context.Context,
	connection banking.SyncConnection,
	upserts []transactions.NormalizedTransaction,
	removedProviderIDs []string,
	nextCursor string,
) error {
	if len(upserts)+len(removedProviderIDs) > 1000 {
		return errors.New("transaction sync page exceeds the configured bound")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction sync page: %w", err)
	}
	defer tx.Rollback(ctx)
	var activeConnectionID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM bank_connections
		WHERE id = $1 AND user_id = $2 AND status <> 'DISCONNECTED'
		FOR UPDATE
	`, string(connection.ID), string(connection.UserID)).Scan(&activeConnectionID); err != nil {
		return fmt.Errorf("lock active bank connection for transaction sync: %w", err)
	}

	for _, transaction := range upserts {
		reconciled := false
		if transaction.PendingProviderTransactionID != nil && *transaction.PendingProviderTransactionID != "" {
			if _, err := prepareProviderMutation(
				ctx, tx, connection, *transaction.PendingProviderTransactionID, transaction, false,
			); err != nil {
				return err
			}
			result, err := tx.Exec(ctx, `
				UPDATE transactions SET
					provider_transaction_id = $4,
					pending_provider_transaction_id = $5,
					account_id = $6, name = $7, merchant_name = $8,
					amount_minor = $9, currency = $10, transaction_date = $11,
					authorized_date = $12, is_pending = $13,
					provider_category_primary = $14, provider_category_detailed = $15,
					removed_at = NULL, updated_at = now()
				WHERE user_id = $1 AND bank_connection_id = $2 AND provider = 'PLAID'
				  AND provider_transaction_id = $3
			`, string(connection.UserID), string(connection.ID), *transaction.PendingProviderTransactionID,
				transaction.ProviderTransactionID, transaction.PendingProviderTransactionID,
				string(transaction.AccountID), transaction.Name, transaction.MerchantName,
				transaction.AmountMinor, string(transaction.Currency), transaction.Date,
				transaction.AuthorizedDate, transaction.IsPending,
				transaction.ProviderCategoryPrimary, transaction.ProviderCategoryDetailed)
			if err != nil {
				return fmt.Errorf("reconcile pending transaction: %w", err)
			}
			reconciled = result.RowsAffected() == 1
		}
		if reconciled {
			continue
		}
		if _, err := prepareProviderMutation(
			ctx, tx, connection, transaction.ProviderTransactionID, transaction, false,
		); err != nil {
			return err
		}

		result, err := tx.Exec(ctx, `
			INSERT INTO transactions (
				user_id, bank_connection_id, account_id, provider,
				provider_transaction_id, pending_provider_transaction_id,
				name, merchant_name, amount_minor, currency, transaction_date,
				authorized_date, is_pending, provider_category_primary, provider_category_detailed
			) VALUES ($1, $2, $3, 'PLAID', $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (provider, provider_transaction_id) DO UPDATE SET
				account_id = EXCLUDED.account_id,
				pending_provider_transaction_id = EXCLUDED.pending_provider_transaction_id,
				name = EXCLUDED.name, merchant_name = EXCLUDED.merchant_name,
				amount_minor = EXCLUDED.amount_minor, currency = EXCLUDED.currency,
				transaction_date = EXCLUDED.transaction_date,
				authorized_date = EXCLUDED.authorized_date,
				is_pending = EXCLUDED.is_pending,
				provider_category_primary = EXCLUDED.provider_category_primary,
				provider_category_detailed = EXCLUDED.provider_category_detailed,
				removed_at = NULL, updated_at = now()
			WHERE transactions.user_id = EXCLUDED.user_id
			  AND transactions.bank_connection_id = EXCLUDED.bank_connection_id
		`, string(connection.UserID), string(connection.ID), string(transaction.AccountID),
			transaction.ProviderTransactionID, transaction.PendingProviderTransactionID,
			transaction.Name, transaction.MerchantName, transaction.AmountMinor,
			string(transaction.Currency), transaction.Date, transaction.AuthorizedDate,
			transaction.IsPending, transaction.ProviderCategoryPrimary,
			transaction.ProviderCategoryDetailed)
		if err != nil {
			return fmt.Errorf("upsert transaction: %w", err)
		}
		if result.RowsAffected() != 1 {
			return errors.New("provider transaction ownership conflict")
		}
	}

	for _, providerTransactionID := range removedProviderIDs {
		if _, err := prepareProviderMutation(ctx, tx, connection, providerTransactionID, transactions.NormalizedTransaction{}, true); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE transactions SET removed_at = now(), updated_at = now()
			WHERE user_id = $1 AND bank_connection_id = $2
			  AND provider = 'PLAID' AND provider_transaction_id = $3
		`, string(connection.UserID), string(connection.ID), providerTransactionID); err != nil {
			return fmt.Errorf("remove provider transaction: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE sync_cursors SET cursor = $2, updated_at = now()
		WHERE bank_connection_id = $1
	`, string(connection.ID), nextCursor); err != nil {
		return fmt.Errorf("advance transaction cursor: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction sync page: %w", err)
	}
	return nil
}

type providerTransactionState struct {
	id          string
	amountMinor int64
	currency    string
	date        time.Time
	categoryID  *string
	visibility  string
	removed     bool
}

type providerAllocation struct {
	spaceID     string
	amountMinor int64
}

func prepareProviderMutation(
	ctx context.Context,
	tx pgx.Tx,
	connection banking.SyncConnection,
	providerTransactionID string,
	next transactions.NormalizedTransaction,
	remove bool,
) (bool, error) {
	var current providerTransactionState
	var removedAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT id::text, amount_minor, currency, transaction_date, category_id::text, visibility, removed_at
		FROM transactions
		WHERE user_id = $1 AND bank_connection_id = $2
		  AND provider = 'PLAID' AND provider_transaction_id = $3
		FOR UPDATE
	`, string(connection.UserID), string(connection.ID), providerTransactionID).Scan(
		&current.id, &current.amountMinor, &current.currency, &current.date, &current.categoryID, &current.visibility, &removedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock provider transaction derivative: %w", err)
	}
	current.removed = removedAt != nil
	if (remove && current.removed) || (!remove && !current.removed && current.amountMinor == next.AmountMinor &&
		current.currency == string(next.Currency) && current.date.Equal(next.Date)) {
		return true, nil
	}

	allocations, err := loadProviderAllocations(ctx, tx, current.id)
	if err != nil {
		return false, err
	}
	if err := updateProviderSpaceDerivatives(ctx, tx, connection.UserID, current, next, allocations, remove); err != nil {
		return false, err
	}
	if current.amountMinor < 0 && current.amountMinor != math.MinInt64 && !current.removed {
		if err := adjustBudgetSpending(
			ctx, tx, connection.UserID, current.categoryID, nil, current.date, current.currency, -current.amountMinor, false,
		); err != nil {
			return false, err
		}
	}
	if !remove && next.AmountMinor < 0 && next.AmountMinor != math.MinInt64 {
		if err := adjustBudgetSpending(
			ctx, tx, connection.UserID, current.categoryID, nil, next.Date, string(next.Currency), -next.AmountMinor, true,
		); err != nil {
			return false, err
		}
	}
	if current.visibility == "HOUSEHOLD" && (remove || current.removed ||
		current.amountMinor != next.AmountMinor || current.currency != string(next.Currency)) {
		if err := updateProviderHouseholdDerivatives(ctx, tx, connection.UserID, current, next, remove); err != nil {
			return false, err
		}
	}
	return true, nil
}

func loadProviderAllocations(ctx context.Context, tx pgx.Tx, transactionID string) ([]providerAllocation, error) {
	rows, err := tx.Query(ctx, `
		SELECT space_id::text, amount_minor
		FROM transaction_allocations
		WHERE transaction_id = $1
		ORDER BY space_id
		LIMIT 51
	`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("load provider transaction allocations: %w", err)
	}
	defer rows.Close()
	allocations := make([]providerAllocation, 0, 1)
	for rows.Next() {
		var allocation providerAllocation
		if err := rows.Scan(&allocation.spaceID, &allocation.amountMinor); err != nil {
			return nil, fmt.Errorf("scan provider transaction allocation: %w", err)
		}
		allocations = append(allocations, allocation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provider transaction allocations: %w", err)
	}
	if len(allocations) > 50 {
		return nil, errors.New("provider transaction allocation count exceeds bound")
	}
	return allocations, nil
}

func updateProviderSpaceDerivatives(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	current providerTransactionState,
	next transactions.NormalizedTransaction,
	allocations []providerAllocation,
	remove bool,
) error {
	if current.removed || len(allocations) == 0 {
		return nil
	}
	for index, allocation := range allocations {
		if allocation.amountMinor < 0 && allocation.amountMinor != math.MinInt64 {
			spaceID := allocation.spaceID
			if err := adjustBudgetSpending(
				ctx, tx, userID, nil, &spaceID, current.date, current.currency, -allocation.amountMinor, false,
			); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spaces
			SET balance_minor = CASE
			        WHEN balance_period_start = date_trunc('month', CURRENT_DATE)::date
			            THEN balance_minor - $3
			        ELSE monthly_allocation_minor - $3
			    END,
			    balance_period_start = date_trunc('month', CURRENT_DATE)::date,
			    updated_at = now()
			WHERE id = $1 AND user_id = $2
			  AND $4::date >= date_trunc('month', CURRENT_DATE)::date
		`, allocation.spaceID, string(userID), allocation.amountMinor, current.date); err != nil {
			return fmt.Errorf("reverse provider transaction Space balance: %w", err)
		}
		if len(allocations) == 1 && allocation.amountMinor == current.amountMinor && !remove {
			allocations[index].amountMinor = next.AmountMinor
			if _, err := tx.Exec(ctx, `
				UPDATE transaction_allocations
				SET amount_minor = $2, transaction_date = $3, updated_at = now()
				WHERE transaction_id = $1
			`, current.id, next.AmountMinor, next.Date); err != nil {
				return fmt.Errorf("resize provider transaction allocation: %w", err)
			}
		}
	}
	currencyChanged := current.currency != string(next.Currency)
	if remove || currencyChanged {
		if _, err := tx.Exec(ctx, `DELETE FROM transaction_allocations WHERE transaction_id = $1`, current.id); err != nil {
			return fmt.Errorf("clear invalid provider transaction allocations: %w", err)
		}
		if currencyChanged && len(allocations) > 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO inbox_items (
					user_id, item_type, priority, entity_type, entity_id, payload
				) VALUES ($1, 'TRANSACTION_REVIEW', 'HIGH', $2, $3,
				          jsonb_build_object('reason', 'TRANSACTION_CURRENCY_CHANGED', 'space_allocation_cleared', true))
			`, string(userID), string(entities.Transaction), current.id); err != nil {
				return fmt.Errorf("queue changed allocation review: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE users SET open_inbox_count = open_inbox_count + 1, updated_at = now()
				WHERE id = $1
			`, string(userID)); err != nil {
				return fmt.Errorf("increment changed allocation review count: %w", err)
			}
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE transaction_allocations SET transaction_date=$2,updated_at=now() WHERE transaction_id=$1`, current.id, next.Date); err != nil {
		return fmt.Errorf("move provider transaction allocation date: %w", err)
	}
	for _, allocation := range allocations {
		if allocation.amountMinor < 0 && allocation.amountMinor != math.MinInt64 {
			spaceID := allocation.spaceID
			if err := adjustBudgetSpending(
				ctx, tx, userID, nil, &spaceID, next.Date, string(next.Currency), -allocation.amountMinor, true,
			); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE spaces
			SET balance_minor = CASE
			        WHEN balance_period_start = date_trunc('month', CURRENT_DATE)::date
			            THEN balance_minor + $3
			        ELSE monthly_allocation_minor + $3
			    END,
			    balance_period_start = date_trunc('month', CURRENT_DATE)::date,
			    updated_at = now()
			WHERE id = $1 AND user_id = $2
			  AND $4::date >= date_trunc('month', CURRENT_DATE)::date
		`, allocation.spaceID, string(userID), allocation.amountMinor, next.Date); err != nil {
			return fmt.Errorf("apply provider transaction Space balance: %w", err)
		}
	}
	return nil
}

func updateProviderHouseholdDerivatives(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	current providerTransactionState,
	next transactions.NormalizedTransaction,
	remove bool,
) error {
	var householdID string
	err := tx.QueryRow(ctx, `
		SELECT household_id::text FROM household_members WHERE user_id = $1 FOR SHARE
	`, string(userID)).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `DELETE FROM expense_splits WHERE transaction_id = $1`, current.id); err != nil {
			return fmt.Errorf("clear detached household split: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load provider transaction household: %w", err)
	}
	splitCleared, err := clearProviderExpenseSplit(ctx, tx, householdID, string(userID), current.id, current.currency)
	if err != nil {
		return err
	}
	if current.amountMinor < 0 && current.amountMinor != math.MinInt64 && !current.removed {
		if err := deactivateProviderHouseholdExpense(
			ctx, tx, householdID, string(userID), current.currency, -current.amountMinor,
		); err != nil {
			return err
		}
	}
	if !remove && next.AmountMinor < 0 && next.AmountMinor != math.MinInt64 {
		if err := activateProviderHouseholdExpense(
			ctx, tx, householdID, string(userID), string(next.Currency), -next.AmountMinor,
		); err != nil {
			return err
		}
	}
	if splitCleared {
		if _, err := tx.Exec(ctx, `
			INSERT INTO inbox_items (
				user_id, item_type, priority, entity_type, entity_id, payload
			) VALUES ($1, 'POSSIBLE_SHARED_EXPENSE', 'NORMAL', $2, $3,
			          jsonb_build_object('reason', 'PROVIDER_TRANSACTION_CHANGED', 'previous_split_cleared', true))
		`, string(userID), string(entities.Transaction), current.id); err != nil {
			return fmt.Errorf("queue changed expense split review: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET open_inbox_count = open_inbox_count + 1, updated_at = now()
			WHERE id = $1
		`, string(userID)); err != nil {
			return fmt.Errorf("increment changed expense split review count: %w", err)
		}
	}
	return nil
}

func clearProviderExpenseSplit(
	ctx context.Context,
	tx pgx.Tx,
	householdID string,
	userID string,
	transactionID string,
	currency string,
) (bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT participant_user_id::text, amount_minor
		FROM expense_splits
		WHERE transaction_id = $1 AND payer_user_id = $2
		ORDER BY participant_user_id
		FOR UPDATE
		LIMIT 21
	`, transactionID, userID)
	if err != nil {
		return false, fmt.Errorf("load provider transaction split: %w", err)
	}
	type share struct {
		userID string
		amount int64
	}
	shares := make([]share, 0, 20)
	total := int64(0)
	for rows.Next() {
		var item share
		if err := rows.Scan(&item.userID, &item.amount); err != nil {
			rows.Close()
			return false, fmt.Errorf("scan provider transaction split: %w", err)
		}
		if item.amount <= 0 || total > math.MaxInt64-item.amount {
			rows.Close()
			return false, errors.New("stored expense split amount is invalid")
		}
		total += item.amount
		shares = append(shares, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, fmt.Errorf("iterate provider transaction split: %w", err)
	}
	rows.Close()
	if len(shares) > 20 {
		return false, errors.New("stored expense split exceeds bound")
	}
	for _, item := range shares {
		if err := changeProviderHouseholdOwed(ctx, tx, householdID, item.userID, currency, -item.amount); err != nil {
			return false, err
		}
	}
	if total > 0 {
		if err := changeProviderHouseholdOwed(ctx, tx, householdID, userID, currency, total); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM expense_splits WHERE transaction_id = $1`, transactionID); err != nil {
		return false, fmt.Errorf("clear provider transaction split: %w", err)
	}
	return len(shares) > 0, nil
}

func changeProviderHouseholdOwed(
	ctx context.Context,
	tx pgx.Tx,
	householdID string,
	userID string,
	currency string,
	delta int64,
) error {
	result, err := tx.Exec(ctx, `
		UPDATE household_member_summaries
		SET owed_minor = owed_minor + $4,
		    difference_minor = paid_minor - (owed_minor + $4),
		    settlement_minor = NULL,
		    updated_at = now()
		WHERE household_id = $1 AND user_id = $2 AND currency = $3
		  AND owed_minor + $4 >= 0
	`, householdID, userID, currency, delta)
	if err != nil {
		return fmt.Errorf("update provider household owed summary: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("provider household owed summary is inconsistent")
	}
	return nil
}

func deactivateProviderHouseholdExpense(
	ctx context.Context,
	tx pgx.Tx,
	householdID string,
	userID string,
	currency string,
	amountMinor int64,
) error {
	result, err := tx.Exec(ctx, `
		UPDATE household_summaries
		SET total_spending_minor = total_spending_minor - $3, updated_at = now()
		WHERE household_id = $1 AND currency = $2 AND total_spending_minor >= $3
	`, householdID, currency, amountMinor)
	if err != nil {
		return fmt.Errorf("reverse provider household spending: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("provider household spending summary is inconsistent")
	}
	result, err = tx.Exec(ctx, `
		UPDATE household_member_summaries
		SET paid_minor = paid_minor - $4,
		    owed_minor = owed_minor - $4,
		    difference_minor = (paid_minor - $4) - (owed_minor - $4),
		    settlement_minor = NULL,
		    updated_at = now()
		WHERE household_id = $1 AND user_id = $2 AND currency = $3
		  AND paid_minor >= $4 AND owed_minor >= $4
	`, householdID, userID, currency, amountMinor)
	if err != nil {
		return fmt.Errorf("reverse provider household payer summary: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("provider household payer summary is inconsistent")
	}
	return nil
}

func activateProviderHouseholdExpense(
	ctx context.Context,
	tx pgx.Tx,
	householdID string,
	userID string,
	currency string,
	amountMinor int64,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO household_summaries (household_id, currency, total_spending_minor)
		VALUES ($1, $2, $3)
		ON CONFLICT (household_id, currency) DO UPDATE
		SET total_spending_minor = household_summaries.total_spending_minor + EXCLUDED.total_spending_minor,
		    updated_at = now()
	`, householdID, currency, amountMinor); err != nil {
		return fmt.Errorf("apply provider household spending: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO household_member_summaries (
			household_id, user_id, currency, paid_minor, owed_minor, difference_minor
		) VALUES ($1, $2, $3, $4, $4, 0)
		ON CONFLICT (household_id, user_id, currency) DO UPDATE
		SET paid_minor = household_member_summaries.paid_minor + EXCLUDED.paid_minor,
		    owed_minor = household_member_summaries.owed_minor + EXCLUDED.owed_minor,
		    difference_minor = (household_member_summaries.paid_minor + EXCLUDED.paid_minor)
		                     - (household_member_summaries.owed_minor + EXCLUDED.owed_minor),
		    settlement_minor = NULL,
		    updated_at = now()
	`, householdID, userID, currency, amountMinor); err != nil {
		return fmt.Errorf("apply provider household payer summary: %w", err)
	}
	return nil
}

func adjustBudgetSpending(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	categoryID *string,
	spaceID *string,
	transactionDate time.Time,
	currency string,
	amountMinor int64,
	add bool,
) error {
	if amountMinor <= 0 || (categoryID == nil && spaceID == nil) || (categoryID != nil && spaceID != nil) {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, period,
		       CASE period
		           WHEN 'WEEKLY' THEN date_trunc('week', CURRENT_DATE)::date
		           WHEN 'MONTHLY' THEN date_trunc('month', CURRENT_DATE)::date
		           ELSE date_trunc('year', CURRENT_DATE)::date
		       END AS current_period_start
		FROM budgets
		WHERE user_id = $1 AND currency = $2
		  AND (($3::uuid IS NOT NULL AND category_id = $3) OR
		       ($4::uuid IS NOT NULL AND space_id = $4))
		ORDER BY id
		LIMIT 101
	`, string(userID), currency, categoryID, spaceID)
	if err != nil {
		return fmt.Errorf("load matching transaction budgets: %w", err)
	}
	type budgetPeriod struct {
		id          string
		period      string
		periodStart time.Time
	}
	budgets := make([]budgetPeriod, 0, 4)
	for rows.Next() {
		var budget budgetPeriod
		if err := rows.Scan(&budget.id, &budget.period, &budget.periodStart); err != nil {
			rows.Close()
			return fmt.Errorf("scan matching transaction budget: %w", err)
		}
		budgets = append(budgets, budget)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate matching transaction budgets: %w", err)
	}
	rows.Close()
	if len(budgets) > 100 {
		return errors.New("matching transaction budget count exceeds bound")
	}
	transactionDay := time.Date(
		transactionDate.Year(), transactionDate.Month(), transactionDate.Day(), 0, 0, 0, 0, time.UTC,
	)
	for _, budget := range budgets {
		periodEnd := budget.periodStart.AddDate(1, 0, 0)
		switch budget.period {
		case "WEEKLY":
			periodEnd = budget.periodStart.AddDate(0, 0, 7)
		case "MONTHLY":
			periodEnd = budget.periodStart.AddDate(0, 1, 0)
		}
		if transactionDay.Before(budget.periodStart) || !transactionDay.Before(periodEnd) {
			continue
		}
		if add {
			if _, err := tx.Exec(ctx, `
				UPDATE budgets
				SET spent_minor = CASE WHEN period_start = $2 THEN spent_minor + $3 ELSE $3 END,
				    period_start = $2,
				    updated_at = now()
				WHERE id = $1
			`, budget.id, budget.periodStart, amountMinor); err != nil {
				return fmt.Errorf("increase transaction budget spending: %w", err)
			}
		} else {
			result, err := tx.Exec(ctx, `
				UPDATE budgets
				SET spent_minor = CASE
				        WHEN period_start = $2 THEN spent_minor - $3
				        ELSE 0
				    END,
				    period_start = $2,
				    updated_at = now()
				WHERE id = $1 AND (period_start <> $2 OR spent_minor >= $3)
			`, budget.id, budget.periodStart, amountMinor)
			if err != nil {
				return fmt.Errorf("decrease transaction budget spending: %w", err)
			}
			if result.RowsAffected() != 1 {
				return errors.New("transaction budget spending summary is inconsistent")
			}
		}
	}
	return nil
}

type Page struct {
	Transactions []transactions.Transaction
	NextCursor   string
}

type pageCursor struct {
	Date string `json:"date"`
	ID   string `json:"id"`
}

func (r *Repository) List(ctx context.Context, userID shared.UserID, cursor string, limit int, filters transactions.ListFilters) (Page, error) {
	if limit < 1 || limit > 100 {
		return Page{}, errors.New("transaction page limit must be between 1 and 100")
	}
	var cursorDate *time.Time
	var cursorID *string
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil {
			return Page{}, ErrInvalidCursor
		}
		var parsed pageCursor
		if err := json.Unmarshal(decoded, &parsed); err != nil || parsed.ID == "" {
			return Page{}, ErrInvalidCursor
		}
		date, err := time.Parse("2006-01-02", parsed.Date)
		if err != nil {
			return Page{}, ErrInvalidCursor
		}
		cursorDate, cursorID = &date, &parsed.ID
	}
	var queryPrefix *string
	if filters.Query != nil {
		value := escapeTransactionPrefix(strings.ToLower(strings.TrimSpace(*filters.Query))) + "%"
		queryPrefix = &value
	}

	rows, err := r.db.Query(ctx, `
		WITH page AS (
			SELECT t.id, t.user_id, t.account_id, t.name, t.merchant_name, t.amount_minor,
			       t.currency, t.transaction_date, t.is_pending, t.category_id,
			       t.review_status, t.visibility, t.category_source,
			       t.recurring_detection_key IS NOT NULL AS is_recurring
			FROM transactions t
			LEFT JOIN transaction_allocations filter_allocation
			  ON filter_allocation.transaction_id=t.id
			 AND filter_allocation.space_id=$7
			LEFT JOIN spaces filter_space
			  ON filter_space.id=filter_allocation.space_id
			 AND filter_space.user_id=t.user_id
			WHERE t.user_id = $1 AND t.removed_at IS NULL
			  AND ($2::date IS NULL OR (t.transaction_date, t.id) < ($2::date, $3::uuid))
			  AND ($5::uuid IS NULL OR t.account_id = $5)
			  AND ($6::uuid IS NULL OR t.category_id = $6)
			  AND ($7::uuid IS NULL OR filter_space.id IS NOT NULL)
			  AND ($8::text IS NULL OR t.review_status = $8)
			  AND (NOT $9::boolean OR t.transaction_date >= date_trunc('month', CURRENT_DATE)::date)
			  AND ($10::text IS NULL OR lower(COALESCE(t.merchant_name, t.name)) LIKE $10 ESCAPE '\')
			ORDER BY t.transaction_date DESC, t.id DESC
			LIMIT $4
		)
		SELECT p.id::text, p.account_id::text, a.name, p.name, p.merchant_name,
		       p.amount_minor, p.currency, p.transaction_date, p.is_pending,
		       p.category_id::text, c.name, allocation.space_id, allocation.space_name,
		       p.review_status, p.visibility, p.category_source, p.is_recurring
		FROM page p
		JOIN accounts a ON a.id = p.account_id AND a.user_id = p.user_id
		LEFT JOIN categories c ON c.id = p.category_id
		LEFT JOIN LATERAL (
			SELECT ta.space_id::text AS space_id, s.name AS space_name
			FROM transaction_allocations ta
			JOIN spaces s ON s.id = ta.space_id
			WHERE ta.transaction_id = p.id
			ORDER BY ta.space_id
			LIMIT 1
		) allocation ON true
		ORDER BY p.transaction_date DESC, p.id DESC
	`, string(userID), cursorDate, cursorID, limit+1, filters.AccountID, filters.CategoryID,
		filters.SpaceID, filters.ReviewStatus, filters.CurrentMonth, queryPrefix)
	if err != nil {
		return Page{}, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	items := make([]transactions.Transaction, 0, limit+1)
	for rows.Next() {
		var item transactions.Transaction
		var id, accountID, currency string
		if err := rows.Scan(&id, &accountID, &item.AccountName, &item.Name, &item.MerchantName,
			&item.AmountMinor, &currency, &item.Date, &item.IsPending, &item.CategoryID,
			&item.CategoryName, &item.SpaceID, &item.SpaceName, &item.ReviewStatus,
			&item.Visibility, &item.CategorySource, &item.IsRecurring); err != nil {
			return Page{}, fmt.Errorf("scan transaction: %w", err)
		}
		item.ID = shared.TransactionID(id)
		item.AccountID = shared.AccountID(accountID)
		item.Currency = shared.Currency(currency)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, fmt.Errorf("iterate transactions: %w", err)
	}

	page := Page{Transactions: items}
	if len(items) > limit {
		page.Transactions = items[:limit]
		last := page.Transactions[len(page.Transactions)-1]
		encoded, _ := json.Marshal(pageCursor{Date: last.Date.Format("2006-01-02"), ID: string(last.ID)})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return page, nil
}

func escapeTransactionPrefix(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}

func (r *Repository) Detail(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID) (transactions.Detail, error) {
	var detail transactions.Detail
	var id, accountID, currency string
	err := r.db.QueryRow(ctx, `
		SELECT t.id::text, t.account_id::text, a.name, t.name, t.merchant_name,
		       t.amount_minor, t.currency, t.transaction_date, t.is_pending,
		       t.category_id::text, c.name, t.review_status, t.visibility,
		       t.original_description, t.authorized_date, t.category_source,
		       t.recurring_detection_key IS NOT NULL
		FROM transactions t
		JOIN accounts a ON a.id = t.account_id AND a.user_id = t.user_id
		LEFT JOIN categories c ON c.id = t.category_id
		WHERE t.user_id = $1 AND t.id = $2 AND t.removed_at IS NULL
	`, string(userID), string(transactionID)).Scan(
		&id, &accountID, &detail.AccountName, &detail.Name, &detail.MerchantName,
		&detail.AmountMinor, &currency, &detail.Date, &detail.IsPending,
		&detail.CategoryID, &detail.CategoryName, &detail.ReviewStatus,
		&detail.Visibility, &detail.OriginalDescription, &detail.AuthorizedDate,
		&detail.CategorySource, &detail.IsRecurring,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return transactions.Detail{}, ErrNotFound
	}
	if err != nil {
		return transactions.Detail{}, fmt.Errorf("load transaction detail: %w", err)
	}
	detail.ID = shared.TransactionID(id)
	detail.AccountID = shared.AccountID(accountID)
	detail.Currency = shared.Currency(currency)

	rows, err := r.db.Query(ctx, `
		SELECT ta.space_id::text, s.name, ta.amount_minor
		FROM transaction_allocations ta
		JOIN spaces s ON s.id = ta.space_id
		WHERE ta.transaction_id = $1
		ORDER BY ta.space_id
		LIMIT 51
	`, string(transactionID))
	if err != nil {
		return transactions.Detail{}, fmt.Errorf("load transaction allocations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var allocation transactions.Allocation
		if err := rows.Scan(&allocation.SpaceID, &allocation.SpaceName, &allocation.AmountMinor); err != nil {
			return transactions.Detail{}, fmt.Errorf("scan transaction allocation: %w", err)
		}
		detail.Allocations = append(detail.Allocations, allocation)
	}
	if err := rows.Err(); err != nil {
		return transactions.Detail{}, fmt.Errorf("iterate transaction allocations: %w", err)
	}
	if len(detail.Allocations) > 50 {
		return transactions.Detail{}, errors.New("transaction allocation count exceeds the response bound")
	}
	return detail, nil
}

func (r *Repository) Update(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
	update transactions.Update,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction update: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := updateInTransaction(ctx, tx, userID, transactionID, update); err != nil {
		return err
	}
	if update.CategoryID != nil {
		if err := syncCategoryLearning(ctx, tx, userID, []shared.TransactionID{transactionID}); err != nil {
			return err
		}
		if err := enqueueTransactionAnalysis(ctx, tx, userID); err != nil {
			return err
		}
	}
	if err := enqueueMaterialization(ctx, tx, userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction update: %w", err)
	}
	return nil
}

func updateInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	transactionID shared.TransactionID,
	update transactions.Update,
) error {
	var amountMinor int64
	var currency, visibility string
	var transactionDate time.Time
	var categoryID *string
	err := tx.QueryRow(ctx, `
		SELECT amount_minor, currency, transaction_date, category_id::text, visibility
		FROM transactions
		WHERE user_id = $1 AND id = $2 AND removed_at IS NULL
		FOR UPDATE
	`, string(userID), string(transactionID)).Scan(
		&amountMinor, &currency, &transactionDate, &categoryID, &visibility,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock transaction: %w", err)
	}

	if update.CategoryID != nil && *update.CategoryID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM categories
				WHERE id = $1 AND (user_id IS NULL OR user_id = $2)
			)
		`, *update.CategoryID, string(userID)).Scan(&exists); err != nil {
			return fmt.Errorf("validate transaction category: %w", err)
		}
		if !exists {
			return ErrRelatedEntityNotFound
		}
	}
	if update.SpaceID != nil && *update.SpaceID != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM spaces WHERE id = $1 AND user_id = $2 AND currency = $3
			)
		`, *update.SpaceID, string(userID), currency).Scan(&exists); err != nil {
			return fmt.Errorf("validate transaction space: %w", err)
		}
		if !exists {
			return ErrRelatedEntityNotFound
		}
	}
	if update.Visibility != nil && *update.Visibility != visibility {
		if err := changeHouseholdVisibility(
			ctx, tx, string(userID), string(transactionID), currency, visibility, *update.Visibility, amountMinor,
		); err != nil {
			return err
		}
	}
	if update.CategoryID != nil && amountMinor < 0 && amountMinor != math.MinInt64 {
		if err := adjustBudgetSpending(
			ctx, tx, userID, categoryID, nil, transactionDate, currency, -amountMinor, false,
		); err != nil {
			return err
		}
		if *update.CategoryID != "" {
			if err := adjustBudgetSpending(
				ctx, tx, userID, update.CategoryID, nil, transactionDate, currency, -amountMinor, true,
			); err != nil {
				return err
			}
		}
	}
	if update.SpaceID != nil && amountMinor != math.MinInt64 {
		allocations, err := loadProviderAllocations(ctx, tx, string(transactionID))
		if err != nil {
			return err
		}
		for _, allocation := range allocations {
			if allocation.amountMinor < 0 {
				spaceID := allocation.spaceID
				if err := adjustBudgetSpending(
					ctx, tx, userID, nil, &spaceID, transactionDate, currency, -allocation.amountMinor, false,
				); err != nil {
					return err
				}
			}
		}
		if *update.SpaceID != "" && amountMinor < 0 {
			if err := adjustBudgetSpending(
				ctx, tx, userID, nil, update.SpaceID, transactionDate, currency, -amountMinor, true,
			); err != nil {
				return err
			}
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE transactions
		SET category_id = CASE WHEN $3::boolean THEN NULLIF($4, '')::uuid ELSE category_id END,
		    category_source = CASE
		        WHEN $3::boolean AND $4<>'' THEN 'USER'
		        WHEN $3::boolean THEN 'UNASSIGNED'
		        ELSE category_source
		    END,
		    category_confidence_basis_points = CASE WHEN $3::boolean THEN NULL ELSE category_confidence_basis_points END,
		    review_status = CASE
		        WHEN $3::boolean AND $4<>'' THEN 'REVIEWED'
		        WHEN $3::boolean THEN 'NEEDS_REVIEW'
		        WHEN $5::boolean THEN $6
		        ELSE review_status
		    END,
		    visibility = CASE WHEN $7::boolean THEN $8 ELSE visibility END,
		    updated_at = now()
		WHERE user_id = $1 AND id = $2
	`, string(userID), string(transactionID), update.CategoryID != nil, stringValue(update.CategoryID),
		update.ReviewStatus != nil, stringValue(update.ReviewStatus), update.Visibility != nil,
		stringValue(update.Visibility))
	if err != nil {
		return fmt.Errorf("update transaction: %w", err)
	}
	if update.CategoryID != nil && *update.CategoryID != "" {
		if _, err := tx.Exec(ctx, `
			WITH resolved AS (
				UPDATE inbox_items SET status='RESOLVED',resolved_at=now()
				WHERE user_id=$1 AND entity_id=$2 AND item_type='TRANSACTION_REVIEW'
				  AND status='OPEN' AND payload->>'reason'='CATEGORY_UNCERTAIN'
				RETURNING 1
			)
			UPDATE users
			SET open_inbox_count=GREATEST(open_inbox_count-(SELECT count(*) FROM resolved),0),
			    updated_at=now()
			WHERE id=$1 AND EXISTS(SELECT 1 FROM resolved)
		`, string(userID), string(transactionID)); err != nil {
			return fmt.Errorf("resolve transaction category review: %w", err)
		}
	}

	if update.SpaceID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE spaces s
			SET balance_minor = CASE
			        WHEN s.balance_period_start = date_trunc('month', CURRENT_DATE)::date
			            THEN s.balance_minor - old_allocations.amount_minor
			        ELSE s.monthly_allocation_minor - old_allocations.amount_minor
			    END,
			    balance_period_start = date_trunc('month', CURRENT_DATE)::date,
			    updated_at = now()
			FROM (
				SELECT ta.space_id, ta.amount_minor
				FROM transaction_allocations ta
				JOIN transactions current_transaction ON current_transaction.id = ta.transaction_id
				WHERE ta.transaction_id = $1
				  AND current_transaction.transaction_date >= date_trunc('month', CURRENT_DATE)::date
			) old_allocations
			WHERE s.id = old_allocations.space_id AND s.user_id = $2
		`, string(transactionID), string(userID)); err != nil {
			return fmt.Errorf("reverse transaction space balances: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM transaction_allocations WHERE transaction_id = $1`, string(transactionID)); err != nil {
			return fmt.Errorf("clear transaction allocations: %w", err)
		}
		if *update.SpaceID != "" && amountMinor != 0 {
			if _, err := tx.Exec(ctx, `
				INSERT INTO transaction_allocations (transaction_id, space_id, transaction_date, amount_minor)
				VALUES ($1, $2, $3, $4)
			`, string(transactionID), *update.SpaceID, transactionDate, amountMinor); err != nil {
				return fmt.Errorf("allocate transaction to space: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE spaces
				SET balance_minor = CASE
				        WHEN balance_period_start = date_trunc('month', CURRENT_DATE)::date
				            THEN balance_minor + $3
				        ELSE monthly_allocation_minor + $3
				    END,
				    balance_period_start = date_trunc('month', CURRENT_DATE)::date,
				    updated_at = now()
				WHERE id = $1 AND user_id = $2
				  AND EXISTS (
				      SELECT 1 FROM transactions
				      WHERE id = $4 AND transaction_date >= date_trunc('month', CURRENT_DATE)::date
				  )
			`, *update.SpaceID, string(userID), amountMinor, string(transactionID)); err != nil {
				return fmt.Errorf("update transaction space balance: %w", err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'TRANSACTION_UPDATED', $2, $3)
	`, string(userID), string(entities.Transaction), string(transactionID)); err != nil {
		return fmt.Errorf("audit transaction update: %w", err)
	}
	return nil
}

func (r *Repository) BulkUpdate(ctx context.Context, userID shared.UserID, command transactions.BulkUpdate) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bulk transaction update: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, transactionID := range command.TransactionIDs {
		if err := updateInTransaction(ctx, tx, userID, transactionID, command.Update); err != nil {
			return err
		}
	}
	if command.Update.CategoryID != nil {
		if err := syncCategoryLearning(ctx, tx, userID, command.TransactionIDs); err != nil {
			return err
		}
		if err := enqueueTransactionAnalysis(ctx, tx, userID); err != nil {
			return err
		}
	}
	if err := enqueueMaterialization(ctx, tx, userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bulk transaction update: %w", err)
	}
	return nil
}

func syncCategoryLearning(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	transactionIDs []shared.TransactionID,
) error {
	ids := make([]string, len(transactionIDs))
	for index, id := range transactionIDs {
		ids[index] = string(id)
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (categorization_learning_key)
		       categorization_learning_key,category_id::text
		FROM transactions
		WHERE user_id=$1 AND id=ANY($2::uuid[])
		  AND removed_at IS NULL AND categorization_learning_key IS NOT NULL
		ORDER BY categorization_learning_key,updated_at DESC,id DESC
		LIMIT 4097
	`, string(userID), ids)
	if err != nil {
		return fmt.Errorf("load transaction category learning updates: %w", err)
	}
	defer rows.Close()
	updates := make([]categoryrepo.LearningPreferenceUpdate, 0, len(transactionIDs))
	for rows.Next() {
		var update categoryrepo.LearningPreferenceUpdate
		if err := rows.Scan(&update.LearningKey, &update.CategoryID); err != nil {
			return fmt.Errorf("scan transaction category learning update: %w", err)
		}
		updates = append(updates, update)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate transaction category learning updates: %w", err)
	}
	if len(updates) > 4096 {
		return errors.New("transaction category learning update exceeds bound")
	}
	if err := categoryrepo.SyncLearningPreferences(ctx, tx, userID, updates); err != nil {
		return fmt.Errorf("synchronize transaction category learning: %w", err)
	}
	return nil
}

func enqueueTransactionAnalysis(ctx context.Context, tx pgx.Tx, userID shared.UserID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)
		VALUES($1,$2,$1,'TRANSACTION_ANALYSIS_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)
	`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue transaction category analysis: %w", err)
	}
	return nil
}

func enqueueMaterialization(ctx context.Context, tx pgx.Tx, userID shared.UserID) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)
		VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)
	`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue transaction materialization: %w", err)
	}
	return nil
}

func changeHouseholdVisibility(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	transactionID string,
	currency string,
	from string,
	to string,
	amountMinor int64,
) error {
	var householdID string
	err := tx.QueryRow(ctx, `
		SELECT household_id::text
		FROM household_members
		WHERE user_id = $1
		FOR SHARE
	`, userID).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		if to == "HOUSEHOLD" {
			return ErrHouseholdRequired
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("load transaction household: %w", err)
	}

	if from == "HOUSEHOLD" && to == "PRIVATE" {
		var hasSplit bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM expense_splits WHERE transaction_id = $1)
		`, transactionID).Scan(&hasSplit); err != nil {
			return fmt.Errorf("check transaction expense split: %w", err)
		}
		if hasSplit {
			return ErrExpenseSplitExists
		}
	}
	if amountMinor >= 0 {
		return nil
	}
	if amountMinor == math.MinInt64 {
		return errors.New("transaction amount cannot be represented as household spending")
	}
	spendingMinor := -amountMinor

	if from == "PRIVATE" && to == "HOUSEHOLD" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO household_summaries (household_id, currency, total_spending_minor)
			VALUES ($1, $2, $3)
			ON CONFLICT (household_id, currency) DO UPDATE
			SET total_spending_minor = household_summaries.total_spending_minor + EXCLUDED.total_spending_minor,
			    updated_at = now()
		`, householdID, currency, spendingMinor); err != nil {
			return fmt.Errorf("increase household spending summary: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO household_member_summaries (
				household_id, user_id, currency, paid_minor, owed_minor, difference_minor
			) VALUES ($1, $2, $3, $4, $4, 0)
			ON CONFLICT (household_id, user_id, currency) DO UPDATE
			SET paid_minor = household_member_summaries.paid_minor + EXCLUDED.paid_minor,
			    owed_minor = household_member_summaries.owed_minor + EXCLUDED.owed_minor,
			    difference_minor = (household_member_summaries.paid_minor + EXCLUDED.paid_minor)
			                     - (household_member_summaries.owed_minor + EXCLUDED.owed_minor),
			    settlement_minor = NULL,
			    updated_at = now()
		`, householdID, userID, currency, spendingMinor); err != nil {
			return fmt.Errorf("increase household payer summary: %w", err)
		}
	} else if from == "HOUSEHOLD" && to == "PRIVATE" {
		result, err := tx.Exec(ctx, `
			UPDATE household_summaries
			SET total_spending_minor = total_spending_minor - $3, updated_at = now()
			WHERE household_id = $1 AND currency = $2 AND total_spending_minor >= $3
		`, householdID, currency, spendingMinor)
		if err != nil {
			return fmt.Errorf("decrease household spending summary: %w", err)
		}
		if result.RowsAffected() != 1 {
			return errors.New("household spending summary is inconsistent")
		}
		result, err = tx.Exec(ctx, `
			UPDATE household_member_summaries
			SET paid_minor = paid_minor - $4,
			    owed_minor = owed_minor - $4,
			    difference_minor = (paid_minor - $4) - (owed_minor - $4),
			    settlement_minor = NULL,
			    updated_at = now()
			WHERE household_id = $1 AND user_id = $2 AND currency = $3
			  AND paid_minor >= $4 AND owed_minor >= $4
		`, householdID, userID, currency, spendingMinor)
		if err != nil {
			return fmt.Errorf("decrease household payer summary: %w", err)
		}
		if result.RowsAffected() != 1 {
			return errors.New("household payer summary is inconsistent")
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE household_member_summaries
		SET settlement_minor = NULL, updated_at = now()
		WHERE household_id = $1 AND currency = $2
	`, householdID, currency); err != nil {
		return fmt.Errorf("invalidate household settlement: %w", err)
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
