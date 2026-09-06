package repository

import (
	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/bills/models"
	recurringrepo "ledgermeadow/src/modules/recurring/repository"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

const limit = 100

var ErrTooMany = errors.New("bill response exceeds bound")
var ErrRelatedNotFound = errors.New("bill related entity not found")
var ErrNotFound = errors.New("bill not found")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Reclassify(ctx context.Context, userID shared.UserID, id string) error {
	err := recurringrepo.Reclassify(ctx, r.db, userID, id, entities.Bill)
	if errors.Is(err, recurringrepo.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.Bill, error) {
	rows, err := r.db.Query(ctx, `SELECT bill.id::text,bill.name,bill.amount_type,bill.expected_amount_minor,bill.currency,bill.frequency,bill.next_due_at::text,bill.category_id::text,category.name,bill.space_id::text,space.name,bill.source,bill.status,bill.occurrence_count FROM bills bill LEFT JOIN categories category ON category.id=bill.category_id LEFT JOIN spaces space ON space.id=bill.space_id AND space.user_id=bill.user_id WHERE bill.user_id=$1 ORDER BY bill.next_due_at,bill.id LIMIT $2`, string(userID), limit+1)
	if err != nil {
		return nil, fmt.Errorf("list bills: %w", err)
	}
	defer rows.Close()
	items := make([]models.Bill, 0, limit)
	for rows.Next() {
		var i models.Bill
		if err := rows.Scan(&i.ID, &i.Name, &i.AmountType, &i.ExpectedAmountMinor, &i.Currency, &i.Frequency, &i.NextDueAt, &i.CategoryID, &i.CategoryName, &i.SpaceID, &i.SpaceName, &i.Source, &i.Status, &i.OccurrenceCount); err != nil {
			return nil, fmt.Errorf("scan bill: %w", err)
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bills: %w", err)
	}
	if len(items) > limit {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Detail(ctx context.Context, userID shared.UserID, id string) (models.Bill, error) {
	var item models.Bill
	err := r.db.QueryRow(ctx, `SELECT bill.id::text,bill.name,bill.amount_type,bill.expected_amount_minor,bill.currency,bill.frequency,bill.next_due_at::text,bill.category_id::text,category.name,bill.space_id::text,space.name,bill.source,bill.status,bill.occurrence_count FROM bills bill LEFT JOIN categories category ON category.id=bill.category_id LEFT JOIN spaces space ON space.id=bill.space_id AND space.user_id=bill.user_id WHERE bill.user_id=$1 AND bill.id=$2`, string(userID), id).Scan(&item.ID, &item.Name, &item.AmountType, &item.ExpectedAmountMinor, &item.Currency, &item.Frequency, &item.NextDueAt, &item.CategoryID, &item.CategoryName, &item.SpaceID, &item.SpaceName, &item.Source, &item.Status, &item.OccurrenceCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Bill{}, ErrNotFound
	}
	if err != nil {
		return models.Bill{}, fmt.Errorf("load bill: %w", err)
	}
	return item, nil
}

func (r *Repository) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bill update: %w", err)
	}
	defer tx.Rollback(ctx)
	checks := []struct {
		id       *string
		query    string
		currency bool
	}{
		{command.CategoryID, `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND (user_id IS NULL OR user_id=$2))`, false},
		{command.SpaceID, `SELECT EXISTS(SELECT 1 FROM spaces WHERE id=$1 AND user_id=$2 AND currency=$3)`, true},
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
			return fmt.Errorf("validate bill update relation: %w", err)
		}
		if !exists {
			return ErrRelatedNotFound
		}
	}
	result, err := tx.Exec(ctx, `UPDATE bills SET name=$3,amount_type=$4,expected_amount_minor=$5,currency=$6,frequency=$7,next_due_at=$8,category_id=NULLIF($9,'')::uuid,space_id=NULLIF($10,'')::uuid,status=$11,user_modified_at=now(),updated_at=now() WHERE user_id=$1 AND id=$2`, string(userID), id, strings.TrimSpace(command.Name), command.AmountType, command.ExpectedAmountMinor, command.Currency, command.Frequency, command.NextDueAt, stringValue(command.CategoryID), stringValue(command.SpaceID), command.Status)
	if err != nil {
		return fmt.Errorf("update bill: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id)VALUES($1,'BILL_UPDATED',$2,$3)`, string(userID), string(entities.Bill), id); err != nil {
		return fmt.Errorf("audit bill update: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue bill materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bill update: %w", err)
	}
	return nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func (r *Repository) Create(ctx context.Context, userID shared.UserID, c models.Create) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin bill create: %w", err)
	}
	defer tx.Rollback(ctx)
	if c.CategoryID != nil {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND (user_id IS NULL OR user_id=$2))`, *c.CategoryID, string(userID)).Scan(&ok); err != nil {
			return "", fmt.Errorf("validate bill category: %w", err)
		}
		if !ok {
			return "", ErrRelatedNotFound
		}
	}
	if c.SpaceID != nil {
		var ok bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spaces WHERE id=$1 AND user_id=$2 AND currency=$3)`, *c.SpaceID, string(userID), c.Currency).Scan(&ok); err != nil {
			return "", fmt.Errorf("validate bill space: %w", err)
		}
		if !ok {
			return "", ErrRelatedNotFound
		}
	}
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO bills(user_id,name,amount_type,expected_amount_minor,currency,frequency,next_due_at,category_id,space_id,source,kind_confirmed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'MANUAL',now()) RETURNING id::text`, string(userID), strings.TrimSpace(c.Name), c.AmountType, c.ExpectedAmountMinor, c.Currency, c.Frequency, c.NextDueAt, c.CategoryID, c.SpaceID).Scan(&id); err != nil {
		return "", fmt.Errorf("create bill: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'BILL_CREATED',$2,$3)`, string(userID), string(entities.Bill), id); err != nil {
		return "", fmt.Errorf("audit bill create: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(userID), string(entities.User)); err != nil {
		return "", fmt.Errorf("enqueue bill materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit bill create: %w", err)
	}
	return id, nil
}
