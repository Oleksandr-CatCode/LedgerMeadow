package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/platform/changes"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoEvent = errors.New("no outbox event available")

type Event struct {
	ID          string
	EventType   string
	UserID      shared.UserID
	AggregateID string
	Attempts    int
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnqueueOutstandingAnalysis(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO outbox_events(
			user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload
		)
		SELECT candidate.user_id,$1,candidate.user_id,
		       'TRANSACTION_ANALYSIS_REQUESTED',
		       'transaction-analysis:outstanding:'||candidate.user_id::text||':'||gen_random_uuid()::text,
		       '{}'::jsonb
		FROM (
			SELECT DISTINCT transaction.user_id
			FROM transactions transaction
			WHERE transaction.removed_at IS NULL AND NOT transaction.is_pending
			  AND transaction.category_id IS NULL AND transaction.category_source='UNASSIGNED'
			  AND NOT EXISTS (
			      SELECT 1 FROM inbox_items item
			      WHERE item.user_id=transaction.user_id AND item.entity_id=transaction.id
			        AND item.item_type='TRANSACTION_REVIEW' AND item.status='OPEN'
			        AND item.payload->>'reason'='CATEGORY_UNCERTAIN'
			  )
			  AND NOT EXISTS (
			      SELECT 1 FROM outbox_events event
			      WHERE event.user_id=transaction.user_id
			        AND event.event_type='TRANSACTION_ANALYSIS_REQUESTED'
			        AND event.status IN ('PENDING','PROCESSING')
			  )
		) candidate
	`, string(entities.User)); err != nil {
		return fmt.Errorf("enqueue outstanding transaction analysis: %w", err)
	}
	return nil
}

func (r *Repository) Claim(ctx context.Context) (Event, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer tx.Rollback(ctx)

	var event Event
	err = tx.QueryRow(ctx, `
		SELECT id::text, event_type, user_id::text, aggregate_id::text, attempts
		FROM outbox_events
		WHERE available_at <= now()
		  AND (
			status = 'PENDING'
			OR (status = 'PROCESSING' AND locked_at < now() - interval '5 minutes')
		  )
		ORDER BY created_at
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`).Scan(&event.ID, &event.EventType, &event.UserID, &event.AggregateID, &event.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNoEvent
	}
	if err != nil {
		return Event{}, fmt.Errorf("select outbox event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'PROCESSING', attempts = attempts + 1, locked_at = now()
		WHERE id = $1
	`, event.ID); err != nil {
		return Event{}, fmt.Errorf("lock outbox event: %w", err)
	}
	event.Attempts++
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("commit outbox claim: %w", err)
	}
	return event, nil
}

func (r *Repository) CompleteBankSync(ctx context.Context, event Event) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin bank-sync completion: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'COMPLETED', processed_at = now(), locked_at = NULL, last_error_code = NULL
		WHERE id = $1
	`, event.ID); err != nil {
		return fmt.Errorf("complete bank-sync event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		) VALUES ($1, $2, $1, 'TRANSACTION_ANALYSIS_REQUESTED', $3, '{}'::jsonb)
		ON CONFLICT (dedupe_key) DO NOTHING
	`, string(event.UserID), string(entities.User), "transaction-analysis:sync:"+event.ID); err != nil {
		return fmt.Errorf("enqueue post-sync transaction analysis: %w", err)
	}
	if err := changes.Notify(ctx, tx, event.UserID, changes.ResourceAccounts, changes.ResourceActivity); err != nil {
		return fmt.Errorf("notify bank-sync completion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bank-sync completion: %w", err)
	}
	return nil
}

func (r *Repository) CompleteAnalysis(ctx context.Context, event Event) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction-analysis completion: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'COMPLETED', processed_at = now(), locked_at = NULL, last_error_code = NULL
		WHERE id = $1
	`, event.ID); err != nil {
		return fmt.Errorf("complete transaction-analysis event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (
			user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload
		) VALUES ($1, $2, $1, 'FINANCIAL_MATERIALIZATION_REQUESTED', $3, '{}'::jsonb)
		ON CONFLICT (dedupe_key) DO NOTHING
	`, string(event.UserID), string(entities.User), "financial-materialization:analysis:"+event.ID); err != nil {
		return fmt.Errorf("enqueue post-analysis materialization: %w", err)
	}
	if err := changes.Notify(ctx, tx, event.UserID, changes.ResourceInbox, changes.ResourceNotifications, changes.ResourceActivity); err != nil {
		return fmt.Errorf("notify transaction-analysis completion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction-analysis completion: %w", err)
	}
	return nil
}

func (r *Repository) CompleteMaterialization(ctx context.Context, event Event) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin materialization completion: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'COMPLETED', processed_at = now(), locked_at = NULL, last_error_code = NULL
		WHERE id = $1
	`, event.ID); err != nil {
		return fmt.Errorf("complete materialization event: %w", err)
	}
	if err := changes.Notify(ctx, tx, event.UserID, changes.ResourceDashboard, changes.ResourceActivity); err != nil {
		return fmt.Errorf("notify materialization completion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit materialization completion: %w", err)
	}
	return nil
}

func (r *Repository) Fail(ctx context.Context, event Event, errorCode string) error {
	if event.Attempts >= 5 {
		_, err := r.db.Exec(ctx, `
			UPDATE outbox_events
			SET status = 'FAILED', locked_at = NULL, last_error_code = $2
			WHERE id = $1
		`, event.ID, errorCode)
		return err
	}
	backoff := time.Duration(event.Attempts*event.Attempts) * time.Second
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'PENDING', available_at = now() + $2::interval,
		    locked_at = NULL, last_error_code = $3
		WHERE id = $1
	`, event.ID, backoff.String(), errorCode)
	return err
}
