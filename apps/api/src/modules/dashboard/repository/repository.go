package repository

import (
	"ledgermeadow/src/modules/dashboard/models"
	shared "ledgermeadow/src/shared/types"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
)

const monthlyBudgetLimit = 100

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) Get(ctx context.Context, u shared.UserID) (models.Dashboard, error) {
	tx, e := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("begin dashboard snapshot: %w", e)
	}
	defer tx.Rollback(ctx)
	var d models.Dashboard
	if e := tx.QueryRow(ctx, `SELECT open_inbox_count FROM users WHERE id=$1`, string(u)).Scan(&d.AttentionCount); e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard attention count: %w", e)
	}
	rows, e := tx.Query(ctx, `SELECT total_minor,available_minor,protected_minor,projected_month_end_minor,currency,status,breakdown,updated_at FROM financial_projections WHERE user_id=$1 ORDER BY currency LIMIT 2`, string(u))
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard projections: %w", e)
	}
	for rows.Next() {
		var i models.Projection
		var breakdown []byte
		if e := rows.Scan(&i.TotalMinor, &i.AvailableMinor, &i.ProtectedMinor, &i.ProjectedMonthEndMinor, &i.Currency, &i.Status, &breakdown, &i.UpdatedAt); e != nil {
			rows.Close()
			return models.Dashboard{}, fmt.Errorf("scan dashboard projection: %w", e)
		}
		if e := json.Unmarshal(breakdown, &i.Breakdown); e != nil || len(i.Breakdown) > 515 {
			rows.Close()
			return models.Dashboard{}, errors.New("stored available-to-spend breakdown is invalid")
		}
		for _, component := range i.Breakdown {
			if component.ID == "" || component.Label == "" {
				rows.Close()
				return models.Dashboard{}, errors.New("stored available-to-spend component is invalid")
			}
		}
		d.Projections = append(d.Projections, i)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return models.Dashboard{}, fmt.Errorf("iterate dashboard projections: %w", e)
	}
	rows.Close()
	rows, e = tx.Query(ctx, `SELECT id::text,name,space_type,currency,monthly_allocation_minor,CASE WHEN balance_period_start=date_trunc('month',CURRENT_DATE)::date THEN balance_minor ELSE monthly_allocation_minor END,protected,visibility FROM spaces WHERE user_id=$1 ORDER BY created_at,id LIMIT 6`, string(u))
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard spaces: %w", e)
	}
	for rows.Next() {
		var i models.Space
		if e := rows.Scan(&i.ID, &i.Name, &i.Type, &i.Currency, &i.MonthlyAllocationMinor, &i.BalanceMinor, &i.Protected, &i.Visibility); e != nil {
			rows.Close()
			return models.Dashboard{}, fmt.Errorf("scan dashboard space: %w", e)
		}
		d.Spaces = append(d.Spaces, i)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return models.Dashboard{}, fmt.Errorf("iterate dashboard spaces: %w", e)
	}
	rows.Close()
	rows, e = tx.Query(ctx, `SELECT id::text,name,limit_minor,CASE WHEN period_start=date_trunc('month',CURRENT_DATE)::date THEN spent_minor ELSE 0 END,currency FROM budgets WHERE user_id=$1 AND period='MONTHLY' ORDER BY CASE WHEN period_start=date_trunc('month',CURRENT_DATE)::date THEN spent_minor ELSE 0 END DESC,id LIMIT 6`, string(u))
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard budgets: %w", e)
	}
	for rows.Next() {
		var i models.Budget
		if e := rows.Scan(&i.ID, &i.Name, &i.LimitMinor, &i.SpentMinor, &i.Currency); e != nil {
			rows.Close()
			return models.Dashboard{}, fmt.Errorf("scan dashboard budget: %w", e)
		}
		d.Budgets = append(d.Budgets, i)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return models.Dashboard{}, fmt.Errorf("iterate dashboard budgets: %w", e)
	}
	rows.Close()
	rows, e = tx.Query(ctx, `WITH current_monthly_budgets AS (SELECT limit_minor,CASE WHEN period_start=date_trunc('month',CURRENT_DATE)::date THEN spent_minor ELSE 0 END spent_minor,currency FROM budgets WHERE user_id=$1 AND period='MONTHLY' ORDER BY created_at,id LIMIT $2) SELECT SUM(limit_minor)::text,SUM(spent_minor)::text,currency FROM current_monthly_budgets GROUP BY currency ORDER BY currency LIMIT 2`, string(u), monthlyBudgetLimit)
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard monthly plans: %w", e)
	}
	for rows.Next() {
		var i models.MonthlyPlan
		var plannedMinor, spentMinor string
		if e := rows.Scan(&plannedMinor, &spentMinor, &i.Currency); e != nil {
			rows.Close()
			return models.Dashboard{}, fmt.Errorf("scan dashboard monthly plan: %w", e)
		}
		planned, plannedErr := strconv.ParseInt(plannedMinor, 10, 64)
		spent, spentErr := strconv.ParseInt(spentMinor, 10, 64)
		if plannedErr != nil || spentErr != nil {
			rows.Close()
			return models.Dashboard{}, errors.New("dashboard monthly plan exceeds supported money range")
		}
		i.PlannedMinor = shared.MinorUnits(planned)
		i.SpentMinor = shared.MinorUnits(spent)
		d.MonthlyPlans = append(d.MonthlyPlans, i)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return models.Dashboard{}, fmt.Errorf("iterate dashboard monthly plans: %w", e)
	}
	rows.Close()
	rows, e = tx.Query(ctx, `SELECT id::text,item_type,name,amount_minor,currency,event_date::text FROM((SELECT id,'BILL'::text item_type,name,expected_amount_minor amount_minor,currency,next_due_at event_date FROM bills WHERE user_id=$1 AND status='ACTIVE' AND next_due_at>=CURRENT_DATE ORDER BY next_due_at,id LIMIT 10) UNION ALL (SELECT id,'SUBSCRIPTION',merchant_name,expected_amount_minor,currency,next_expected_at FROM subscriptions WHERE user_id=$1 AND status='ACTIVE' AND next_expected_at>=CURRENT_DATE ORDER BY next_expected_at,id LIMIT 10))upcoming ORDER BY event_date,id LIMIT 10`, string(u))
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard upcoming: %w", e)
	}
	for rows.Next() {
		var i models.Upcoming
		if e := rows.Scan(&i.ID, &i.Type, &i.Name, &i.AmountMinor, &i.Currency, &i.Date); e != nil {
			rows.Close()
			return models.Dashboard{}, fmt.Errorf("scan dashboard upcoming: %w", e)
		}
		d.Upcoming = append(d.Upcoming, i)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return models.Dashboard{}, fmt.Errorf("iterate dashboard upcoming: %w", e)
	}
	rows.Close()
	rows, e = tx.Query(ctx, `SELECT t.id::text,t.account_id::text,a.name,t.name,t.merchant_name,t.amount_minor,t.currency,t.transaction_date::text,CASE WHEN t.is_pending THEN 'PENDING' ELSE 'POSTED' END,c.name,allocation.space_name,t.review_status,t.visibility FROM transactions t JOIN accounts a ON a.id=t.account_id AND a.user_id=t.user_id LEFT JOIN categories c ON c.id=t.category_id LEFT JOIN LATERAL(SELECT s.name space_name FROM transaction_allocations ta JOIN spaces s ON s.id=ta.space_id WHERE ta.transaction_id=t.id ORDER BY ta.space_id LIMIT 1)allocation ON true WHERE t.user_id=$1 AND t.removed_at IS NULL ORDER BY t.transaction_date DESC,t.id DESC LIMIT 5`, string(u))
	if e != nil {
		return models.Dashboard{}, fmt.Errorf("load dashboard transactions: %w", e)
	}
	for rows.Next() {
		var i models.Transaction
		if e := rows.Scan(&i.ID, &i.AccountID, &i.AccountName, &i.Name, &i.MerchantName, &i.AmountMinor, &i.Currency, &i.Date, &i.Status, &i.CategoryName, &i.SpaceName, &i.ReviewStatus, &i.Visibility); e != nil {
			rows.Close()
			return models.Dashboard{}, fmt.Errorf("scan dashboard transaction: %w", e)
		}
		d.RecentTransactions = append(d.RecentTransactions, i)
	}
	if e := rows.Err(); e != nil {
		rows.Close()
		return models.Dashboard{}, fmt.Errorf("iterate dashboard transactions: %w", e)
	}
	rows.Close()
	if e := tx.Commit(ctx); e != nil {
		return models.Dashboard{}, fmt.Errorf("commit dashboard snapshot: %w", e)
	}
	return d, nil
}
