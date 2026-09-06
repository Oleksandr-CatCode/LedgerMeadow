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

func TestLoadClassifiesDetectedRecurringCashFlow(t *testing.T) {
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
	var userID shared.UserID
	if err := db.QueryRow(ctx, `
		INSERT INTO users(clerk_user_id, timezone) VALUES($1, 'UTC') RETURNING id::text
	`, "materialization-user-"+suffix).Scan(&userID); err != nil {
		t.Fatalf("create materialization user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, string(userID))
	})

	var connectionID, accountID string
	if err := db.QueryRow(ctx, `
		INSERT INTO bank_connections(
			user_id, provider, provider_item_id, access_token_ciphertext, status
		) VALUES($1, 'FILE_IMPORT', $2, NULL, 'READY') RETURNING id::text
	`, string(userID), "materialization-connection-"+suffix).Scan(&connectionID); err != nil {
		t.Fatalf("create materialization connection: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO accounts(
			user_id, bank_connection_id, provider, provider_account_id, name,
			account_type, balance_minor, currency
		) VALUES($1, $2, 'FILE_IMPORT', $3, 'Cash account', 'depository', 500000, 'CAD')
		RETURNING id::text
	`, string(userID), connectionID, "materialization-account-"+suffix).Scan(&accountID); err != nil {
		t.Fatalf("create materialization account: %v", err)
	}

	type transactionFixture struct {
		name          string
		amount        int64
		recurrenceKey *string
	}
	billKey := "bill:rent:cad"
	subscriptionKey := "subscription:streaming:cad"
	fixtures := []transactionFixture{
		{name: "PAYROLL", amount: 100000},
		{name: "RENT", amount: -120000, recurrenceKey: &billKey},
		{name: "STREAMING", amount: -1999, recurrenceKey: &subscriptionKey},
		{name: "GROCERIES", amount: -500},
	}
	transactionIDs := make([]string, 0, len(fixtures))
	for index, fixture := range fixtures {
		var transactionID string
		if err := db.QueryRow(ctx, `
			INSERT INTO transactions(
				user_id, bank_connection_id, account_id, provider,
				provider_transaction_id, name, amount_minor, currency,
				transaction_date, is_pending, recurring_detection_key
			) VALUES($1, $2, $3, 'FILE_IMPORT', $4, $5, $6, 'CAD', '2026-08-10', false, $7)
			RETURNING id::text
		`, string(userID), connectionID, accountID,
			fmt.Sprintf("materialization-transaction-%s-%d", suffix, index),
			fixture.name, fixture.amount, fixture.recurrenceKey,
		).Scan(&transactionID); err != nil {
			t.Fatalf("create materialization transaction %d: %v", index, err)
		}
		transactionIDs = append(transactionIDs, transactionID)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO bills(
			user_id, name, amount_type, expected_amount_minor, currency, frequency,
			next_due_at, source, status, detection_key, confidence_basis_points,
			occurrence_count, detected_from_transaction_id
		) VALUES($1, 'RENT', 'VARIABLE', 120000, 'CAD', 'MONTHLY', '2026-09-10',
		         'DETECTED', 'ACTIVE', $2, 9500, 3, $3)
	`, string(userID), billKey, transactionIDs[1]); err != nil {
		t.Fatalf("create detected bill: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO subscriptions(
			user_id, merchant_name, expected_amount_minor, currency, frequency,
			next_expected_at, payment_account_id, status, detected_from_transaction_id,
			source, detection_key, confidence_basis_points, occurrence_count
		) VALUES($1, 'STREAMING', 1999, 'CAD', 'MONTHLY', '2026-09-10', $2,
		         'ACTIVE', $3, 'DETECTED', $4, 9500, 3)
	`, string(userID), accountID, transactionIDs[2], subscriptionKey); err != nil {
		t.Fatalf("create detected subscription: %v", err)
	}

	input, err := New(db).Load(ctx, userID, time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("load materialization input: %v", err)
	}
	if len(input.Currencies) != 1 {
		t.Fatalf("currency input count = %d, want 1", len(input.Currencies))
	}
	actual := input.Currencies[0].Actual
	if actual.IncomeMinor != 100000 || actual.FixedOutflowMinor != 120000 ||
		actual.SubscriptionsMinor != 1999 || actual.VariableOutflowMinor != 500 {
		t.Fatalf("actual cash flow = %#v", actual)
	}
}
