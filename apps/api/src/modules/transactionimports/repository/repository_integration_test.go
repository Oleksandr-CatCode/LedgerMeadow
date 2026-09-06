package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"ledgermeadow/src/modules/transactionimports/models"
	shared "ledgermeadow/src/shared/types"
)

func TestImportIsAtomicAndDuplicateSafe(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(db.Close)

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	var userID shared.UserID
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "synthetic-import-user-"+suffix).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id=$1`, string(userID))
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, string(userID))
	})

	file := models.ParsedFile{
		AccountType: models.AccountTypeVisa,
		Mask:        "0000",
		Currency:    shared.CurrencyCAD,
		DateFrom:    time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		DateTo:      time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC),
		Transactions: []models.Transaction{
			{Date: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Name: "Synthetic Credit Row", OriginalDescription: "SYNTHETIC CREDIT RECORD", AmountMinor: 101, Fingerprint: "synthetic-import-row-alpha"},
			{Date: time.Date(2000, 1, 2, 0, 0, 0, 0, time.UTC), Name: "Synthetic Debit Row", OriginalDescription: "SYNTHETIC DEBIT RECORD", AmountMinor: -2, Fingerprint: "synthetic-import-row-beta"},
		},
	}
	repository := New(db)
	if _, err := repository.Import(ctx, userID, models.ImportCommand{
		AccountName: "Synthetic Card Without Balance", ConnectionOpaqueID: "synthetic-no-balance-connection-" + suffix,
		AccountOpaqueID: "synthetic-no-balance-account-" + suffix, BatchOpaqueID: "synthetic-no-balance-batch-" + suffix,
		File: file,
	}); err != ErrCurrentBalanceRequired {
		t.Fatalf("new account without balance error = %v, want ErrCurrentBalanceRequired", err)
	}
	initialBalance := int64(500)
	first, err := repository.Import(ctx, userID, models.ImportCommand{
		AccountName: "Synthetic Card Import", CurrentBalanceMinor: &initialBalance,
		ConnectionOpaqueID: "synthetic-primary-connection-" + suffix, AccountOpaqueID: "synthetic-primary-account-" + suffix,
		BatchOpaqueID: "synthetic-first-batch-" + suffix, File: file,
	})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	secondFile := file
	secondFile.DateTo = time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC)
	secondFile.Transactions = append(append([]models.Transaction{}, file.Transactions...), models.Transaction{
		Date: time.Date(2000, 1, 3, 0, 0, 0, 0, time.UTC), Name: "Synthetic Added Debit",
		OriginalDescription: "SYNTHETIC ADDED RECORD", AmountMinor: -3, Fingerprint: "synthetic-import-row-gamma",
	})
	match, err := repository.Match(ctx, userID, file)
	if err != nil || !match.Exists || match.AccountID != first.AccountID || match.AccountName != "Synthetic Card Import" {
		t.Fatalf("matched account = %#v, error = %v", match, err)
	}
	second, err := repository.Import(ctx, userID, models.ImportCommand{
		AccountName:        "Synthetic Alternate Label",
		ConnectionOpaqueID: "synthetic-unused-connection-" + suffix, AccountOpaqueID: "synthetic-unused-account-" + suffix,
		BatchOpaqueID: "synthetic-repeat-batch-" + suffix, File: secondFile,
	})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if first.AccountID != second.AccountID || first.ConnectionID != second.ConnectionID {
		t.Fatalf("reimport created another account: first %#v, second %#v", first, second)
	}
	if first.ImportedCount != 2 || first.DuplicateCount != 0 || second.ImportedCount != 1 || second.DuplicateCount != 2 {
		t.Fatalf("unexpected import counts: first %#v, second %#v", first, second)
	}

	var provider, mask, name string
	var balance int64
	var token []byte
	if err := db.QueryRow(ctx, `
		SELECT a.provider, a.mask, a.name, a.balance_minor, bc.access_token_ciphertext
		FROM accounts a JOIN bank_connections bc ON bc.id=a.bank_connection_id
		WHERE a.id=$1
	`, string(first.AccountID)).Scan(&provider, &mask, &name, &balance, &token); err != nil {
		t.Fatalf("load imported account: %v", err)
	}
	if provider != "FILE_IMPORT" || mask != "0000" || name != "Synthetic Card Import" || balance != -503 || token != nil {
		t.Fatalf("stored account provider=%q mask=%q name=%q balance=%d has_token=%v", provider, mask, name, balance, token != nil)
	}
	var transactionCount, analysisCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE user_id=$1 AND provider='FILE_IMPORT'`, string(userID)).Scan(&transactionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE user_id=$1 AND event_type='TRANSACTION_ANALYSIS_REQUESTED'`, string(userID)).Scan(&analysisCount); err != nil {
		t.Fatal(err)
	}
	if transactionCount != 3 || analysisCount != 2 {
		t.Fatalf("transactions=%d analysis jobs=%d", transactionCount, analysisCount)
	}
}
