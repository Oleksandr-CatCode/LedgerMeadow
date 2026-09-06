package repository

import (
	"ledgermeadow/src/modules/timeline/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) Get(c context.Context, u shared.UserID) ([]models.Snapshot, error) {
	rows, e := r.db.Query(c, `SELECT latest.currency,latest.as_of_date::text,latest.projection_end_date::text,latest.starting_balance_minor,latest.ending_balance_minor,latest.minimum_balance_minor,latest.minimum_balance_date::text,latest.points,latest.updated_at FROM unnest(ARRAY['CAD','USD']) requested(currency) CROSS JOIN LATERAL(SELECT currency,as_of_date,projection_end_date,starting_balance_minor,ending_balance_minor,minimum_balance_minor,minimum_balance_date,points,updated_at FROM timeline_snapshots WHERE user_id=$1 AND currency=requested.currency ORDER BY as_of_date DESC LIMIT 1)latest ORDER BY latest.currency LIMIT 2`, string(u))
	if e != nil {
		return nil, fmt.Errorf("load timeline snapshots: %w", e)
	}
	defer rows.Close()
	v := make([]models.Snapshot, 0, 2)
	for rows.Next() {
		var i models.Snapshot
		var points []byte
		if e := rows.Scan(
			&i.Currency, &i.AsOfDate, &i.ProjectionEndDate, &i.StartingBalanceMinor,
			&i.EndingBalanceMinor, &i.MinimumBalanceMinor, &i.MinimumBalanceDate,
			&points, &i.UpdatedAt,
		); e != nil {
			return nil, fmt.Errorf("scan timeline snapshot: %w", e)
		}
		if e := json.Unmarshal(points, &i.Points); e != nil || len(i.Points) > 512 {
			return nil, errors.New("stored timeline points are invalid")
		}
		for _, point := range i.Points {
			if point.SourceID == "" || point.Name == "" || point.Date == "" {
				return nil, errors.New("stored timeline point is invalid")
			}
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate timeline snapshots: %w", e)
	}
	return v, nil
}
