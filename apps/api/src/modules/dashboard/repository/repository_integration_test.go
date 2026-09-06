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

func TestGetScopesMonthlyPlanAndUpcomingToOwner(t *testing.T) {
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
	var ownerID, otherUserID shared.UserID
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "dashboard-owner-"+suffix).Scan(&ownerID); err != nil {
		t.Fatalf("create dashboard owner: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "dashboard-other-"+suffix).Scan(&otherUserID); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{string(ownerID), string(otherUserID)})
	})

	var ownerCategoryID, otherCategoryID string
	if err := db.QueryRow(ctx, `INSERT INTO categories(user_id,name,category_type) VALUES($1,'Synthetic groceries','EXPENSE') RETURNING id::text`, string(ownerID)).Scan(&ownerCategoryID); err != nil {
		t.Fatalf("create owner category: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO categories(user_id,name,category_type) VALUES($1,'Foreign category','EXPENSE') RETURNING id::text`, string(otherUserID)).Scan(&otherCategoryID); err != nil {
		t.Fatalf("create other category: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO budgets(user_id,name,category_id,period,limit_minor,spent_minor,period_start,currency,warning_threshold,critical_threshold) VALUES($1,'Synthetic groceries',$2,'MONTHLY',60000,30000,date_trunc('month',CURRENT_DATE)::date,'CAD',80,95),($1,'Synthetic transit',$2,'MONTHLY',40000,10000,date_trunc('month',CURRENT_DATE)::date,'CAD',80,95)`, string(ownerID), ownerCategoryID); err != nil {
		t.Fatalf("create owner budgets: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO budgets(user_id,name,category_id,period,limit_minor,spent_minor,period_start,currency,warning_threshold,critical_threshold) VALUES($1,'Foreign budget',$2,'MONTHLY',900000,800000,date_trunc('month',CURRENT_DATE)::date,'CAD',80,95)`, string(otherUserID), otherCategoryID); err != nil {
		t.Fatalf("create other budget: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO bills(user_id,name,amount_type,expected_amount_minor,currency,frequency,next_due_at,source) VALUES($1,'Synthetic rent','FIXED',12500,'CAD','MONTHLY',CURRENT_DATE + 1,'MANUAL'),($2,'Foreign bill','FIXED',9900,'CAD','MONTHLY',CURRENT_DATE + 1,'MANUAL')`, string(ownerID), string(otherUserID)); err != nil {
		t.Fatalf("create dashboard bills: %v", err)
	}

	dashboard, err := New(db).Get(ctx, ownerID)
	if err != nil {
		t.Fatalf("get owner dashboard: %v", err)
	}
	if len(dashboard.MonthlyPlans) != 1 || dashboard.MonthlyPlans[0].PlannedMinor != 100000 || dashboard.MonthlyPlans[0].SpentMinor != 40000 {
		t.Fatalf("monthly plans = %#v", dashboard.MonthlyPlans)
	}
	if len(dashboard.Budgets) != 2 {
		t.Fatalf("owner budgets count = %d, want 2", len(dashboard.Budgets))
	}
	if len(dashboard.Upcoming) != 1 || dashboard.Upcoming[0].Name != "Synthetic rent" || dashboard.Upcoming[0].AmountMinor != 12500 {
		t.Fatalf("owner upcoming = %#v", dashboard.Upcoming)
	}
}
