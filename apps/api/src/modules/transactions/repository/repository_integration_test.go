package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	bankingrepo "ledgermeadow/src/modules/banking/repository"
	banking "ledgermeadow/src/modules/banking/types"
	transactions "ledgermeadow/src/modules/transactions/types"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryIsolationAndPendingReconciliation(t *testing.T) {
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
	userA := insertUser(t, db, "clerk-a-"+suffix)
	userB := insertUser(t, db, "clerk-b-"+suffix)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id = ANY($1::uuid[])`, []string{string(userA), string(userB)})
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{string(userA), string(userB)})
	})
	connectionA, accountA := insertConnectionAndAccount(t, db, userA, "item-a-"+suffix, "account-a-"+suffix)
	connectionB, accountB := insertConnectionAndAccount(t, db, userB, "item-b-"+suffix, "account-b-"+suffix)
	repository := New(db)
	bankRepository := bankingrepo.New(db)

	if _, err := db.Exec(ctx, `
		INSERT INTO accounts (
			user_id, bank_connection_id, provider, provider_account_id,
			name, account_type, balance_minor, currency
		) VALUES ($1, $2, 'PLAID', $3, 'Cross-user account', 'depository', 100, 'CAD')
	`, string(userA), string(connectionB), "cross-user-"+suffix); err == nil {
		t.Fatal("database accepted an account owned by a different user than its bank connection")
	}

	accountsA, err := bankRepository.ListAccounts(ctx, userA)
	if err != nil {
		t.Fatalf("list user A accounts: %v", err)
	}
	if len(accountsA) != 1 || accountsA[0].ID != accountA {
		t.Fatalf("user A account isolation failed: %#v", accountsA)
	}

	webhookDedupeKey := "webhook-test-" + suffix
	if err := bankRepository.EnqueueWebhookSync(ctx, connectionA, userA, webhookDedupeKey); err != nil {
		t.Fatalf("enqueue first webhook: %v", err)
	}
	if err := bankRepository.EnqueueWebhookSync(ctx, connectionA, userA, webhookDedupeKey); err != nil {
		t.Fatalf("enqueue duplicate webhook: %v", err)
	}
	var webhookEvents int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE dedupe_key = $1`, webhookDedupeKey).Scan(&webhookEvents); err != nil {
		t.Fatalf("count webhook events: %v", err)
	}
	if webhookEvents != 1 {
		t.Fatalf("duplicate webhook created %d events, want 1", webhookEvents)
	}

	pendingProviderID := "pending-" + suffix
	if err := repository.ApplySyncPage(ctx, banking.SyncConnection{ID: connectionA, UserID: userA}, []transactions.NormalizedTransaction{{
		ProviderTransactionID: pendingProviderID,
		AccountID:             accountA, Name: "Pending coffee", AmountMinor: -450,
		Currency: shared.CurrencyCAD, Date: time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC), IsPending: true,
	}}, nil, "cursor-a-1"); err != nil {
		t.Fatalf("store pending transaction: %v", err)
	}
	postedProviderID := "posted-" + suffix
	if err := repository.ApplySyncPage(ctx, banking.SyncConnection{ID: connectionA, UserID: userA}, []transactions.NormalizedTransaction{{
		ProviderTransactionID: postedProviderID, PendingProviderTransactionID: &pendingProviderID,
		AccountID: accountA, Name: "Posted coffee", AmountMinor: -500,
		Currency: shared.CurrencyCAD, Date: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC), IsPending: false,
	}}, []string{pendingProviderID}, "cursor-a-2"); err != nil {
		t.Fatalf("reconcile posted transaction: %v", err)
	}
	if err := repository.ApplySyncPage(ctx, banking.SyncConnection{ID: connectionB, UserID: userB}, []transactions.NormalizedTransaction{{
		ProviderTransactionID: "user-b-" + suffix, AccountID: accountB, Name: "Private B",
		AmountMinor: -100, Currency: shared.CurrencyCAD,
		Date: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC), IsPending: false,
	}}, nil, "cursor-b-1"); err != nil {
		t.Fatalf("store user B transaction: %v", err)
	}

	page, err := repository.List(ctx, userA, "", 25, transactions.ListFilters{})
	if err != nil {
		t.Fatalf("list user A transactions: %v", err)
	}
	if len(page.Transactions) != 1 {
		t.Fatalf("user A received %d transactions, want 1", len(page.Transactions))
	}
	if page.Transactions[0].Name != "Posted coffee" || page.Transactions[0].AmountMinor != -500 || page.Transactions[0].IsPending {
		t.Fatalf("pending transaction was not reconciled: %#v", page.Transactions[0])
	}
	var canonicalCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM transactions
		WHERE user_id = $1 AND (provider_transaction_id = $2 OR pending_provider_transaction_id = $2)
	`, string(userA), pendingProviderID).Scan(&canonicalCount); err != nil {
		t.Fatalf("count canonical transactions: %v", err)
	}
	if canonicalCount != 1 {
		t.Fatalf("pending-to-posted reconciliation created %d rows, want 1", canonicalCount)
	}
}

func TestCategoryUpdateResolvesUncertainCategoryReviewAtomically(t *testing.T) {
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
	userID := insertUser(t, db, "category-review-"+suffix)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id=$1`, string(userID))
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, string(userID))
	})
	connectionID, accountID := insertConnectionAndAccount(
		t, db, userID, "category-review-item-"+suffix, "category-review-account-"+suffix,
	)
	repository := New(db)
	if err := repository.ApplySyncPage(ctx, banking.SyncConnection{ID: connectionID, UserID: userID}, []transactions.NormalizedTransaction{{
		ProviderTransactionID: "category-review-transaction-" + suffix,
		AccountID:             accountID, Name: "Unknown merchant", AmountMinor: -1500,
		Currency: shared.CurrencyCAD, Date: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
	}}, nil, "category-review-cursor"); err != nil {
		t.Fatalf("store category-review transaction: %v", err)
	}
	page, err := repository.List(ctx, userID, "", 1, transactions.ListFilters{})
	if err != nil || len(page.Transactions) != 1 {
		t.Fatalf("load category-review transaction: %v, %#v", err, page.Transactions)
	}
	transactionID := page.Transactions[0].ID
	learningKey := strings.Repeat("a", 64)
	if _, err := db.Exec(ctx, `
		UPDATE transactions SET categorization_learning_key=$3
		WHERE id=$1 AND user_id=$2
	`, string(transactionID), string(userID), learningKey); err != nil {
		t.Fatalf("store category-review learning key: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO inbox_items(user_id,item_type,priority,entity_type,entity_id,payload)
		VALUES($1,'TRANSACTION_REVIEW','NORMAL','TRANSACTION',$2,'{"reason":"CATEGORY_UNCERTAIN"}'::jsonb)
	`, string(userID), string(transactionID)); err != nil {
		t.Fatalf("create category-review Inbox item: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE users SET open_inbox_count=1 WHERE id=$1`, string(userID)); err != nil {
		t.Fatalf("update category-review Inbox count: %v", err)
	}
	var categoryID string
	if err := db.QueryRow(ctx, `SELECT id::text FROM categories WHERE system_key='groceries'`).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	reviewed := "REVIEWED"
	if err := repository.Update(ctx, userID, transactionID, transactions.Update{
		CategoryID: &categoryID, ReviewStatus: &reviewed,
	}); err != nil {
		t.Fatalf("assign reviewed category: %v", err)
	}

	var categoryKey, categorySource, reviewStatus, inboxStatus string
	var openInboxCount int
	if err := db.QueryRow(ctx, `
		SELECT category.system_key,transaction.category_source,transaction.review_status
		FROM transactions transaction JOIN categories category ON category.id=transaction.category_id
		WHERE transaction.id=$1
	`, string(transactionID)).Scan(&categoryKey, &categorySource, &reviewStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `
		SELECT status FROM inbox_items WHERE user_id=$1 AND entity_id=$2
	`, string(userID), string(transactionID)).Scan(&inboxStatus); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT open_inbox_count FROM users WHERE id=$1`, string(userID)).Scan(&openInboxCount); err != nil {
		t.Fatal(err)
	}
	if categoryKey != "groceries" || categorySource != "USER" || reviewStatus != "REVIEWED" ||
		inboxStatus != "RESOLVED" || openInboxCount != 0 {
		t.Fatalf(
			"category review state = %s/%s/%s inbox=%s/%d",
			categoryKey, categorySource, reviewStatus, inboxStatus, openInboxCount,
		)
	}
	var preferenceCount, globalUserCount int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM category_pattern_preferences
		WHERE user_id=$1 AND learning_key=$2 AND category_id=$3
	`, string(userID), learningKey, categoryID).Scan(&preferenceCount); err != nil {
		t.Fatalf("load learned category preference: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT distinct_user_count FROM categorization_global_signal_counts
		WHERE learning_key=$1 AND category_id=$2
	`, learningKey, categoryID).Scan(&globalUserCount); err != nil {
		t.Fatalf("load global category signal count: %v", err)
	}
	if preferenceCount != 1 || globalUserCount != 1 {
		t.Fatalf("category learning preference/global count = %d/%d", preferenceCount, globalUserCount)
	}
	emptyCategory := ""
	if err := repository.Update(ctx, userID, transactionID, transactions.Update{CategoryID: &emptyCategory}); err != nil {
		t.Fatalf("clear learned category: %v", err)
	}
	var clearedCategory *string
	if err := db.QueryRow(ctx, `
		SELECT category_id::text,category_source,review_status
		FROM transactions WHERE id=$1 AND user_id=$2
	`, string(transactionID), string(userID)).Scan(&clearedCategory, &categorySource, &reviewStatus); err != nil {
		t.Fatalf("load cleared category state: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM category_pattern_preferences
		WHERE user_id=$1 AND learning_key=$2
	`, string(userID), learningKey).Scan(&preferenceCount); err != nil {
		t.Fatalf("count cleared category preference: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM categorization_global_signal_counts
		WHERE learning_key=$1 AND category_id=$2
	`, learningKey, categoryID).Scan(&globalUserCount); err != nil {
		t.Fatalf("count cleared global category signal: %v", err)
	}
	if clearedCategory != nil || categorySource != "UNASSIGNED" || reviewStatus != "NEEDS_REVIEW" ||
		preferenceCount != 0 || globalUserCount != 0 {
		t.Fatalf("cleared category state = %v/%s/%s preference/global=%d/%d", clearedCategory, categorySource, reviewStatus, preferenceCount, globalUserCount)
	}
}

func TestFilteredListAndBulkUpdateAreTenantSafeAndAtomic(t *testing.T) {
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
	userA := insertUser(t, db, "bulk-a-"+suffix)
	userB := insertUser(t, db, "bulk-b-"+suffix)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id = ANY($1::uuid[])`, []string{string(userA), string(userB)})
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1::uuid[])`, []string{string(userA), string(userB)})
	})
	connectionA, accountA := insertConnectionAndAccount(t, db, userA, "bulk-item-a-"+suffix, "bulk-account-a-"+suffix)
	connectionB, accountB := insertConnectionAndAccount(t, db, userB, "bulk-item-b-"+suffix, "bulk-account-b-"+suffix)
	repository := New(db)
	today := time.Now().UTC()
	if err := repository.ApplySyncPage(ctx, banking.SyncConnection{ID: connectionA, UserID: userA}, []transactions.NormalizedTransaction{
		{ProviderTransactionID: "bulk-a1-" + suffix, AccountID: accountA, Name: "Bulk %alpha", AmountMinor: -100, Currency: shared.CurrencyCAD, Date: today},
		{ProviderTransactionID: "bulk-a2-" + suffix, AccountID: accountA, Name: "Bulk beta", MerchantName: stringPointer("Visible %merchant"), AmountMinor: -200, Currency: shared.CurrencyCAD, Date: today},
	}, nil, "bulk-cursor-a"); err != nil {
		t.Fatalf("store owned bulk transactions: %v", err)
	}
	if err := repository.ApplySyncPage(ctx, banking.SyncConnection{ID: connectionB, UserID: userB}, []transactions.NormalizedTransaction{
		{ProviderTransactionID: "bulk-b1-" + suffix, AccountID: accountB, Name: "Bulk private", MerchantName: stringPointer("Visible %private"), AmountMinor: -300, Currency: shared.CurrencyCAD, Date: today},
	}, nil, "bulk-cursor-b"); err != nil {
		t.Fatalf("store foreign bulk transaction: %v", err)
	}

	pageA, err := repository.List(ctx, userA, "", 10, transactions.ListFilters{})
	if err != nil || len(pageA.Transactions) != 2 {
		t.Fatalf("list owned bulk transactions: %d, %v", len(pageA.Transactions), err)
	}
	pageB, err := repository.List(ctx, userB, "", 10, transactions.ListFilters{})
	if err != nil || len(pageB.Transactions) != 1 {
		t.Fatalf("list foreign bulk transaction: %d, %v", len(pageB.Transactions), err)
	}
	reviewed := "REVIEWED"
	if err := repository.BulkUpdate(ctx, userA, transactions.BulkUpdate{
		TransactionIDs: []shared.TransactionID{pageA.Transactions[0].ID, pageB.Transactions[0].ID},
		Update:         transactions.Update{ReviewStatus: &reviewed},
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant bulk update error = %v, want ErrNotFound", err)
	}
	var reviewStatus string
	if err := db.QueryRow(ctx, `SELECT review_status FROM transactions WHERE id=$1`, string(pageA.Transactions[0].ID)).Scan(&reviewStatus); err != nil || reviewStatus != "NEEDS_REVIEW" {
		t.Fatalf("failed bulk update was not atomic: %s, %v", reviewStatus, err)
	}

	var categoryID, spaceID string
	if err := db.QueryRow(ctx, `INSERT INTO categories(user_id,name,category_type) VALUES($1,'Bulk category','EXPENSE') RETURNING id::text`, string(userA)).Scan(&categoryID); err != nil {
		t.Fatalf("create bulk category: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO spaces(user_id,name,space_type,currency,monthly_allocation_minor,balance_minor) VALUES($1,'Bulk Space','DAILY','CAD',1000,1000) RETURNING id::text`, string(userA)).Scan(&spaceID); err != nil {
		t.Fatalf("create bulk Space: %v", err)
	}
	ids := []shared.TransactionID{pageA.Transactions[0].ID, pageA.Transactions[1].ID}
	if err := repository.BulkUpdate(ctx, userA, transactions.BulkUpdate{TransactionIDs: ids, Update: transactions.Update{CategoryID: &categoryID, SpaceID: &spaceID, ReviewStatus: &reviewed}}); err != nil {
		t.Fatalf("bulk update owned transactions: %v", err)
	}

	query := "Bulk %"
	filtered, err := repository.List(ctx, userA, "", 10, transactions.ListFilters{AccountID: stringPointer(string(accountA)), CategoryID: &categoryID, SpaceID: &spaceID, ReviewStatus: &reviewed, CurrentMonth: true, Query: &query})
	if err != nil {
		t.Fatalf("run filtered transaction query: %v", err)
	}
	if len(filtered.Transactions) != 1 || filtered.Transactions[0].Name != "Bulk %alpha" {
		t.Fatalf("LIKE escaping or filters failed: %#v", filtered.Transactions)
	}
	merchantQuery := "Visible %"
	merchantFiltered, err := repository.List(ctx, userA, "", 10, transactions.ListFilters{Query: &merchantQuery})
	if err != nil {
		t.Fatalf("run displayed merchant query: %v", err)
	}
	if len(merchantFiltered.Transactions) != 1 || merchantFiltered.Transactions[0].Name != "Bulk beta" || merchantFiltered.Transactions[0].MerchantName == nil || *merchantFiltered.Transactions[0].MerchantName != "Visible %merchant" {
		t.Fatalf("displayed merchant search or tenant scope failed: %#v", merchantFiltered.Transactions)
	}
	var balance int64
	if err := db.QueryRow(ctx, `SELECT balance_minor FROM spaces WHERE id=$1`, spaceID).Scan(&balance); err != nil || balance != 700 {
		t.Fatalf("bulk Space derivative = %d, %v; want 700", balance, err)
	}
}

