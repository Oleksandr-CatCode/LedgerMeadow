package repository

import (
	"ledgermeadow/src/modules/planning/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const collectionLimit = 100

var ErrTooMany = errors.New("planning response exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) Get(ctx context.Context, userID shared.UserID) (models.Workspace, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return models.Workspace{}, fmt.Errorf("begin planning snapshot: %w", err)
	}
	defer tx.Rollback(ctx)
	result := models.Workspace{}
	if err := tx.QueryRow(ctx, `
		SELECT local_date::text,
		       (date_trunc('month', local_date) + INTERVAL '1 month')::date::text,
		       (date_trunc('month', local_date) + INTERVAL '2 months - 1 day')::date::text
		FROM (
			SELECT (CURRENT_TIMESTAMP AT TIME ZONE timezone)::date AS local_date
			FROM users
			WHERE id = $1
		) current_user_date
	`, string(userID)).Scan(&result.AsOfDate, &result.PlanStart, &result.PlanEnd); err != nil {
		return models.Workspace{}, fmt.Errorf("load planning period: %w", err)
	}
	if result.Bills, err = listBills(ctx, tx, userID); err != nil {
		return models.Workspace{}, err
	}
	if result.Subscriptions, err = listSubscriptions(ctx, tx, userID); err != nil {
		return models.Workspace{}, err
	}
	if result.RecurringIncome, err = listRecurringIncome(ctx, tx, userID); err != nil {
		return models.Workspace{}, err
	}
	if result.Budgets, err = listBudgets(ctx, tx, userID); err != nil {
		return models.Workspace{}, err
	}
	if result.Goals, err = listGoals(ctx, tx, userID); err != nil {
		return models.Workspace{}, err
	}
	result.Summaries = make([]models.Summary, 0, 2)
	if err := tx.Commit(ctx); err != nil {
		return models.Workspace{}, fmt.Errorf("commit planning snapshot: %w", err)
	}
	return result, nil
}
func listBills(ctx context.Context, tx pgx.Tx, userID shared.UserID) ([]models.Bill, error) {
	rows, err := tx.Query(ctx, `SELECT bill.id::text,bill.name,bill.amount_type,bill.expected_amount_minor,bill.currency,bill.frequency,bill.next_due_at::text,bill.category_id::text,category.name,bill.space_id::text,space.name,bill.source,bill.status,bill.occurrence_count FROM bills bill LEFT JOIN categories category ON category.id=bill.category_id LEFT JOIN spaces space ON space.id=bill.space_id AND space.user_id=bill.user_id WHERE bill.user_id=$1 ORDER BY bill.next_due_at,bill.id LIMIT $2`, string(userID), collectionLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list planning bills: %w", err)
	}
	defer rows.Close()
	items := make([]models.Bill, 0, collectionLimit)
	for rows.Next() {
		var i models.Bill
		if err := rows.Scan(&i.ID, &i.Name, &i.AmountType, &i.ExpectedAmountMinor, &i.Currency, &i.Frequency, &i.NextDueAt, &i.CategoryID, &i.CategoryName, &i.SpaceID, &i.SpaceName, &i.Source, &i.Status, &i.OccurrenceCount); err != nil {
			return nil, fmt.Errorf("scan planning bill: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning bills: %w", err)
	}
	return bounded(items)
}
func listSubscriptions(ctx context.Context, tx pgx.Tx, userID shared.UserID) ([]models.Subscription, error) {
	rows, err := tx.Query(ctx, `SELECT sub.id::text,sub.merchant_name,sub.expected_amount_minor,sub.currency,sub.frequency,sub.next_expected_at::text,sub.category_id::text,c.name,sub.space_id::text,s.name,sub.payment_account_id::text,a.name,sub.status,sub.source,sub.occurrence_count FROM subscriptions sub LEFT JOIN categories c ON c.id=sub.category_id LEFT JOIN spaces s ON s.id=sub.space_id AND s.user_id=sub.user_id LEFT JOIN accounts a ON a.id=sub.payment_account_id AND a.user_id=sub.user_id WHERE sub.user_id=$1 ORDER BY sub.next_expected_at,sub.id LIMIT $2`, string(userID), collectionLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list planning subscriptions: %w", err)
	}
	defer rows.Close()
	items := make([]models.Subscription, 0, collectionLimit)
	for rows.Next() {
		var i models.Subscription
		if err := rows.Scan(&i.ID, &i.MerchantName, &i.ExpectedAmountMinor, &i.Currency, &i.Frequency, &i.NextExpectedAt, &i.CategoryID, &i.CategoryName, &i.SpaceID, &i.SpaceName, &i.PaymentAccountID, &i.PaymentAccountName, &i.Status, &i.Source, &i.OccurrenceCount); err != nil {
			return nil, fmt.Errorf("scan planning subscription: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning subscriptions: %w", err)
	}
	return bounded(items)
}
func listRecurringIncome(ctx context.Context, tx pgx.Tx, userID shared.UserID) ([]models.RecurringIncome, error) {
	rows, err := tx.Query(ctx, `SELECT income.id::text,income.name,income.expected_amount_minor,income.currency,income.frequency,income.next_expected_at::text,income.category_id::text,category.name,income.payment_account_id::text,account.name,income.status,income.source,income.occurrence_count FROM recurring_income_sources income LEFT JOIN categories category ON category.id=income.category_id LEFT JOIN accounts account ON account.id=income.payment_account_id AND account.user_id=income.user_id WHERE income.user_id=$1 ORDER BY income.next_expected_at,income.id LIMIT $2`, string(userID), collectionLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list planning recurring income: %w", err)
	}
	defer rows.Close()
	items := make([]models.RecurringIncome, 0, collectionLimit)
	for rows.Next() {
		var item models.RecurringIncome
		if err := rows.Scan(&item.ID, &item.Name, &item.ExpectedAmountMinor, &item.Currency, &item.Frequency, &item.NextExpectedAt, &item.CategoryID, &item.CategoryName, &item.PaymentAccountID, &item.PaymentAccountName, &item.Status, &item.Source, &item.OccurrenceCount); err != nil {
			return nil, fmt.Errorf("scan planning recurring income: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning recurring income: %w", err)
	}
	return bounded(items)
}
func listBudgets(ctx context.Context, tx pgx.Tx, userID shared.UserID) ([]models.Budget, error) {
	rows, err := tx.Query(ctx, `SELECT id::text,name,category_id::text,space_id::text,period,limit_minor,CASE WHEN period_start=CASE period WHEN 'WEEKLY' THEN date_trunc('week',CURRENT_DATE)::date WHEN 'MONTHLY' THEN date_trunc('month',CURRENT_DATE)::date ELSE date_trunc('year',CURRENT_DATE)::date END THEN spent_minor ELSE 0 END,currency,warning_threshold,critical_threshold,carryover_enabled FROM budgets WHERE user_id=$1 ORDER BY created_at,id LIMIT $2`, string(userID), collectionLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list planning budgets: %w", err)
	}
	defer rows.Close()
	items := make([]models.Budget, 0, collectionLimit)
	for rows.Next() {
		var i models.Budget
		if err := rows.Scan(&i.ID, &i.Name, &i.CategoryID, &i.SpaceID, &i.Period, &i.LimitMinor, &i.SpentMinor, &i.Currency, &i.WarningThreshold, &i.CriticalThreshold, &i.CarryoverEnabled); err != nil {
			return nil, fmt.Errorf("scan planning budget: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning budgets: %w", err)
	}
	return bounded(items)
}
func listGoals(ctx context.Context, tx pgx.Tx, userID shared.UserID) ([]models.Goal, error) {
	rows, err := tx.Query(ctx, `SELECT g.id::text,g.name,g.target_minor,g.current_minor,g.currency,g.target_date::text,g.space_id::text,s.name,g.status FROM goals g LEFT JOIN spaces s ON s.id=g.space_id AND s.user_id=g.user_id WHERE g.user_id=$1 ORDER BY g.status,g.target_date NULLS LAST,g.id LIMIT $2`, string(userID), collectionLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list planning goals: %w", err)
	}
	defer rows.Close()
	items := make([]models.Goal, 0, collectionLimit)
	for rows.Next() {
		var i models.Goal
		if err := rows.Scan(&i.ID, &i.Name, &i.TargetMinor, &i.CurrentMinor, &i.Currency, &i.TargetDate, &i.SpaceID, &i.SpaceName, &i.Status); err != nil {
			return nil, fmt.Errorf("scan planning goal: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate planning goals: %w", err)
	}
	return bounded(items)
}
func bounded[T any](items []T) ([]T, error) {
	if len(items) > collectionLimit {
		return nil, ErrTooMany
	}
	return items, nil
}
