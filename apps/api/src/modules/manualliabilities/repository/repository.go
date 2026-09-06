package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/manualliabilities/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrTooMany = errors.New("manual liability response exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.Liability, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, liability_type, balance_minor, currency, include_in_net_worth
		FROM manual_liabilities WHERE user_id = $1 ORDER BY created_at, id LIMIT 101
	`, string(userID))
	if err != nil {
		return nil, fmt.Errorf("list manual liabilities: %w", err)
	}
	defer rows.Close()
	items := make([]models.Liability, 0, 100)
	for rows.Next() {
		var item models.Liability
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.BalanceMinor, &item.Currency,
			&item.IncludeInNetWorth); err != nil {
			return nil, fmt.Errorf("scan manual liability: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate manual liabilities: %w", err)
	}
	if len(items) > 100 {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin manual liability create: %w", err)
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO manual_liabilities (
			user_id, name, liability_type, balance_minor, currency, include_in_net_worth
		) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text
	`, string(userID), strings.TrimSpace(command.Name), command.Type, command.BalanceMinor,
		command.Currency, command.IncludeInNetWorth).Scan(&id); err != nil {
		return "", fmt.Errorf("create manual liability: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'MANUAL_LIABILITY_CREATED', $2, $3)
	`, string(userID), string(entities.ManualLiability), id); err != nil {
		return "", fmt.Errorf("audit manual liability create: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload)
		VALUES ($1, $2, $1, 'FINANCIAL_MATERIALIZATION_REQUESTED', gen_random_uuid()::text, '{}'::jsonb)
	`, string(userID), string(entities.User)); err != nil {
		return "", fmt.Errorf("enqueue manual liability materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit manual liability create: %w", err)
	}
	return id, nil
}
