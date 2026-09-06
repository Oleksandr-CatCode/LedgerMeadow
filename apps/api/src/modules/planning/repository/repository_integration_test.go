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

func TestGetScopesPlanningWorkspaceToOwner(t *testing.T) {
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
		if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id,timezone) VALUES($1,'UTC') RETURNING id::text`, fmt.Sprintf("planning-user-%s-%d", suffix, index)).Scan(&userIDs[index]); err != nil {
			t.Fatalf("create user %d: %v", index, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=ANY($1::uuid[])`, []string{string(userIDs[0]), string(userIDs[1])})
	})

	for index, userID := range userIDs {
		if _, err := db.Exec(ctx, `
			INSERT INTO bills(user_id,name,amount_type,expected_amount_minor,currency,frequency,next_due_at,source)
			VALUES($1,$2,'FIXED',$3,'CAD','MONTHLY','2026-09-01','MANUAL')
		`, string(userID), fmt.Sprintf("planning-bill-%d", index), 10_000+index); err != nil {
			t.Fatalf("create bill %d: %v", index, err)
		}
	}

	workspace, err := New(db).Get(ctx, userIDs[0])
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(workspace.Bills) != 1 || workspace.Bills[0].Name != "planning-bill-0" {
		t.Fatalf("owner bills = %#v", workspace.Bills)
	}
	if workspace.AsOfDate == "" || workspace.PlanStart == "" || workspace.PlanEnd == "" {
		t.Fatalf("planning period = %q, %q, %q", workspace.AsOfDate, workspace.PlanStart, workspace.PlanEnd)
	}
}
