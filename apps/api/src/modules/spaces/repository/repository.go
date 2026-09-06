package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/spaces/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listLimit = 100

var ErrTooMany = errors.New("space response exceeds bound")
var ErrHouseholdRequired = errors.New("household membership is required for a shared Space")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.Space, error) {
	rows, err := r.db.Query(ctx, `
		SELECT s.id::text, s.name, s.space_type, s.currency,
		       s.monthly_allocation_minor,
		       CASE
		           WHEN s.balance_period_start = date_trunc('month', CURRENT_DATE)::date THEN s.balance_minor
		           ELSE s.monthly_allocation_minor
		       END AS balance_minor,
		       s.protected, s.visibility
		FROM spaces s
		WHERE s.user_id = $1
		ORDER BY s.created_at, s.id
		LIMIT $2
	`, string(userID), listLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list spaces: %w", err)
	}
	defer rows.Close()
	items := make([]models.Space, 0, listLimit)
	for rows.Next() {
		var item models.Space
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Currency,
			&item.MonthlyAllocationMinor, &item.BalanceMinor, &item.Protected, &item.Visibility); err != nil {
			return nil, fmt.Errorf("scan space: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate spaces: %w", err)
	}
	if len(items) > listLimit {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin space create: %w", err)
	}
	defer tx.Rollback(ctx)
	var householdID *string
	if command.Visibility == "HOUSEHOLD" {
		var id string
		err := tx.QueryRow(ctx, `
			SELECT household_id::text
			FROM household_members
			WHERE user_id = $1
			FOR SHARE
		`, string(userID)).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrHouseholdRequired
		}
		if err != nil {
			return "", fmt.Errorf("load shared Space household: %w", err)
		}
		householdID = &id
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO spaces (
			user_id, household_id, name, space_type, currency, monthly_allocation_minor,
			balance_minor, protected, visibility
		) VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8)
		RETURNING id::text
	`, string(userID), householdID, strings.TrimSpace(command.Name), command.Type, command.Currency,
		command.MonthlyAllocationMinor, command.Protected, command.Visibility).Scan(&id); err != nil {
		return "", fmt.Errorf("create space: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'SPACE_CREATED', $2, $3)
	`, string(userID), string(entities.Space), id); err != nil {
		return "", fmt.Errorf("audit space create: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (user_id, aggregate_type, aggregate_id, event_type, dedupe_key, payload)
		VALUES ($1, $2, $1, 'FINANCIAL_MATERIALIZATION_REQUESTED', gen_random_uuid()::text, '{}'::jsonb)
	`, string(userID), string(entities.User)); err != nil {
		return "", fmt.Errorf("enqueue Space materialization: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit space create: %w", err)
	}
	return id, nil
}
