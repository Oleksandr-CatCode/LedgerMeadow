package repository

import (
	"ledgermeadow/src/modules/analytics/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) Get(ctx context.Context, userID shared.UserID) ([]models.Snapshot, error) {
	rows, err := r.db.Query(ctx, `
		SELECT latest.view_type, latest.currency, latest.period_start::text,
		       latest.period_end::text, latest.total_minor, latest.breakdown, latest.updated_at
		FROM unnest(ARRAY['SPENDING','INCOME','CASH_FLOW','CATEGORIES','MERCHANTS','RECURRING']) requested_view(view_type)
		CROSS JOIN unnest(ARRAY['CAD','USD']) requested_currency(currency)
		CROSS JOIN LATERAL (
			SELECT view_type, currency, period_start, period_end, total_minor, breakdown, updated_at
			FROM analytics_snapshots
			WHERE user_id = $1 AND view_type = requested_view.view_type
			  AND currency = requested_currency.currency
			ORDER BY period_end DESC, period_start DESC LIMIT 1
		) latest
		ORDER BY latest.view_type, latest.currency
		LIMIT 12
	`, string(userID))
	if err != nil {
		return nil, fmt.Errorf("load analytics snapshots: %w", err)
	}
	defer rows.Close()
	items := make([]models.Snapshot, 0, 12)
	for rows.Next() {
		var item models.Snapshot
		var breakdown []byte
		if err := rows.Scan(&item.ViewType, &item.Currency, &item.PeriodStart, &item.PeriodEnd,
			&item.TotalMinor, &breakdown, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan analytics snapshot: %w", err)
		}
		if err := json.Unmarshal(breakdown, &item.Breakdown); err != nil || len(item.Breakdown) > 512 {
			return nil, errors.New("stored analytics breakdown is invalid")
		}
		for _, bucket := range item.Breakdown {
			if bucket.ID == "" || bucket.Label == "" || bucket.ShareBasisPoints > 10000 {
				return nil, errors.New("stored analytics bucket is invalid")
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate analytics snapshots: %w", err)
	}
	return items, nil
}
