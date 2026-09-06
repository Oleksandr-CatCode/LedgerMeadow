package repository

import (
	"context"
	"fmt"

	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ResolveByClerkID(ctx context.Context, clerkUserID string) (shared.UserID, error) {
	var userID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO users (clerk_user_id)
		VALUES ($1)
		ON CONFLICT (clerk_user_id) DO UPDATE
		SET clerk_user_id = EXCLUDED.clerk_user_id
		RETURNING id::text
	`, clerkUserID).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("resolve application user: %w", err)
	}
	return shared.UserID(userID), nil
}
