package repository

import (
	"context"
	"fmt"

	"ledgermeadow/src/modules/activity/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context, userID shared.UserID) (models.Summary, error) {
	var summary models.Summary
	if err := r.db.QueryRow(ctx, `
		SELECT users.open_inbox_count,
		       (SELECT count(*)::integer FROM notifications
		        WHERE user_id = $1 AND read_at IS NULL),
		       GREATEST(
		           users.updated_at,
		           COALESCE((SELECT max(created_at) FROM notifications WHERE user_id = $1), users.updated_at),
		           COALESCE((SELECT max(updated_at) FROM financial_projections WHERE user_id = $1), users.updated_at)
		       )
		FROM users WHERE id = $1
	`, string(userID)).Scan(
		&summary.InboxCount,
		&summary.UnreadNotificationCount,
		&summary.UpdatedAt,
	); err != nil {
		return models.Summary{}, fmt.Errorf("load activity summary: %w", err)
	}
	return summary, nil
}
