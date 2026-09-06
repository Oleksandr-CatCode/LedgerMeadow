package repository

import (
	"ledgermeadow/src/modules/cashflow/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) Get(c context.Context, u shared.UserID) ([]models.Snapshot, error) {
	rows, e := r.db.Query(c, `
		SELECT recent.currency, recent.period_start::text, recent.period_end::text,
		       recent.actual, recent.projected, recent.updated_at
		FROM unnest(ARRAY['CAD','USD']) requested(currency)
		CROSS JOIN LATERAL (
			SELECT currency, period_start, period_end, actual, projected, updated_at
			FROM cash_flow_snapshots
			WHERE user_id = $1 AND currency = requested.currency
			ORDER BY period_end DESC, period_start DESC
			LIMIT 6
		) recent
		ORDER BY recent.currency, recent.period_end DESC, recent.period_start DESC
		LIMIT 12
	`, string(u))
	if e != nil {
		return nil, fmt.Errorf("load cash flow snapshots: %w", e)
	}
	defer rows.Close()
	v := make([]models.Snapshot, 0, 12)
	for rows.Next() {
		var i models.Snapshot
		var actual, projected []byte
		if e := rows.Scan(&i.Currency, &i.PeriodStart, &i.PeriodEnd, &actual, &projected, &i.UpdatedAt); e != nil {
			return nil, fmt.Errorf("scan cash flow snapshot: %w", e)
		}
		if e := json.Unmarshal(actual, &i.Actual); e != nil {
			return nil, fmt.Errorf("decode actual cash flow: %w", e)
		}
		if e := json.Unmarshal(projected, &i.Projected); e != nil {
			return nil, fmt.Errorf("decode projected cash flow: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate cash flow snapshots: %w", e)
	}
	return v, nil
}
