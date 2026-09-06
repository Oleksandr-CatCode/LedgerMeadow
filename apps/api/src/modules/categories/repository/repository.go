package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/categories/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listLimit = 200

var ErrTooMany = errors.New("category response exceeds bound")
var ErrParentNotFound = errors.New("category parent not found")
var ErrLimitReached = errors.New("category limit reached")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.Category, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, category_type, parent_id::text
		FROM categories
		WHERE user_id IS NULL OR user_id = $1
		ORDER BY name, id
		LIMIT $2
	`, string(userID), listLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()
	items := make([]models.Category, 0, listLimit)
	for rows.Next() {
		var item models.Category
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.ParentID); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}
	if len(items) > listLimit {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin category create: %w", err)
	}
	defer tx.Rollback(ctx)
	var lockedUserID string
	if err := tx.QueryRow(ctx, `
		SELECT id::text FROM users WHERE id = $1 FOR UPDATE
	`, string(userID)).Scan(&lockedUserID); err != nil {
		return "", fmt.Errorf("lock category owner: %w", err)
	}
	var categoryCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM (
			SELECT 1 FROM categories
			WHERE user_id IS NULL OR user_id = $1
			LIMIT $2
		) visible_categories
	`, string(userID), listLimit).Scan(&categoryCount); err != nil {
		return "", fmt.Errorf("count visible categories: %w", err)
	}
	if categoryCount >= listLimit {
		return "", ErrLimitReached
	}
	if command.ParentID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM categories WHERE id = $1 AND (user_id IS NULL OR user_id = $2))
		`, *command.ParentID, string(userID)).Scan(&exists); err != nil {
			return "", fmt.Errorf("validate category parent: %w", err)
		}
		if !exists {
			return "", ErrParentNotFound
		}
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO categories (user_id, name, category_type, parent_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text
	`, string(userID), strings.TrimSpace(command.Name), command.Type, command.ParentID).Scan(&id); err != nil {
		return "", fmt.Errorf("create category: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'CATEGORY_CREATED', $2, $3)
	`, string(userID), string(entities.Category), id); err != nil {
		return "", fmt.Errorf("audit category create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit category create: %w", err)
	}
	return id, nil
}
