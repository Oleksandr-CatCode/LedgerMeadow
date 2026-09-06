package repository

import (
	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/manualassets/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
)

var ErrTooMany = errors.New("manual asset response exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context, u shared.UserID) ([]models.Asset, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text,name,asset_type,value_minor,currency,include_in_net_worth FROM manual_assets WHERE user_id=$1 ORDER BY created_at,id LIMIT 101`, string(u))
	if e != nil {
		return nil, fmt.Errorf("list manual assets: %w", e)
	}
	defer rows.Close()
	v := make([]models.Asset, 0, 100)
	for rows.Next() {
		var i models.Asset
		if e := rows.Scan(&i.ID, &i.Name, &i.Type, &i.ValueMinor, &i.Currency, &i.IncludeInNetWorth); e != nil {
			return nil, fmt.Errorf("scan manual asset: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate manual assets: %w", e)
	}
	if len(v) > 100 {
		return nil, ErrTooMany
	}
	return v, nil
}
func (r *Repository) Create(ctx context.Context, u shared.UserID, c models.Create) (string, error) {
	tx, e := r.db.Begin(ctx)
	if e != nil {
		return "", fmt.Errorf("begin manual asset create: %w", e)
	}
	defer tx.Rollback(ctx)
	var id string
	if e := tx.QueryRow(ctx, `INSERT INTO manual_assets(user_id,name,asset_type,value_minor,currency,include_in_net_worth)VALUES($1,$2,$3,$4,$5,$6)RETURNING id::text`, string(u), strings.TrimSpace(c.Name), c.Type, c.ValueMinor, c.Currency, c.IncludeInNetWorth).Scan(&id); e != nil {
		return "", fmt.Errorf("create manual asset: %w", e)
	}
	if _, e := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id)VALUES($1,'MANUAL_ASSET_CREATED',$2,$3)`, string(u), string(entities.ManualAsset), id); e != nil {
		return "", fmt.Errorf("audit manual asset create: %w", e)
	}
	if _, e := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(u), string(entities.User)); e != nil {
		return "", fmt.Errorf("enqueue manual asset materialization: %w", e)
	}
	if e := tx.Commit(ctx); e != nil {
		return "", fmt.Errorf("commit manual asset create: %w", e)
	}
	return id, nil
}
