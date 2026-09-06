package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	inboxrepo "ledgermeadow/src/modules/inbox/repository"
	"ledgermeadow/src/modules/transactionanalysis/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyIsIdempotentAndProtectsUserCorrections(t *testing.T) {
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
	`, "transaction-analysis-user-"+suffix).Scan(&userID); err != nil {
		t.Fatalf("create analysis user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, string(userID))
	})

	var connectionID, accountID string
	if err := db.QueryRow(ctx, `
		INSERT INTO bank_connections(
			user_id, provider, provider_item_id, access_token_ciphertext, status
		) VALUES($1, 'FILE_IMPORT', $2, NULL, 'READY') RETURNING id::text
	`, string(userID), "analysis-connection-"+suffix).Scan(&connectionID); err != nil {
		t.Fatalf("create analysis connection: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO accounts(
			user_id, bank_connection_id, provider, provider_account_id, name,
			account_type, balance_minor, currency
		) VALUES($1, $2, 'FILE_IMPORT', $3, 'Analysis account', 'depository', 500000, 'CAD')
		RETURNING id::text
	`, string(userID), connectionID, "analysis-account-"+suffix).Scan(&accountID); err != nil {
		t.Fatalf("create analysis account: %v", err)
	}

	amounts := []int64{300000, 300000, 300000, -1999, -1999, -1999}
	names := []string{"ACME PAYROLL", "ACME PAYROLL", "ACME PAYROLL", "STREAMING SERVICE", "STREAMING SERVICE", "STREAMING SERVICE"}
	dates := []string{"2026-05-29", "2026-06-30", "2026-07-31", "2026-05-15", "2026-06-15", "2026-07-15"}
	transactionIDs := make([]string, 0, len(amounts))
	for index := range amounts {
		var transactionID string
		if err := db.QueryRow(ctx, `
			INSERT INTO transactions(
				user_id, bank_connection_id, account_id, provider,
				provider_transaction_id, name, amount_minor, currency,
				transaction_date, is_pending
			) VALUES($1, $2, $3, 'FILE_IMPORT', $4, $5, $6, 'CAD', $7, false)
			RETURNING id::text
		`, string(userID), connectionID, accountID,
			fmt.Sprintf("analysis-transaction-%s-%d", suffix, index), names[index], amounts[index], dates[index],
		).Scan(&transactionID); err != nil {
			t.Fatalf("create analysis transaction %d: %v", index, err)
		}
		transactionIDs = append(transactionIDs, transactionID)
	}
	categoryIDs := make(map[string]string, 5)
	rows, err := db.Query(ctx, `
		SELECT system_key,id::text FROM categories
		WHERE system_key=ANY($1::text[])
	`, []string{"income.salary", "subscriptions", "other", "groceries", "dining"})
	if err != nil {
		t.Fatalf("load analysis categories: %v", err)
	}
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			rows.Close()
			t.Fatalf("scan analysis category: %v", err)
		}
		categoryIDs[key] = id
	}
	rows.Close()

	result := models.Result{
		Categories: []models.CategoryAssignment{
			{TransactionID: transactionIDs[0], CategoryID: categoryIDs["income.salary"], ConfidenceBasisPoints: 9800},
			{TransactionID: transactionIDs[3], CategoryID: categoryIDs["subscriptions"], ConfidenceBasisPoints: 9300},
		},
		CategoryReviews: []models.CategoryReview{{
			TransactionID: transactionIDs[4], SuggestedCategoryID: categoryIDs["other"],
			ConfidenceBasisPoints: 5000, Explanation: "No reliable category-specific rule matched.",
		}},
		Recurring: []models.RecurringCandidate{
			{
				DetectionKey: "income:acme-payroll:cad", Name: "ACME PAYROLL", Kind: "INCOME",
				Currency: "CAD", Frequency: "MONTHLY", ExpectedAmountMinor: 300000,
				NextExpectedAt: "2026-08-31", ConfidenceBasisPoints: 9500, OccurrenceCount: 3,
				AccountID: accountID, SupportingIDs: transactionIDs[:3],
				Explanation: "Three consistent monthly deposits", Status: "ACTIVE",
			},
			{
				DetectionKey: "subscription:streaming-service:cad", Name: "STREAMING SERVICE", Kind: "SUBSCRIPTION",
				Currency: "CAD", Frequency: "MONTHLY", ExpectedAmountMinor: 1999,
				NextExpectedAt: "2026-08-15", ConfidenceBasisPoints: 7600, OccurrenceCount: 3,
				AccountID: accountID, CategoryID: categoryIDs["subscriptions"], SupportingIDs: transactionIDs[3:],
				Explanation: "Three consistent monthly charges", Status: "UNKNOWN",
			},
		},
	}
	repository := New(db)
	if err := repository.Apply(ctx, userID, result); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := repository.Apply(ctx, userID, result); err != nil {
		t.Fatalf("second apply: %v", err)
	}

	var incomeCount, subscriptionCount, inboxCount, linkedCount, notificationCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM recurring_income_sources WHERE user_id=$1`, string(userID)).Scan(&incomeCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE user_id=$1`, string(userID)).Scan(&subscriptionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM inbox_items WHERE user_id=$1 AND status='OPEN'`, string(userID)).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE user_id=$1 AND recurring_detection_key IS NOT NULL`, string(userID)).Scan(&linkedCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id=$1`, string(userID)).Scan(&notificationCount); err != nil {
		t.Fatal(err)
	}
	if incomeCount != 1 || subscriptionCount != 1 || inboxCount != 2 || linkedCount != 6 || notificationCount != 2 {
		t.Fatalf("income/subscription/inbox/linked/notification counts = %d/%d/%d/%d/%d", incomeCount, subscriptionCount, inboxCount, linkedCount, notificationCount)
	}

	if _, err := db.Exec(ctx, `
		UPDATE transactions SET category_id=(SELECT id FROM categories WHERE system_key='groceries'),
		       category_source='USER',category_confidence_basis_points=NULL
		WHERE id=$1 AND user_id=$2
	`, transactionIDs[3], string(userID)); err != nil {
		t.Fatalf("set user category correction: %v", err)
	}
	result.Categories[1].CategoryID = categoryIDs["dining"]
	if err := repository.Apply(ctx, userID, result); err != nil {
		t.Fatalf("apply after category correction: %v", err)
	}
	var categoryKey, categorySource string
	if err := db.QueryRow(ctx, `
		SELECT category.system_key, transaction.category_source
		FROM transactions transaction JOIN categories category ON category.id=transaction.category_id
		WHERE transaction.id=$1
	`, transactionIDs[3]).Scan(&categoryKey, &categorySource); err != nil {
		t.Fatal(err)
	}
	if categoryKey != "groceries" || categorySource != "USER" {
		t.Fatalf("user category correction was overwritten: %s/%s", categoryKey, categorySource)
	}
	if _, err := db.Exec(ctx, `
		UPDATE subscriptions SET status='ACTIVE',user_modified_at=now()
		WHERE user_id=$1 AND detection_key='subscription:streaming-service:cad'
	`, string(userID)); err != nil {
		t.Fatalf("confirm recurring subscription: %v", err)
	}

	observationIDs := make([]string, 0, 2)
	for index, amount := range []int64{-1200, -287} {
		var transactionID string
		if err := db.QueryRow(ctx, `
			INSERT INTO transactions(
				user_id,bank_connection_id,account_id,provider,provider_transaction_id,
				name,amount_minor,currency,transaction_date,is_pending
			) VALUES($1,$2,$3,'FILE_IMPORT',$4,'STREAMING SERVICE',$5,'CAD','2026-08-15',false)
			RETURNING id::text
		`, string(userID), connectionID, accountID,
			fmt.Sprintf("analysis-amount-observation-%s-%d", suffix, index), amount,
		).Scan(&transactionID); err != nil {
			t.Fatalf("create amount observation %d: %v", index, err)
		}
		observationIDs = append(observationIDs, transactionID)
	}
	observedAmount := int64(1487)
	result.Recurring[1].ObservedAmountMinor = &observedAmount
	result.Recurring[1].AmountObservationIDs = observationIDs
	result.Recurring[1].NextExpectedAt = "2026-09-15"
	if err := repository.Apply(ctx, userID, result); err != nil {
		t.Fatalf("apply amount observation: %v", err)
	}
	var priceChangeID string
	if err := db.QueryRow(ctx, `
		SELECT id::text FROM inbox_items
		WHERE user_id=$1 AND item_type='PRICE_CHANGE' AND status='OPEN'
	`, string(userID)).Scan(&priceChangeID); err != nil {
		t.Fatalf("load price change review: %v", err)
	}
	var storedAmount int64
	if err := db.QueryRow(ctx, `
		SELECT expected_amount_minor FROM subscriptions
		WHERE user_id=$1 AND detection_key='subscription:streaming-service:cad'
	`, string(userID)).Scan(&storedAmount); err != nil {
		t.Fatal(err)
	}
	if storedAmount != 1999 {
		t.Fatalf("amount changed before confirmation: %d", storedAmount)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM transactions
		WHERE user_id=$1 AND id=ANY($2::uuid[]) AND recurring_detection_key IS NOT NULL
	`, string(userID), observationIDs).Scan(&linkedCount); err != nil {
		t.Fatal(err)
	}
	if linkedCount != 0 {
		t.Fatalf("amount observations linked before confirmation: %d", linkedCount)
	}
	inboxRepository := inboxrepo.New(db)
	if err := inboxRepository.Resolve(ctx, shared.UserID("00000000-0000-4000-8000-000000000099"), priceChangeID, "CONFIRMED"); !errors.Is(err, inboxrepo.ErrNotFound) {
		t.Fatalf("cross-tenant price change resolution error = %v", err)
	}
	if err := inboxRepository.Resolve(ctx, userID, priceChangeID, "CONFIRMED"); err != nil {
		t.Fatalf("confirm price change: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT expected_amount_minor FROM subscriptions
		WHERE user_id=$1 AND detection_key='subscription:streaming-service:cad'
	`, string(userID)).Scan(&storedAmount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM transactions
		WHERE user_id=$1 AND id=ANY($2::uuid[])
		  AND recurring_detection_key='subscription:streaming-service:cad'
	`, string(userID), observationIDs).Scan(&linkedCount); err != nil {
		t.Fatal(err)
	}
	if storedAmount != observedAmount || linkedCount != len(observationIDs) {
		t.Fatalf("confirmed amount/linked observations = %d/%d", storedAmount, linkedCount)
	}

	if _, err := db.Exec(ctx, `
		UPDATE subscriptions SET status='CANCELLED',user_modified_at=now()
		WHERE user_id=$1 AND detection_key='subscription:streaming-service:cad'
	`, string(userID)); err != nil {
		t.Fatalf("record recurring rejection: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE inbox_items SET status='DISMISSED',resolved_at=now() WHERE user_id=$1 AND status='OPEN'`, string(userID)); err != nil {
		t.Fatalf("dismiss recurring review: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE transactions SET recurring_detection_key=NULL
		WHERE user_id=$1 AND recurring_detection_key='subscription:streaming-service:cad'
	`, string(userID)); err != nil {
		t.Fatalf("clear recurring links: %v", err)
	}
	if err := repository.Apply(ctx, userID, result); err != nil {
		t.Fatalf("apply after recurring rejection: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM inbox_items
		WHERE user_id=$1 AND status='OPEN' AND item_type='POSSIBLE_SUBSCRIPTION'
	`, string(userID)).Scan(&inboxCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM transactions
		WHERE user_id=$1 AND recurring_detection_key='subscription:streaming-service:cad'
	`, string(userID)).Scan(&linkedCount); err != nil {
		t.Fatal(err)
	}
	if inboxCount != 0 || linkedCount != 0 {
		t.Fatalf("rejected recurrence was restored: inbox/linked counts = %d/%d", inboxCount, linkedCount)
	}
}
