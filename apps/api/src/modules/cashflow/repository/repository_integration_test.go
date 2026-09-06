package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGetScopesAndBoundsCashFlowHistory(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(db.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	userIDs := make([]shared.UserID, 2)
	for index := range userIDs {
		if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("cash-flow-user-%s-%d", suffix, index)).Scan(&userIDs[index]); err != nil {
			t.Fatalf("create user %d: %v", index, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=ANY($1::uuid[])`, []string{string(userIDs[0]), string(userIDs[1])})
	})

	for month := 1; month <= 7; month++ {
		periodStart := time.Date(2026, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 1, -1)
		if _, err := db.Exec(ctx, `
			INSERT INTO cash_flow_snapshots(user_id,currency,period_start,period_end,actual,projected)
			VALUES($1,'CAD',$2,$3,'{"income_minor":"100","fixed_outflow_minor":"0","variable_outflow_minor":"0","subscriptions_minor":"0","savings_minor":"0","total_outflow_minor":"0","net_minor":"100"}'::jsonb,'{"income_minor":"0","fixed_outflow_minor":"0","variable_outflow_minor":"0","subscriptions_minor":"0","savings_minor":"0","total_outflow_minor":"0","net_minor":"0"}'::jsonb)
		`, string(userIDs[0]), periodStart, periodEnd); err != nil {
			t.Fatalf("create owner snapshot %d: %v", month, err)
		}
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO cash_flow_snapshots(user_id,currency,period_start,period_end,actual,projected)
		VALUES($1,'CAD','2026-08-01','2026-08-31','{"income_minor":"999","fixed_outflow_minor":"0","variable_outflow_minor":"0","subscriptions_minor":"0","savings_minor":"0","total_outflow_minor":"0","net_minor":"999"}'::jsonb,'{"income_minor":"0","fixed_outflow_minor":"0","variable_outflow_minor":"0","subscriptions_minor":"0","savings_minor":"0","total_outflow_minor":"0","net_minor":"0"}'::jsonb)
	`, string(userIDs[1])); err != nil {
		t.Fatalf("create foreign snapshot: %v", err)
	}

	snapshots, err := New(db).Get(ctx, userIDs[0])
	if err != nil {
		t.Fatalf("get cash-flow history: %v", err)
	}
	if len(snapshots) != 6 {
		t.Fatalf("snapshot count = %d, want 6", len(snapshots))
	}
	if snapshots[0].PeriodStart != "2026-07-01" || snapshots[5].PeriodStart != "2026-02-01" {
		t.Fatalf("snapshot bounds = %s through %s", snapshots[0].PeriodStart, snapshots[5].PeriodStart)
	}
	for _, snapshot := range snapshots {
		if snapshot.Actual.IncomeMinor != 100 {
			t.Fatalf("foreign snapshot leaked into response: %#v", snapshot)
		}
	}
}