func stringPointer(value string) *string { return &value }

func insertUser(t *testing.T, db *pgxpool.Pool, clerkID string) shared.UserID {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), `INSERT INTO users (clerk_user_id) VALUES ($1) RETURNING id::text`, clerkID).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return shared.UserID(id)
}

func insertConnectionAndAccount(
	t *testing.T,
	db *pgxpool.Pool,
	userID shared.UserID,
	itemID string,
	providerAccountID string,
) (shared.BankConnectionID, shared.AccountID) {
	t.Helper()
	ctx := context.Background()
	var connectionID string
	if err := db.QueryRow(ctx, `
		INSERT INTO bank_connections (
			user_id, provider, provider_item_id, access_token_ciphertext
		) VALUES ($1, 'PLAID', $2, $3) RETURNING id::text
	`, string(userID), itemID, []byte("encrypted-test-token")).Scan(&connectionID); err != nil {
		t.Fatalf("insert connection: %v", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO sync_cursors (bank_connection_id) VALUES ($1)`, connectionID); err != nil {
		t.Fatalf("insert cursor: %v", err)
	}
	var accountID string
	if err := db.QueryRow(ctx, `
		INSERT INTO accounts (
			user_id, bank_connection_id, provider, provider_account_id,
			name, account_type, balance_minor, currency
		) VALUES ($1, $2, 'PLAID', $3, 'Test account', 'depository', 10000, 'CAD')
		RETURNING id::text
	`, string(userID), connectionID, providerAccountID).Scan(&accountID); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	return shared.BankConnectionID(connectionID), shared.AccountID(accountID)
}
