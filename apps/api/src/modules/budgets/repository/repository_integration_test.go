package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ledgermeadow/src/modules/budgets/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateInitializesCurrentPeriodSpending(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(db.Close)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID shared.UserID
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "budget-test-"+suffix).Scan(&userID); err != nil {
		t.Fatalf("create budget test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id=$1`, string(userID))
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, string(userID))
	})
	var connectionID, accountID, categoryID string
	if err := db.QueryRow(ctx, `INSERT INTO bank_connections(user_id,provider,provider_item_id,access_token_ciphertext) VALUES($1,'PLAID',$2,$3) RETURNING id::text`, string(userID), "budget-item-"+suffix, []byte("ciphertext")).Scan(&connectionID); err != nil {
		t.Fatalf("create budget test connection: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO accounts(user_id,bank_connection_id,provider,provider_account_id,name,account_type,balance_minor,currency) VALUES($1,$2,'PLAID',$3,'Budget account','depository',0,'CAD') RETURNING id::text`, string(userID), connectionID, "budget-account-"+suffix).Scan(&accountID); err != nil {
		t.Fatalf("create budget test account: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO categories(user_id,name,category_type) VALUES($1,'Groceries','EXPENSE') RETURNING id::text`, string(userID)).Scan(&categoryID); err != nil {
		t.Fatalf("create budget test category: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO transactions(user_id,bank_connection_id,account_id,provider,provider_transaction_id,name,amount_minor,currency,transaction_date,is_pending,category_id) VALUES($1,$2,$3,'PLAID',$4,'Current expense',-325,'CAD',CURRENT_DATE,false,$5),($1,$2,$3,'PLAID',$6,'Removed expense',-900,'CAD',CURRENT_DATE,false,$5),($1,$2,$3,'PLAID',$7,'Previous expense',-700,'CAD',(date_trunc('month',CURRENT_DATE)-INTERVAL '1 day')::date,false,$5)`, string(userID), connectionID, accountID, "budget-current-"+suffix, categoryID, "budget-removed-"+suffix, "budget-old-"+suffix); err != nil {
		t.Fatalf("create budget test transactions: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE transactions SET removed_at=now() WHERE provider_transaction_id=$1`, "budget-removed-"+suffix); err != nil {
		t.Fatalf("remove budget test transaction: %v", err)
	}

	repository := New(db)
	if _, err := repository.Create(ctx, userID, models.Create{Name: "Monthly groceries", CategoryID: &categoryID, Period: "MONTHLY", LimitMinor: 10000, Currency: "CAD", WarningThreshold: 75, CriticalThreshold: 90}); err != nil {
		t.Fatalf("create initialized budget: %v", err)
	}
	items, err := repository.List(ctx, userID)
	if err != nil {
		t.Fatalf("list initialized budget: %v", err)
	}
	if len(items) != 1 || int64(items[0].SpentMinor) != 325 {
		t.Fatalf("initialized spent_minor = %#v, want 325", items)
	}
}
