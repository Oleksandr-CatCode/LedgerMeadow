package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/recurringincome/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listLimit = 100

var ErrNotFound = errors.New("recurring income not found")
var ErrRelatedNotFound = errors.New("recurring income related entity not found")
var ErrTooMany = errors.New("recurring income response exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.RecurringIncome, error) {
	rows, err := r.db.Query(ctx, `
		SELECT income.id::text, income.name, income.expected_amount_minor,
		       income.currency, income.frequency, income.next_expected_at::text,
		       income.category_id::text, category.name,
		       income.payment_account_id::text, account.name, income.status,
		       income.source, income.occurrence_count
		FROM recurring_income_sources income
		LEFT JOIN categories category ON category.id = income.category_id
		LEFT JOIN accounts account ON account.id = income.payment_account_id
		 AND account.user_id = income.user_id
		WHERE income.user_id = $1
		ORDER BY income.next_expected_at, income.id
		LIMIT $2
	`, string(userID), listLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list recurring income: %w", err)
	}
	defer rows.Close()
	items := make([]models.RecurringIncome, 0, listLimit)
	for rows.Next() {
		var item models.RecurringIncome
		if err := rows.Scan(
			&item.ID, &item.Name, &item.ExpectedAmountMinor, &item.Currency,
			&item.Frequency, &item.NextExpectedAt, &item.CategoryID, &item.CategoryName,
			&item.PaymentAccountID, &item.PaymentAccountName, &item.Status,
			&item.Source, &item.OccurrenceCount,
		); err != nil {
			return nil, fmt.Errorf("scan recurring income: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recurring income: %w", err)
	}
	if len(items) > listLimit {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Detail(ctx context.Context, userID shared.UserID, id string) (models.RecurringIncome, error) {
	items, err := r.listByID(ctx, userID, id)
	if err != nil {
		return models.RecurringIncome{}, err
	}
	if len(items) == 0 {
		return models.RecurringIncome{}, ErrNotFound
	}
	return items[0], nil
}

func (r *Repository) listByID(ctx context.Context, userID shared.UserID, id string) ([]models.RecurringIncome, error) {
	rows, err := r.db.Query(ctx, `
		SELECT income.id::text, income.name, income.expected_amount_minor,
		       income.currency, income.frequency, income.next_expected_at::text,
		       income.category_id::text, category.name,
		       income.payment_account_id::text, account.name, income.status,
		       income.source, income.occurrence_count
		FROM recurring_income_sources income
		LEFT JOIN categories category ON category.id = income.category_id
		LEFT JOIN accounts account ON account.id = income.payment_account_id
		 AND account.user_id = income.user_id
		WHERE income.user_id = $1 AND income.id = $2
	`, string(userID), id)
	if err != nil {
		return nil, fmt.Errorf("load recurring income: %w", err)
	}
	defer rows.Close()
	items := make([]models.RecurringIncome, 0, 1)
	for rows.Next() {
		var item models.RecurringIncome
		if err := rows.Scan(
			&item.ID, &item.Name, &item.ExpectedAmountMinor, &item.Currency,
			&item.Frequency, &item.NextExpectedAt, &item.CategoryID, &item.CategoryName,
			&item.PaymentAccountID, &item.PaymentAccountName, &item.Status,
			&item.Source, &item.OccurrenceCount,
		); err != nil {
			return nil, fmt.Errorf("scan recurring income: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin recurring income update: %w", err)
	}
	defer tx.Rollback(ctx)
	checks := []struct {
		id       *string
		query    string
		currency bool
	}{
		{command.CategoryID, `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND (user_id IS NULL OR user_id=$2) AND category_type='INCOME')`, false},
		{command.PaymentAccountID, `SELECT EXISTS(SELECT 1 FROM accounts account JOIN bank_connections connection ON connection.id=account.bank_connection_id WHERE account.id=$1 AND account.user_id=$2 AND account.currency=$3 AND connection.status<>'DISCONNECTED')`, true},
	}
	for _, check := range checks {
		if check.id == nil || *check.id == "" {
			continue
		}
		args := []any{*check.id, string(userID)}
		if check.currency {
			args = append(args, command.Currency)
		}
		var exists bool
		if err := tx.QueryRow(ctx, check.query, args...).Scan(&exists); err != nil {
			return fmt.Errorf("validate recurring income relation: %w", err)
		}
		if !exists {
			return ErrRelatedNotFound
		}
	}
	result, err := tx.Exec(ctx, `
		UPDATE recurring_income_sources
		SET name=$3, expected_amount_minor=$4, currency=$5, frequency=$6,
		    next_expected_at=$7, category_id=NULLIF($8,'')::uuid,
		    payment_account_id=NULLIF($9,'')::uuid, status=$10,
		    user_modified_at=now(), updated_at=now()
		WHERE user_id=$1 AND id=$2
	`, string(userID), id, strings.TrimSpace(command.Name), command.ExpectedAmountMinor,
		command.Currency, command.Frequency, command.NextExpectedAt,
		stringValue(command.CategoryID), stringValue(command.PaymentAccountID), command.Status)
	if err != nil {
		return fmt.Errorf("update recurring income: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id)
		VALUES($1,'RECURRING_INCOME_UPDATED',$2,$3)
	`, string(userID), string(entities.RecurringIncome), id); err != nil {
		return fmt.Errorf("audit recurring income update: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)
		VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)
	`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue recurring income materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recurring income update: %w", err)
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
