package repository

import (
	"context"
	"errors"
	"fmt"

	"ledgermeadow/src/entities"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("recurring entity not found")

type recurringEntity struct {
	name                      string
	expectedAmountMinor       int64
	currency                  string
	frequency                 string
	nextDate                  string
	categoryID                *string
	spaceID                   *string
	status                    string
	source                    string
	detectionKey              *string
	confidenceBasisPoints     *int32
	occurrenceCount           *int32
	detectedFromTransactionID *string
}

func Reclassify(
	ctx context.Context,
	db *pgxpool.Pool,
	userID shared.UserID,
	id string,
	from entities.Kind,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin recurring reclassification: %w", err)
	}
	defer tx.Rollback(ctx)

	var targetID string
	var target entities.Kind
	switch from {
	case entities.Subscription:
		target = entities.Bill
		targetID, err = subscriptionToBill(ctx, tx, userID, id)
	case entities.Bill:
		target = entities.Subscription
		targetID, err = billToSubscription(ctx, tx, userID, id)
	default:
		return errors.New("recurring reclassification kind is unsupported")
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE inbox_items
		SET entity_type=$3,entity_id=$4
		WHERE user_id=$1 AND entity_type=$2 AND entity_id=$5
	`, string(userID), string(from), string(target), targetID, id); err != nil {
		return fmt.Errorf("repoint recurring inbox items: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE notifications
		SET entity_type=$3,entity_id=$4
		WHERE user_id=$1 AND entity_type=$2 AND entity_id=$5
	`, string(userID), string(from), string(target), targetID, id); err != nil {
		return fmt.Errorf("repoint recurring notifications: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id,metadata)
		VALUES($1,'RECURRING_RECLASSIFIED',$2,$3,
		       jsonb_build_object('from_kind',$4::text,'from_id',$5::text))
	`, string(userID), string(target), targetID, string(from), id); err != nil {
		return fmt.Errorf("audit recurring reclassification: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events(
			user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload
		) VALUES(
			$1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb
		)
	`, string(userID), string(entities.User)); err != nil {
		return fmt.Errorf("enqueue recurring reclassification materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit recurring reclassification: %w", err)
	}
	return nil
}

func subscriptionToBill(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	id string,
) (string, error) {
	var entity recurringEntity
	err := tx.QueryRow(ctx, `
		SELECT subscription.merchant_name,subscription.expected_amount_minor,
		       subscription.currency,subscription.frequency,subscription.next_expected_at::text,
		       subscription.category_id::text,subscription.space_id::text,
		       subscription.status,subscription.source,subscription.detection_key,
		       subscription.confidence_basis_points,subscription.occurrence_count,
		       subscription.detected_from_transaction_id::text
		FROM subscriptions subscription
		LEFT JOIN categories category ON category.id=subscription.category_id
		LEFT JOIN spaces space ON space.id=subscription.space_id
		WHERE subscription.user_id=$1 AND subscription.id=$2
		  AND (subscription.category_id IS NULL
		       OR (category.id IS NOT NULL AND (category.user_id IS NULL OR category.user_id=$1)))
		  AND (subscription.space_id IS NULL
		       OR (space.id IS NOT NULL AND space.user_id=$1 AND space.currency=subscription.currency))
		FOR UPDATE OF subscription
	`, string(userID), id).Scan(
		&entity.name, &entity.expectedAmountMinor, &entity.currency, &entity.frequency,
		&entity.nextDate, &entity.categoryID, &entity.spaceID, &entity.status,
		&entity.source, &entity.detectionKey, &entity.confidenceBasisPoints,
		&entity.occurrenceCount, &entity.detectedFromTransactionID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load subscription for reclassification: %w", err)
	}
	var targetID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO bills(
			user_id,name,amount_type,expected_amount_minor,currency,frequency,next_due_at,
			category_id,space_id,source,status,detection_key,confidence_basis_points,
			occurrence_count,detected_from_transaction_id,user_modified_at,kind_confirmed_at
		) VALUES(
			$1,$2,'FIXED',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),now()
		) RETURNING id::text
	`, string(userID), entity.name, entity.expectedAmountMinor, entity.currency,
		entity.frequency, entity.nextDate, entity.categoryID, entity.spaceID,
		entity.source, entity.status, entity.detectionKey, entity.confidenceBasisPoints,
		entity.occurrenceCount, entity.detectedFromTransactionID,
	).Scan(&targetID); err != nil {
		return "", fmt.Errorf("create reclassified bill: %w", err)
	}
	result, err := tx.Exec(ctx, `DELETE FROM subscriptions WHERE user_id=$1 AND id=$2`, string(userID), id)
	if err != nil {
		return "", fmt.Errorf("delete reclassified subscription: %w", err)
	}
	if result.RowsAffected() != 1 {
		return "", ErrNotFound
	}
	return targetID, nil
}

func billToSubscription(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	id string,
) (string, error) {
	var entity recurringEntity
	err := tx.QueryRow(ctx, `
		SELECT bill.name,bill.expected_amount_minor,bill.currency,bill.frequency,
		       bill.next_due_at::text,bill.category_id::text,bill.space_id::text,
		       bill.status,bill.source,bill.detection_key,bill.confidence_basis_points,
		       bill.occurrence_count,bill.detected_from_transaction_id::text
		FROM bills bill
		LEFT JOIN categories category ON category.id=bill.category_id
		LEFT JOIN spaces space ON space.id=bill.space_id
		WHERE bill.user_id=$1 AND bill.id=$2
		  AND (bill.category_id IS NULL
		       OR (category.id IS NOT NULL AND (category.user_id IS NULL OR category.user_id=$1)))
		  AND (bill.space_id IS NULL
		       OR (space.id IS NOT NULL AND space.user_id=$1 AND space.currency=bill.currency))
		FOR UPDATE OF bill
	`, string(userID), id).Scan(
		&entity.name, &entity.expectedAmountMinor, &entity.currency, &entity.frequency,
		&entity.nextDate, &entity.categoryID, &entity.spaceID, &entity.status,
		&entity.source, &entity.detectionKey, &entity.confidenceBasisPoints,
		&entity.occurrenceCount, &entity.detectedFromTransactionID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load bill for reclassification: %w", err)
	}
	var targetID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO subscriptions(
			user_id,merchant_name,expected_amount_minor,currency,frequency,next_expected_at,
			category_id,space_id,payment_account_id,status,detected_from_transaction_id,
			source,detection_key,confidence_basis_points,occurrence_count,user_modified_at,
			kind_confirmed_at
		) VALUES(
			$1,$2,$3,$4,$5,$6,$7,$8,NULL,$9,$10,$11,$12,$13,$14,now(),now()
		) RETURNING id::text
	`, string(userID), entity.name, entity.expectedAmountMinor, entity.currency,
		entity.frequency, entity.nextDate, entity.categoryID, entity.spaceID,
		entity.status, entity.detectedFromTransactionID, entity.source,
		entity.detectionKey, entity.confidenceBasisPoints, entity.occurrenceCount,
	).Scan(&targetID); err != nil {
		return "", fmt.Errorf("create reclassified subscription: %w", err)
	}
	result, err := tx.Exec(ctx, `DELETE FROM bills WHERE user_id=$1 AND id=$2`, string(userID), id)
	if err != nil {
		return "", fmt.Errorf("delete reclassified bill: %w", err)
	}
	if result.RowsAffected() != 1 {
		return "", ErrNotFound
	}
	return targetID, nil
}
