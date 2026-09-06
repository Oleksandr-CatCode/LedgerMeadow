package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	shared "ledgermeadow/src/shared/types"
)

func TestDetailScopesSubscriptionAndLinkedPaymentsToUser(t *testing.T) {
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
		if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id,timezone) VALUES($1,'UTC') RETURNING id::text`, fmt.Sprintf("subscription-detail-user-%s-%d", suffix, index)).Scan(&userIDs[index]); err != nil {
			t.Fatalf("create user %d: %v", index, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=ANY($1::uuid[])`, []string{string(userIDs[0]), string(userIDs[1])})
	})

	accountIDs := make([]string, 2)
	for index, userID := range userIDs {
		var connectionID string
		if err := db.QueryRow(ctx, `INSERT INTO bank_connections(user_id,provider,provider_item_id,access_token_ciphertext,status) VALUES($1,'FILE_IMPORT',$2,NULL,'READY') RETURNING id::text`, string(userID), fmt.Sprintf("subscription-detail-connection-%s-%d", suffix, index)).Scan(&connectionID); err != nil {
			t.Fatalf("create connection %d: %v", index, err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO accounts(user_id,bank_connection_id,provider,provider_account_id,name,account_type,balance_minor,currency) VALUES($1,$2,'FILE_IMPORT',$3,'RBC Visa','credit',0,'CAD') RETURNING id::text`, string(userID), connectionID, fmt.Sprintf("subscription-detail-account-%s-%d", suffix, index)).Scan(&accountIDs[index]); err != nil {
			t.Fatalf("create account %d: %v", index, err)
		}
	}

	detectionKey := "subscription:synthetic-service:cad:" + suffix
	transactionDate := time.Now().UTC().Format("2006-01-02")
	for index, userID := range userIDs {
		var connectionID string
		if err := db.QueryRow(ctx, `SELECT bank_connection_id::text FROM accounts WHERE id=$1 AND user_id=$2`, accountIDs[index], string(userID)).Scan(&connectionID); err != nil {
			t.Fatalf("load connection %d: %v", index, err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO transactions(user_id,bank_connection_id,account_id,provider,provider_transaction_id,name,amount_minor,currency,transaction_date,is_pending,recurring_detection_key) VALUES($1,$2,$3,'FILE_IMPORT',$4,'SYNTHETIC SERVICE',-123,'CAD',$5,false,$6)`, string(userID), connectionID, accountIDs[index], fmt.Sprintf("subscription-detail-transaction-%s-%d", suffix, index), transactionDate, detectionKey); err != nil {
			t.Fatalf("create transaction %d: %v", index, err)
		}
	}
	var subscriptionID string
	if err := db.QueryRow(ctx, `INSERT INTO subscriptions(user_id,merchant_name,expected_amount_minor,currency,frequency,next_expected_at,payment_account_id,status,source,detection_key) VALUES($1,'Synthetic Subscription Service',123,'CAD','MONTHLY',$2,$3,'ACTIVE','DETECTED',$4) RETURNING id::text`, string(userIDs[0]), transactionDate, accountIDs[0], detectionKey).Scan(&subscriptionID); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	repository := New(db)
	detail, err := repository.Detail(ctx, userIDs[0], subscriptionID)
	if err != nil {
		t.Fatalf("load owned subscription: %v", err)
	}
	if len(detail.Payments) != 1 || detail.Payments[0].AmountMinor != -123 {
		t.Fatalf("owned linked payments = %#v", detail.Payments)
	}
	if _, err := repository.Detail(ctx, userIDs[1], subscriptionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign subscription detail error = %v, want ErrNotFound", err)
	}
}
