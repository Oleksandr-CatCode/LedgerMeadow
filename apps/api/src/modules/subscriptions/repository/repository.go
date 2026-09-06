package repository

import (
	"ledgermeadow/src/entities"
	recurringrepo "ledgermeadow/src/modules/recurring/repository"
	"ledgermeadow/src/modules/subscriptions/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

const limit = 100

var ErrTooMany = errors.New("subscription response exceeds bound")
var ErrRelatedNotFound = errors.New("subscription related entity not found")
var ErrNotFound = errors.New("subscription not found")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Reclassify(ctx context.Context, userID shared.UserID, id string) error {
	err := recurringrepo.Reclassify(ctx, r.db, userID, id, entities.Subscription)
	if errors.Is(err, recurringrepo.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func (r *Repository) List(ctx context.Context, u shared.UserID) ([]models.Subscription, error) {
	rows, e := r.db.Query(ctx, `SELECT sub.id::text,sub.merchant_name,sub.expected_amount_minor,sub.currency,sub.frequency,sub.next_expected_at::text,sub.category_id::text,c.name,sub.space_id::text,s.name,sub.payment_account_id::text,a.name,sub.status,sub.source,sub.occurrence_count FROM subscriptions sub LEFT JOIN categories c ON c.id=sub.category_id LEFT JOIN spaces s ON s.id=sub.space_id AND s.user_id=sub.user_id LEFT JOIN accounts a ON a.id=sub.payment_account_id AND a.user_id=sub.user_id WHERE sub.user_id=$1 ORDER BY sub.next_expected_at,sub.id LIMIT $2`, string(u), limit+1)
	if e != nil {
		return nil, fmt.Errorf("list subscriptions: %w", e)
	}
	defer rows.Close()
	v := make([]models.Subscription, 0, limit)
	for rows.Next() {
		var i models.Subscription
		if e := rows.Scan(&i.ID, &i.MerchantName, &i.ExpectedAmountMinor, &i.Currency, &i.Frequency, &i.NextExpectedAt, &i.CategoryID, &i.CategoryName, &i.SpaceID, &i.SpaceName, &i.PaymentAccountID, &i.PaymentAccountName, &i.Status, &i.Source, &i.OccurrenceCount); e != nil {
			return nil, fmt.Errorf("scan subscription: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", e)
	}
	if len(v) > limit {
		return nil, ErrTooMany
	}
	return v, nil
}

func (r *Repository) Detail(ctx context.Context, userID shared.UserID, id string) (models.DetailSource, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return models.DetailSource{}, fmt.Errorf("begin subscription detail: %w", err)
	}
	defer tx.Rollback(ctx)

	var item models.DetailSource
	var detectionKey *string
	var timezone string
	err = tx.QueryRow(ctx, `SELECT sub.id::text,sub.merchant_name,sub.expected_amount_minor,sub.currency,sub.frequency,sub.next_expected_at::text,c.id::text,c.name,s.id::text,s.name,a.id::text,a.name,sub.status,sub.source,sub.occurrence_count,sub.detection_key,u.timezone FROM subscriptions sub JOIN users u ON u.id=sub.user_id LEFT JOIN categories c ON c.id=sub.category_id AND (c.user_id IS NULL OR c.user_id=sub.user_id) LEFT JOIN spaces s ON s.id=sub.space_id AND s.user_id=sub.user_id LEFT JOIN accounts a ON a.id=sub.payment_account_id AND a.user_id=sub.user_id WHERE sub.user_id=$1 AND sub.id=$2`, string(userID), id).Scan(&item.ID, &item.MerchantName, &item.ExpectedAmountMinor, &item.Currency, &item.Frequency, &item.NextExpectedAt, &item.CategoryID, &item.CategoryName, &item.SpaceID, &item.SpaceName, &item.PaymentAccountID, &item.PaymentAccountName, &item.Status, &item.Source, &item.OccurrenceCount, &detectionKey, &timezone)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.DetailSource{}, ErrNotFound
	}
	if err != nil {
		return models.DetailSource{}, fmt.Errorf("load subscription: %w", err)
	}
	item.Payments = make([]models.PaymentSource, 0, models.PaymentHistoryLimit)
	if detectionKey != nil {
		location, err := time.LoadLocation(timezone)
		if err != nil {
			return models.DetailSource{}, errors.New("stored user timezone is invalid")
		}
		now := time.Now().In(location)
		yearStart := time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, location)
		nextYear := yearStart.AddDate(1, 0, 0)
		rows, err := tx.Query(ctx, `SELECT t.transaction_date::text,t.amount_minor FROM transactions t WHERE t.user_id=$1 AND t.recurring_detection_key=$2 AND t.currency=$3 AND t.removed_at IS NULL AND NOT t.is_pending AND t.amount_minor<0 AND t.transaction_date>=$4 AND t.transaction_date<$5 ORDER BY t.transaction_date DESC,t.id DESC LIMIT $6`, string(userID), *detectionKey, item.Currency, yearStart.Format("2006-01-02"), nextYear.Format("2006-01-02"), models.DetailPaymentLimit+1)
		if err != nil {
			return models.DetailSource{}, fmt.Errorf("load subscription payments: %w", err)
		}
		defer rows.Close()
		item.Payments = make([]models.PaymentSource, 0, models.DetailPaymentLimit)
		for rows.Next() {
			var payment models.PaymentSource
			if err := rows.Scan(&payment.PaidAt, &payment.AmountMinor); err != nil {
				return models.DetailSource{}, fmt.Errorf("scan subscription payment: %w", err)
			}
			item.Payments = append(item.Payments, payment)
		}
		if err := rows.Err(); err != nil {
			return models.DetailSource{}, fmt.Errorf("iterate subscription payments: %w", err)
		}
		if len(item.Payments) > models.DetailPaymentLimit {
			return models.DetailSource{}, ErrTooMany
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return models.DetailSource{}, fmt.Errorf("commit subscription detail: %w", err)
	}
	return item, nil
}

func (r *Repository) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription update: %w", err)
	}
	defer tx.Rollback(ctx)
	checks := []struct {
		id            *string
		query         string
		matchCurrency bool
	}{
		{command.CategoryID, `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND (user_id IS NULL OR user_id=$2))`, false},
		{command.SpaceID, `SELECT EXISTS(SELECT 1 FROM spaces WHERE id=$1 AND user_id=$2 AND currency=$3)`, true},
		{command.PaymentAccountID, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN bank_connections bc ON bc.id=a.bank_connection_id WHERE a.id=$1 AND a.user_id=$2 AND a.currency=$3 AND bc.status<>'DISCONNECTED')`, true},
	}
	for _, check := range checks {
		if check.id == nil {
			continue
		}
		var exists bool
		args := []any{*check.id, string(userID)}
		if check.matchCurrency {
			args = append(args, command.Currency)
		}
		if err := tx.QueryRow(ctx, check.query, args...).Scan(&exists); err != nil {
			return fmt.Errorf("validate subscription update relation: %w", err)
		}
		if !exists {
			return ErrRelatedNotFound
		}
	}
	result, err := tx.Exec(ctx, `UPDATE subscriptions SET merchant_name=$3,expected_amount_minor=$4,currency=$5,frequency=$6,next_expected_at=$7,category_id=$8,space_id=$9,payment_account_id=$10,status=$11,user_modified_at=now(),updated_at=now() WHERE user_id=$1 AND id=$2`, string(userID), id, strings.TrimSpace(command.MerchantName), command.ExpectedAmountMinor, command.Currency, command.Frequency, command.NextExpectedAt, command.CategoryID, command.SpaceID, command.PaymentAccountID, command.Status)
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id)VALUES($1,'SUBSCRIPTION_UPDATED',$2,$3)`, string(userID), string(entities.Subscription), id); err != nil {
		return fmt.Errorf("audit subscription update: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue subscription materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit subscription update: %w", err)
	}
	return nil
}
func (r *Repository) Create(ctx context.Context, u shared.UserID, c models.Create) (string, error) {
	tx, e := r.db.Begin(ctx)
	if e != nil {
		return "", fmt.Errorf("begin subscription create: %w", e)
	}
	defer tx.Rollback(ctx)
	checks := []struct {
		id            *string
		query         string
		matchCurrency bool
	}{
		{c.CategoryID, `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND (user_id IS NULL OR user_id=$2))`, false},
		{c.SpaceID, `SELECT EXISTS(SELECT 1 FROM spaces WHERE id=$1 AND user_id=$2 AND currency=$3)`, true},
		{c.PaymentAccountID, `SELECT EXISTS(SELECT 1 FROM accounts a JOIN bank_connections bc ON bc.id=a.bank_connection_id WHERE a.id=$1 AND a.user_id=$2 AND a.currency=$3 AND bc.status<>'DISCONNECTED')`, true},
	}
	for _, check := range checks {
		if check.id == nil {
			continue
		}
		var ok bool
		args := []any{*check.id, string(u)}
		if check.matchCurrency {
			args = append(args, c.Currency)
		}
		if e := tx.QueryRow(ctx, check.query, args...).Scan(&ok); e != nil {
			return "", fmt.Errorf("validate subscription relation: %w", e)
		}
		if !ok {
			return "", ErrRelatedNotFound
		}
	}
	var id string
	if e := tx.QueryRow(ctx, `INSERT INTO subscriptions(user_id,merchant_name,expected_amount_minor,currency,frequency,next_expected_at,category_id,space_id,payment_account_id,status,kind_confirmed_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'ACTIVE',now())RETURNING id::text`, string(u), strings.TrimSpace(c.MerchantName), c.ExpectedAmountMinor, c.Currency, c.Frequency, c.NextExpectedAt, c.CategoryID, c.SpaceID, c.PaymentAccountID).Scan(&id); e != nil {
		return "", fmt.Errorf("create subscription: %w", e)
	}
	if _, e := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id)VALUES($1,'SUBSCRIPTION_CREATED',$2,$3)`, string(u), string(entities.Subscription), id); e != nil {
		return "", fmt.Errorf("audit subscription create: %w", e)
	}
	if _, e := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(u), string(entities.User)); e != nil {
		return "", fmt.Errorf("enqueue subscription materialization: %w", e)
	}
	if e := tx.Commit(ctx); e != nil {
		return "", fmt.Errorf("commit subscription create: %w", e)
	}
	return id, nil
}
