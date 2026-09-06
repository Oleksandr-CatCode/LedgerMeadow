package repository

import (
	"ledgermeadow/src/modules/notifications/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("notification not found")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context, u shared.UserID) ([]models.Notification, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text,notification_type,title,body,entity_type,entity_id::text,read_at,created_at FROM notifications WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 20`, string(u))
	if e != nil {
		return nil, fmt.Errorf("list notifications: %w", e)
	}
	defer rows.Close()
	v := make([]models.Notification, 0, 20)
	for rows.Next() {
		var i models.Notification
		if e := rows.Scan(&i.ID, &i.Type, &i.Title, &i.Body, &i.EntityType, &i.EntityID, &i.ReadAt, &i.CreatedAt); e != nil {
			return nil, fmt.Errorf("scan notification: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate notifications: %w", e)
	}
	return v, nil
}
func (r *Repository) Preferences(ctx context.Context, u shared.UserID) ([]models.Preference, error) {
	rows, e := r.db.Query(ctx, `SELECT notification_type,in_app_enabled,email_enabled FROM notification_preferences WHERE user_id=$1 ORDER BY notification_type LIMIT 100`, string(u))
	if e != nil {
		return nil, fmt.Errorf("list notification preferences: %w", e)
	}
	defer rows.Close()
	v := make([]models.Preference, 0, 100)
	for rows.Next() {
		var i models.Preference
		if e := rows.Scan(&i.Type, &i.InAppEnabled, &i.EmailEnabled); e != nil {
			return nil, fmt.Errorf("scan notification preference: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate notification preferences: %w", e)
	}
	return v, nil
}
func (r *Repository) UpdatePreference(ctx context.Context, u shared.UserID, p models.Preference) error {
	_, e := r.db.Exec(ctx, `INSERT INTO notification_preferences(user_id,notification_type,in_app_enabled,email_enabled)VALUES($1,$2,$3,$4)ON CONFLICT(user_id,notification_type)DO UPDATE SET in_app_enabled=EXCLUDED.in_app_enabled,email_enabled=EXCLUDED.email_enabled,updated_at=now()`, string(u), p.Type, p.InAppEnabled, p.EmailEnabled)
	if e != nil {
		return fmt.Errorf("update notification preference: %w", e)
	}
	return nil
}
func (r *Repository) MarkRead(ctx context.Context, u shared.UserID, id string) error {
	command, err := r.db.Exec(ctx, `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND user_id=$2`, id, string(u))
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
