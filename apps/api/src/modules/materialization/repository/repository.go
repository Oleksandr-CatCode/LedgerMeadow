package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"ledgermeadow/src/modules/materialization/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInputBound = errors.New("financial materialization input exceeds bound")

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Load(ctx context.Context, userID shared.UserID, now time.Time) (models.Input, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return models.Input{}, fmt.Errorf("begin materialization input: %w", err)
	}
	defer tx.Rollback(ctx)

	var timezone string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM users WHERE id = $1`, string(userID)).Scan(&timezone); err != nil {
		return models.Input{}, fmt.Errorf("load materialization timezone: %w", err)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return models.Input{}, errors.New("stored user timezone is invalid")
	}
	localNow := now.In(location)
	asOf := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	periodStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, location)
	periodEnd := periodStart.AddDate(0, 1, -1)

	byCurrency := make(map[string]*models.CurrencyInput, 2)
	currencyInput := func(currency string) (*models.CurrencyInput, error) {
		if currency != "CAD" && currency != "USD" {
			return nil, fmt.Errorf("unsupported materialization currency %q", currency)
		}
		input := byCurrency[currency]
		if input == nil {
			input = &models.CurrencyInput{Currency: currency, Analytics: make(map[string][]models.AnalyticsBucket, 6)}
			byCurrency[currency] = input
		}
		return input, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT a.id::text, left(a.name, 200), a.account_type, a.balance_minor, a.currency
		FROM accounts a
		JOIN bank_connections connection ON connection.id = a.bank_connection_id
		WHERE a.user_id = $1 AND connection.status <> 'DISCONNECTED'
		ORDER BY a.currency, a.id
		LIMIT 513
	`, string(userID))
	if err != nil {
		return models.Input{}, fmt.Errorf("load materialization accounts: %w", err)
	}
	accountCount := 0
	for rows.Next() {
		var id, label, accountType, currency string
		var amount int64
		if err := rows.Scan(&id, &label, &accountType, &amount, &currency); err != nil {
			rows.Close()
			return models.Input{}, fmt.Errorf("scan materialization account: %w", err)
		}
		accountCount++
		input, err := currencyInput(currency)
		if err != nil {
			rows.Close()
			return models.Input{}, err
		}
		if amount < 0 {
			if amount == math.MinInt64 {
				rows.Close()
				return models.Input{}, errors.New("account liability cannot be negated")
			}
			input.Liabilities = append(input.Liabilities, models.Item{ID: "account:" + id, Label: label, AmountMinor: -amount})
		} else {
			input.Assets = append(input.Assets, models.Item{ID: "account:" + id, Label: label, AmountMinor: amount})
		}
		if accountType == "depository" {
			input.LiquidBalanceMinor, err = checkedAdd(input.LiquidBalanceMinor, amount)
			if err != nil {
				rows.Close()
				return models.Input{}, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return models.Input{}, fmt.Errorf("iterate materialization accounts: %w", err)
	}
	rows.Close()
	if accountCount > models.MaxValuations {
		return models.Input{}, ErrInputBound
	}

	if err := loadManualValuations(ctx, tx, userID, byCurrency, currencyInput); err != nil {
		return models.Input{}, err
	}
	if err := loadSpaces(ctx, tx, userID, periodStart, currencyInput); err != nil {
		return models.Input{}, err
	}
	if err := loadProjectionSources(ctx, tx, userID, currencyInput); err != nil {
		return models.Input{}, err
	}
	if err := loadActualCashFlow(ctx, tx, userID, periodStart, periodEnd, currencyInput); err != nil {
		return models.Input{}, err
	}
	if err := loadAnalytics(ctx, tx, userID, periodStart, periodEnd, currencyInput); err != nil {
		return models.Input{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return models.Input{}, fmt.Errorf("commit materialization input: %w", err)
	}
	currencies := make([]models.CurrencyInput, 0, len(byCurrency))
	for _, input := range byCurrency {
		if len(input.Assets)+len(input.Liabilities) > models.MaxValuations {
			return models.Input{}, ErrInputBound
		}
		if len(input.ProtectedSpaces)+len(input.PlannedAllocations) > models.MaxBreakdownItems ||
			len(input.ProjectionSources) > models.MaxProjectionSources {
			return models.Input{}, ErrInputBound
		}
		currencies = append(currencies, *input)
	}
	sort.Slice(currencies, func(left, right int) bool { return currencies[left].Currency < currencies[right].Currency })
	return models.Input{AsOfDate: asOf, PeriodStart: periodStart, PeriodEnd: periodEnd, Currencies: currencies}, nil
}

func loadManualValuations(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	byCurrency map[string]*models.CurrencyInput,
	currencyInput func(string) (*models.CurrencyInput, error),
) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, left(name, 200), value_minor, currency, false AS liability
		FROM manual_assets WHERE user_id = $1 AND include_in_net_worth = true
		UNION ALL
		SELECT id::text, left(name, 200), balance_minor, currency, true
		FROM manual_liabilities WHERE user_id = $1 AND include_in_net_worth = true
		ORDER BY currency, liability, id
		LIMIT 513
	`, string(userID))
	if err != nil {
		return fmt.Errorf("load manual valuations: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, label, currency string
		var amount int64
		var liability bool
		if err := rows.Scan(&id, &label, &amount, &currency, &liability); err != nil {
			return fmt.Errorf("scan manual valuation: %w", err)
		}
		count++
		input, err := currencyInput(currency)
		if err != nil {
			return err
		}
		prefix := "asset:"
		if liability {
			prefix = "liability:"
		}
		item := models.Item{ID: prefix + id, Label: label, AmountMinor: amount}
		if liability {
			input.Liabilities = append(input.Liabilities, item)
		} else {
			input.Assets = append(input.Assets, item)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate manual valuations: %w", err)
	}
	if count > models.MaxValuations {
		return ErrInputBound
	}
	for _, input := range byCurrency {
		if len(input.Assets)+len(input.Liabilities) > models.MaxValuations {
			return ErrInputBound
		}
	}
	return nil
}

func loadSpaces(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	periodStart time.Time,
	currencyInput func(string) (*models.CurrencyInput, error),
) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, left(name, 200), currency, protected,
		       CASE WHEN balance_period_start = $2::date THEN GREATEST(balance_minor, 0) ELSE monthly_allocation_minor END,
		       monthly_allocation_minor
		FROM spaces WHERE user_id = $1
		ORDER BY currency, id LIMIT 513
	`, string(userID), periodStart)
	if err != nil {
		return fmt.Errorf("load materialization spaces: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var id, label, currency string
		var protected bool
		var balance, allocation int64
		if err := rows.Scan(&id, &label, &currency, &protected, &balance, &allocation); err != nil {
			return fmt.Errorf("scan materialization space: %w", err)
		}
		count++
		input, err := currencyInput(currency)
		if err != nil {
			return err
		}
		if protected && balance > 0 {
			input.ProtectedSpaces = append(input.ProtectedSpaces, models.Item{ID: "space:" + id, Label: label, AmountMinor: balance})
		} else if !protected && allocation > 0 {
			input.PlannedAllocations = append(input.PlannedAllocations, models.Item{ID: "space:" + id, Label: label, AmountMinor: allocation})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate materialization spaces: %w", err)
	}
	if count > models.MaxBreakdownItems {
		return ErrInputBound
	}
	return nil
}

func loadProjectionSources(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	currencyInput func(string) (*models.CurrencyInput, error),
) error {
	rows, err := tx.Query(ctx, `
		SELECT 'bill:' || id::text, left(name, 200), 'BILL', -expected_amount_minor, currency, next_due_at, frequency
		FROM bills WHERE user_id = $1 AND status = 'ACTIVE' AND expected_amount_minor > 0
		UNION ALL
		SELECT 'subscription:' || id::text, left(merchant_name, 200), 'SUBSCRIPTION', -expected_amount_minor, currency, next_expected_at, frequency
		FROM subscriptions WHERE user_id = $1 AND status = 'ACTIVE' AND expected_amount_minor > 0
		UNION ALL
		SELECT 'income:' || id::text, left(name, 200), 'SALARY', expected_amount_minor, currency, next_expected_at, frequency
		FROM recurring_income_sources WHERE user_id = $1 AND status = 'ACTIVE' AND expected_amount_minor > 0
		UNION ALL
		SELECT 'loan:' || id::text, left(name, 200), 'LOAN_PAYMENT', -monthly_payment_minor, currency, next_payment_at, 'MONTHLY'
		FROM loans WHERE user_id = $1 AND next_payment_at IS NOT NULL AND principal_remaining_minor > 0
		ORDER BY 5, 1 LIMIT 513
	`, string(userID))
	if err != nil {
		return fmt.Errorf("load projection sources: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var source models.ProjectionSource
		var currency string
		if err := rows.Scan(&source.ID, &source.Name, &source.Kind, &source.AmountMinor, &currency, &source.FirstDate, &source.Frequency); err != nil {
			return fmt.Errorf("scan projection source: %w", err)
		}
		count++
		input, err := currencyInput(currency)
		if err != nil {
			return err
		}
		input.ProjectionSources = append(input.ProjectionSources, source)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate projection sources: %w", err)
	}
	if count > models.MaxProjectionSources {
		return ErrInputBound
	}
	return nil
}

func loadActualCashFlow(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	periodStart time.Time,
	periodEnd time.Time,
	currencyInput func(string) (*models.CurrencyInput, error),
) error {
	rows, err := tx.Query(ctx, `
		SELECT t.currency,
		       COALESCE(sum(t.amount_minor) FILTER (WHERE t.amount_minor > 0), 0),
		       count(*) FILTER (WHERE t.amount_minor > 0),
		       COALESCE(sum(-t.amount_minor) FILTER (
		           WHERE t.amount_minor < 0 AND EXISTS (
		               SELECT 1 FROM bills bill
		               WHERE bill.user_id = t.user_id AND bill.status = 'ACTIVE'
		                 AND bill.detection_key = t.recurring_detection_key
		           )
		       ), 0),
		       COALESCE(sum(-t.amount_minor) FILTER (
		           WHERE t.amount_minor < 0 AND NOT EXISTS (
		               SELECT 1 FROM subscriptions subscription
		               WHERE subscription.user_id = t.user_id AND subscription.status = 'ACTIVE'
		                 AND subscription.detection_key = t.recurring_detection_key
		           )
		           AND NOT EXISTS (
		               SELECT 1 FROM bills bill
		               WHERE bill.user_id = t.user_id AND bill.status = 'ACTIVE'
		                 AND bill.detection_key = t.recurring_detection_key
		           )
		       ), 0),
		       COALESCE(sum(-t.amount_minor) FILTER (
		           WHERE t.amount_minor < 0 AND EXISTS (
		               SELECT 1 FROM subscriptions subscription
		               WHERE subscription.user_id = t.user_id AND subscription.status = 'ACTIVE'
		                 AND subscription.detection_key = t.recurring_detection_key
		           )
		       ), 0),
		       count(*) FILTER (WHERE t.amount_minor < 0)
		FROM transactions t
		LEFT JOIN categories category ON category.id = t.category_id
		WHERE t.user_id = $1 AND t.removed_at IS NULL AND NOT t.is_pending
		  AND t.transaction_date BETWEEN $2::date AND $3::date
		  AND COALESCE(category.category_type, '') <> 'TRANSFER'
		GROUP BY t.currency ORDER BY t.currency LIMIT 3
	`, string(userID), periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("load actual cash flow: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var currency string
		var actual models.CashFlowComponents
		var incomeCount, outflowCount int64
		if err := rows.Scan(
			&currency, &actual.IncomeMinor, &incomeCount, &actual.FixedOutflowMinor,
			&actual.VariableOutflowMinor,
			&actual.SubscriptionsMinor, &outflowCount,
		); err != nil {
			return fmt.Errorf("scan actual cash flow: %w", err)
		}
		if incomeCount < 0 || incomeCount > math.MaxUint32 || outflowCount < 0 || outflowCount > math.MaxUint32 {
			return errors.New("cash-flow item count is invalid")
		}
		actual.IncomeCount = uint32(incomeCount)
		actual.OutflowCount = uint32(outflowCount)
		count++
		input, err := currencyInput(currency)
		if err != nil {
			return err
		}
		input.Actual = actual
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate actual cash flow: %w", err)
	}
	if count > 2 {
		return ErrInputBound
	}
	return nil
}

func loadAnalytics(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	periodStart time.Time,
	periodEnd time.Time,
	currencyInput func(string) (*models.CurrencyInput, error),
) error {
	queries := []struct {
		views []string
		sql   string
	}{
		{[]string{"SPENDING", "CATEGORIES"}, `
			SELECT t.currency, COALESCE(category.id::text, 'uncategorized'),
			       left(COALESCE(category.name, 'Uncategorized'), 200), sum(-t.amount_minor), count(*)
			FROM transactions t LEFT JOIN categories category ON category.id = t.category_id
			WHERE t.user_id = $1 AND t.removed_at IS NULL AND NOT t.is_pending AND t.amount_minor < 0
			  AND t.transaction_date BETWEEN $2::date AND $3::date
			  AND COALESCE(category.category_type, '') <> 'TRANSFER'
			GROUP BY t.currency, category.id, category.name ORDER BY t.currency, sum(-t.amount_minor) DESC LIMIT 513
		`},
		{[]string{"MERCHANTS"}, `
			SELECT t.currency, md5('expense:' || t.currency || ':' || COALESCE(NULLIF(t.merchant_name, ''), t.name)),
			       left(COALESCE(NULLIF(t.merchant_name, ''), t.name), 200), sum(-t.amount_minor), count(*)
			FROM transactions t LEFT JOIN categories category ON category.id = t.category_id
			WHERE t.user_id = $1 AND t.removed_at IS NULL AND NOT t.is_pending AND t.amount_minor < 0
			  AND t.transaction_date BETWEEN $2::date AND $3::date
			  AND COALESCE(category.category_type, '') <> 'TRANSFER'
			GROUP BY t.currency, COALESCE(NULLIF(t.merchant_name, ''), t.name)
			ORDER BY t.currency, sum(-t.amount_minor) DESC LIMIT 513
		`},
		{[]string{"INCOME"}, `
			SELECT t.currency, md5('income:' || t.currency || ':' || COALESCE(NULLIF(t.merchant_name, ''), t.name)),
			       left(COALESCE(NULLIF(t.merchant_name, ''), t.name), 200), sum(t.amount_minor), count(*)
			FROM transactions t LEFT JOIN categories category ON category.id = t.category_id
			WHERE t.user_id = $1 AND t.removed_at IS NULL AND NOT t.is_pending AND t.amount_minor > 0
			  AND t.transaction_date BETWEEN $2::date AND $3::date
			  AND COALESCE(category.category_type, '') <> 'TRANSFER'
			GROUP BY t.currency, COALESCE(NULLIF(t.merchant_name, ''), t.name)
			ORDER BY t.currency, sum(t.amount_minor) DESC LIMIT 513
		`},
	}
	for _, query := range queries {
		rows, err := tx.Query(ctx, query.sql, string(userID), periodStart, periodEnd)
		if err != nil {
			return fmt.Errorf("load analytics buckets: %w", err)
		}
		count := 0
		for rows.Next() {
			var currency string
			var bucket models.AnalyticsBucket
			var itemCount int64
			if err := rows.Scan(&currency, &bucket.ID, &bucket.Label, &bucket.AmountMinor, &itemCount); err != nil {
				rows.Close()
				return fmt.Errorf("scan analytics bucket: %w", err)
			}
			if itemCount < 0 || itemCount > math.MaxUint32 {
				rows.Close()
				return errors.New("analytics item count is invalid")
			}
			bucket.ItemCount = uint32(itemCount)
			count++
			input, err := currencyInput(currency)
			if err != nil {
				rows.Close()
				return err
			}
			for _, view := range query.views {
				input.Analytics[view] = append(input.Analytics[view], bucket)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate analytics buckets: %w", err)
		}
		rows.Close()
		if count > models.MaxAnalyticsBuckets {
			return ErrInputBound
		}
	}
	return nil
}

func (r *Repository) Persist(ctx context.Context, userID shared.UserID, result models.Result) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin financial materialization write: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, currency := range result.Currencies {
		if _, err := tx.Exec(ctx, `
			INSERT INTO financial_projections (
				user_id, currency, total_minor, available_minor, protected_minor, projected_month_end_minor, status, breakdown
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (user_id, currency) DO UPDATE SET
				total_minor = EXCLUDED.total_minor, available_minor = EXCLUDED.available_minor,
				protected_minor = EXCLUDED.protected_minor,
				projected_month_end_minor = EXCLUDED.projected_month_end_minor,
				status = EXCLUDED.status, breakdown = EXCLUDED.breakdown, updated_at = now()
		`, string(userID), currency.Currency, currency.TotalMinor, currency.AvailableMinor,
			currency.ProtectedMinor, currency.ProjectedMonthEndMinor, currency.ProjectionStatus,
			currency.BreakdownJSON); err != nil {
			return fmt.Errorf("persist financial projection: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO net_worth_snapshots (
				user_id, snapshot_date, value_minor, currency, total_assets_minor, total_liabilities_minor
			) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, snapshot_date, currency) DO UPDATE SET
				value_minor = EXCLUDED.value_minor, total_assets_minor = EXCLUDED.total_assets_minor,
				total_liabilities_minor = EXCLUDED.total_liabilities_minor
		`, string(userID), result.AsOfDate, currency.NetWorthMinor, currency.Currency,
			currency.TotalAssetsMinor, currency.TotalLiabilitiesMinor); err != nil {
			return fmt.Errorf("persist net worth snapshot: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cash_flow_snapshots (user_id, currency, period_start, period_end, actual, projected)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (user_id, currency, period_start, period_end) DO UPDATE SET
				actual = EXCLUDED.actual, projected = EXCLUDED.projected, updated_at = now()
		`, string(userID), currency.Currency, result.PeriodStart, result.PeriodEnd,
			currency.ActualCashFlowJSON, currency.ProjectedCashFlowJSON); err != nil {
			return fmt.Errorf("persist cash-flow snapshot: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO timeline_snapshots (
				user_id, currency, as_of_date, projection_end_date, starting_balance_minor,
				ending_balance_minor, minimum_balance_minor, minimum_balance_date, points
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (user_id, currency, as_of_date) DO UPDATE SET
				projection_end_date = EXCLUDED.projection_end_date,
				starting_balance_minor = EXCLUDED.starting_balance_minor,
				ending_balance_minor = EXCLUDED.ending_balance_minor,
				minimum_balance_minor = EXCLUDED.minimum_balance_minor,
				minimum_balance_date = EXCLUDED.minimum_balance_date,
				points = EXCLUDED.points, updated_at = now()
		`, string(userID), currency.Currency, result.AsOfDate, currency.TimelineEndDate,
			currency.TimelineStartingMinor, currency.TimelineEndingMinor, currency.TimelineMinimumMinor,
			currency.TimelineMinimumDate, currency.TimelineJSON); err != nil {
			return fmt.Errorf("persist timeline snapshot: %w", err)
		}
		for _, analytics := range currency.Analytics {
			if _, err := tx.Exec(ctx, `
				INSERT INTO analytics_snapshots (
					user_id, view_type, currency, period_start, period_end, total_minor, breakdown
				) VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (user_id, view_type, currency, period_start, period_end) DO UPDATE SET
					total_minor = EXCLUDED.total_minor, breakdown = EXCLUDED.breakdown, updated_at = now()
			`, string(userID), analytics.ViewType, currency.Currency, result.PeriodStart,
				result.PeriodEnd, analytics.TotalMinor, analytics.BreakdownJSON); err != nil {
				return fmt.Errorf("persist analytics snapshot: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit financial materialization: %w", err)
	}
	return nil
}

func checkedAdd(left int64, right int64) (int64, error) {
	if (right > 0 && left > math.MaxInt64-right) || (right < 0 && left < math.MinInt64-right) {
		return 0, errors.New("financial materialization arithmetic overflow")
	}
	return left + right, nil
}
