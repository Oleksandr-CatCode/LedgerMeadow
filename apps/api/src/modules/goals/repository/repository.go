package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/goals/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listLimit = 100

var ErrTooMany = errors.New("goal response exceeds bound")
var ErrSpaceNotFound = errors.New("goal space not found")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.Goal, error) {
	rows, err := r.db.Query(ctx, `
		SELECT g.id::text, g.name, g.target_minor, g.current_minor, g.currency,
		       g.target_date::text, g.space_id::text, s.name, g.status
		FROM goals g
		LEFT JOIN spaces s ON s.id = g.space_id AND s.user_id = g.user_id
		WHERE g.user_id = $1
		ORDER BY g.status, g.target_date NULLS LAST, g.id
		LIMIT $2
	`, string(userID), listLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	defer rows.Close()
	items := make([]models.Goal, 0, listLimit)
	for rows.Next() {
		var item models.Goal
		if err := rows.Scan(&item.ID, &item.Name, &item.TargetMinor, &item.CurrentMinor,
			&item.Currency, &item.TargetDate, &item.SpaceID, &item.SpaceName, &item.Status); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate goals: %w", err)
	}
	if len(items) > listLimit {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin goal create: %w", err)
	}
	defer tx.Rollback(ctx)
	if command.SpaceID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM spaces WHERE id = $1 AND user_id = $2 AND currency = $3)`,
			*command.SpaceID, string(userID), command.Currency).Scan(&exists); err != nil {
			return "", fmt.Errorf("validate goal space: %w", err)
		}
		if !exists {
			return "", ErrSpaceNotFound
		}
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO goals (user_id, name, target_minor, current_minor, currency, target_date, space_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text
	`, string(userID), strings.TrimSpace(command.Name), command.TargetMinor, command.CurrentMinor,
		command.Currency, command.TargetDate, command.SpaceID).Scan(&id); err != nil {
		return "", fmt.Errorf("create goal: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'GOAL_CREATED', $2, $3)
	`, string(userID), string(entities.Goal), id); err != nil {
		return "", fmt.Errorf("audit goal create: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload)
		VALUES ($1, $2, $1, 'FINANCIAL_MATERIALIZATION_REQUESTED', gen_random_uuid()::text, '{}'::jsonb)
	`, string(userID), string(entities.User)); err != nil {
		return "", fmt.Errorf("enqueue goal materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit goal create: %w", err)
	}
	return id, nil
}
