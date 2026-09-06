package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/budgets/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const limit = 100

var ErrTooMany = errors.New("budget response exceeds bound")
var ErrRelatedNotFound = errors.New("budget related entity not found")
var ErrInitializationBound = errors.New("budget initialization transaction bound exceeded")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context, u shared.UserID) ([]models.Budget, error) {
	rows, e := r.db.Query(ctx, `SELECT id::text,name,category_id::text,space_id::text,period,limit_minor,CASE WHEN period_start=CASE period WHEN 'WEEKLY' THEN date_trunc('week',CURRENT_DATE)::date WHEN 'MONTHLY' THEN date_trunc('month',CURRENT_DATE)::date ELSE date_trunc('year',CURRENT_DATE)::date END THEN spent_minor ELSE 0 END,currency,warning_threshold,critical_threshold,carryover_enabled FROM budgets WHERE user_id=$1 ORDER BY created_at,id LIMIT $2`, string(u), limit+1)
	if e != nil {
		return nil, fmt.Errorf("list budgets: %w", e)
	}
	defer rows.Close()
	v := make([]models.Budget, 0, limit)
	for rows.Next() {
		var i models.Budget
		if e := rows.Scan(&i.ID, &i.Name, &i.CategoryID, &i.SpaceID, &i.Period, &i.LimitMinor, &i.SpentMinor, &i.Currency, &i.WarningThreshold, &i.CriticalThreshold, &i.CarryoverEnabled); e != nil {
			return nil, fmt.Errorf("scan budget: %w", e)
		}
		v = append(v, i)
	}
	if e := rows.Err(); e != nil {
		return nil, fmt.Errorf("iterate budgets: %w", e)
	}
	if len(v) > limit {
		return nil, ErrTooMany
	}
	return v, nil
}
func (r *Repository) Create(ctx context.Context, u shared.UserID, c models.Create) (string, error) {
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if e != nil {
		return "", fmt.Errorf("begin budget create: %w", e)
	}
	defer tx.Rollback(ctx)
	var query string
	var idValue string
	var queryArgs []any
	if c.CategoryID != nil {
		query = `SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1 AND (user_id IS NULL OR user_id=$2))`
		idValue = *c.CategoryID
		queryArgs = []any{idValue, string(u)}
	} else {
		query = `SELECT EXISTS(SELECT 1 FROM spaces WHERE id=$1 AND user_id=$2 AND currency=$3)`
		idValue = *c.SpaceID
		queryArgs = []any{idValue, string(u), c.Currency}
	}
	var ok bool
	if e := tx.QueryRow(ctx, query, queryArgs...).Scan(&ok); e != nil {
		return "", fmt.Errorf("validate budget relation: %w", e)
	}
	if !ok {
		return "", ErrRelatedNotFound
	}
	spentMinor, periodStart, e := initialBudgetSpent(ctx, tx, u, c)
	if e != nil {
		return "", e
	}
	var id string
	if e := tx.QueryRow(ctx, `INSERT INTO budgets(user_id,name,category_id,space_id,period,limit_minor,spent_minor,currency,warning_threshold,critical_threshold,carryover_enabled,period_start)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)RETURNING id::text`, string(u), strings.TrimSpace(c.Name), c.CategoryID, c.SpaceID, c.Period, c.LimitMinor, spentMinor, c.Currency, c.WarningThreshold, c.CriticalThreshold, c.CarryoverEnabled, periodStart).Scan(&id); e != nil {
		return "", fmt.Errorf("create budget: %w", e)
	}
	if _, e := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id)VALUES($1,'BUDGET_CREATED',$2,$3)`, string(u), string(entities.Budget), id); e != nil {
		return "", fmt.Errorf("audit budget create: %w", e)
	}
	if e := tx.Commit(ctx); e != nil {
		return "", fmt.Errorf("commit budget create: %w", e)
	}
	return id, nil
}

func initialBudgetSpent(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	command models.Create,
) (int64, time.Time, error) {
	var periodStart, periodEnd time.Time
	if err := tx.QueryRow(ctx, `
		SELECT start_date,
		       CASE $1
		           WHEN 'WEEKLY' THEN start_date + 7
		           WHEN 'MONTHLY' THEN (start_date + INTERVAL '1 month')::date
		           ELSE (start_date + INTERVAL '1 year')::date
		       END
		FROM (
			SELECT CASE $1
			    WHEN 'WEEKLY' THEN date_trunc('week', CURRENT_DATE)::date
			    WHEN 'MONTHLY' THEN date_trunc('month', CURRENT_DATE)::date
			    ELSE date_trunc('year', CURRENT_DATE)::date
			END AS start_date
		) period
	`, command.Period).Scan(&periodStart, &periodEnd); err != nil {
		return 0, time.Time{}, fmt.Errorf("load initial budget period: %w", err)
	}
	var query string
	var relationID string
	if command.CategoryID != nil {
		query = `
			WITH candidates AS (
				SELECT -amount_minor AS spending_minor
				FROM transactions
				WHERE user_id = $1 AND category_id = $2 AND currency = $3
				  AND transaction_date >= $4 AND transaction_date < $5
				  AND amount_minor < 0 AND removed_at IS NULL
				ORDER BY transaction_date, id
				LIMIT 10001
			)
			SELECT count(*), COALESCE(sum(spending_minor), 0) FROM candidates
		`
		relationID = *command.CategoryID
	} else {
		query = `
			WITH candidates AS (
				SELECT -ta.amount_minor AS spending_minor
				FROM transaction_allocations ta
				JOIN transactions t ON t.id = ta.transaction_id
				WHERE ta.space_id = $2 AND t.user_id = $1 AND t.currency = $3
				  AND ta.transaction_date >= $4 AND ta.transaction_date < $5
				  AND t.transaction_date = ta.transaction_date
				  AND ta.amount_minor < 0 AND t.removed_at IS NULL
				ORDER BY ta.transaction_date, ta.transaction_id
				LIMIT 10001
			)
			SELECT count(*), COALESCE(sum(spending_minor), 0) FROM candidates
		`
		relationID = *command.SpaceID
	}
	var count int
	var spentMinor int64
	if err := tx.QueryRow(
		ctx, query, string(userID), relationID, command.Currency, periodStart, periodEnd,
	).Scan(&count, &spentMinor); err != nil {
		return 0, time.Time{}, fmt.Errorf("calculate initial budget spending: %w", err)
	}
	if count > 10000 {
		return 0, time.Time{}, ErrInitializationBound
	}
	return spentMinor, periodStart, nil
}
