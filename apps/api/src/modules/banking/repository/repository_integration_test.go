package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDisconnectTombstonesOnlyOwnedConnection(t *testing.T) {
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
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id,open_inbox_count) VALUES($1,1) RETURNING id::text`, "disconnect-owner-"+suffix).Scan(&ownerID); err != nil {
		t.Fatalf("create connection owner: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "disconnect-other-"+suffix).Scan(&otherUserID); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id = ANY($1::uuid[])`, []string{string(ownerID), string(otherUserID)})
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{string(ownerID), string(otherUserID)})
	})

	var connectionID shared.BankConnectionID
	if err := db.QueryRow(ctx, `
		INSERT INTO bank_connections(user_id,provider,provider_item_id,institution_name,access_token_ciphertext,status)
		VALUES($1,'PLAID',$2,'Test bank',$3,'READY') RETURNING id::text
	`, string(ownerID), "disconnect-item-"+suffix, []byte("ciphertext")).Scan(&connectionID); err != nil {
		t.Fatalf("create bank connection: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sync_cursors(bank_connection_id,cursor) VALUES($1,'cursor')`, string(connectionID)); err != nil {
		t.Fatalf("create sync cursor: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key) VALUES($1,'BANK_CONNECTION',$2,'BANK_CONNECTION_SYNC_REQUESTED',$3)`, string(ownerID), string(connectionID), "disconnect-event-"+suffix); err != nil {
		t.Fatalf("create sync event: %v", err)
	}
	var accountID string
	if err := db.QueryRow(ctx, `INSERT INTO accounts(user_id,bank_connection_id,provider,provider_account_id,name,account_type,balance_minor,currency) VALUES($1,$2,'PLAID',$3,'Test chequing','depository',10000,'CAD') RETURNING id::text`, string(ownerID), string(connectionID), "disconnect-account-"+suffix).Scan(&accountID); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO subscriptions(user_id,merchant_name,expected_amount_minor,currency,frequency,next_expected_at,payment_account_id) VALUES($1,'Test subscription',1000,'CAD','MONTHLY',CURRENT_DATE,$2)`, string(ownerID), accountID); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO financial_projections(user_id,currency,total_minor,available_minor,protected_minor,projected_month_end_minor,status) VALUES($1,'CAD',10000,10000,0,10000,'ON_TRACK')`, string(ownerID)); err != nil {
		t.Fatalf("create projection: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO inbox_items(user_id,item_type,priority,entity_type,entity_id) VALUES($1,'BANK_CONNECTION_ERROR','HIGH','BANK_CONNECTION',$2)`, string(ownerID), string(connectionID)); err != nil {
		t.Fatalf("create connection inbox item: %v", err)
	}

	repository := New(db)
	if err := repository.Disconnect(ctx, otherUserID, connectionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign disconnect error = %v", err)
	}
	if err := repository.Disconnect(ctx, ownerID, connectionID); err != nil {
		t.Fatalf("owned disconnect error = %v", err)
	}

	var status string
	var token []byte
	var disconnectedAt *time.Time
	if err := db.QueryRow(ctx, `SELECT status,access_token_ciphertext,disconnected_at FROM bank_connections WHERE id=$1`, string(connectionID)).Scan(&status, &token, &disconnectedAt); err != nil {
		t.Fatalf("load disconnected connection: %v", err)
	}
	if status != "DISCONNECTED" || token != nil || disconnectedAt == nil {
		t.Fatalf("disconnected connection = status %q, token present %t, disconnected_at present %t", status, token != nil, disconnectedAt != nil)
	}
	connections, err := repository.ListConnectionStatus(ctx, ownerID)
	if err != nil || len(connections) != 0 {
		t.Fatalf("active connections after disconnect = %#v, %v", connections, err)
	}
	accounts, err := repository.ListAccounts(ctx, ownerID)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("active accounts after disconnect = %#v, %v", accounts, err)
	}
	if _, err := repository.LoadOwnedConnection(ctx, ownerID, connectionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load disconnected token error = %v", err)
	}

	var cursorCount, eventCount, projectionCount, openInboxCount, auditCount int
	var paymentAccountID *string
	var inboxStatus string
	if err := db.QueryRow(ctx, `SELECT count(*) FROM sync_cursors WHERE bank_connection_id=$1`, string(connectionID)).Scan(&cursorCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE aggregate_type='BANK_CONNECTION' AND aggregate_id=$1`, string(connectionID)).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM financial_projections WHERE user_id=$1`, string(ownerID)).Scan(&projectionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT payment_account_id::text FROM subscriptions WHERE user_id=$1`, string(ownerID)).Scan(&paymentAccountID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT status FROM inbox_items WHERE user_id=$1 AND entity_id=$2`, string(ownerID), string(connectionID)).Scan(&inboxStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT open_inbox_count FROM users WHERE id=$1`, string(ownerID)).Scan(&openInboxCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_user_id=$1 AND event_type='BANK_DISCONNECTED' AND entity_id=$2`, string(ownerID), string(connectionID)).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if cursorCount != 0 || eventCount != 0 || projectionCount != 0 || paymentAccountID != nil || inboxStatus != "RESOLVED" || openInboxCount != 0 || auditCount != 1 {
		t.Fatalf("disconnect cleanup = cursor %d, event %d, projection %d, payment account present %t, inbox %q/%d, audit %d", cursorCount, eventCount, projectionCount, paymentAccountID != nil, inboxStatus, openInboxCount, auditCount)
	}
}
