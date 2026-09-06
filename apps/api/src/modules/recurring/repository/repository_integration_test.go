package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"ledgermeadow/src/entities"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReclassifySubscriptionIsAtomicAndUserScoped(t *testing.T) {
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
	var ownerID, otherID shared.UserID
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "recurring-owner-"+suffix).Scan(&ownerID); err != nil {
		t.Fatalf("create recurring owner: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "recurring-other-"+suffix).Scan(&otherID); err != nil {
		t.Fatalf("create recurring other user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id=ANY($1::uuid[])`, []string{string(ownerID), string(otherID)})
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=ANY($1::uuid[])`, []string{string(ownerID), string(otherID)})
	})

	var categoryID, spaceID, subscriptionID, inboxID, notificationID string
	if err := db.QueryRow(ctx, `INSERT INTO categories(user_id,name,category_type) VALUES($1,'Synthetic insurance','EXPENSE') RETURNING id::text`, string(ownerID)).Scan(&categoryID); err != nil {
		t.Fatalf("create recurring category: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO spaces(user_id,name,space_type,currency) VALUES($1,'Synthetic bills','BILLS','CAD') RETURNING id::text`, string(ownerID)).Scan(&spaceID); err != nil {
		t.Fatalf("create recurring space: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO subscriptions(
			user_id,merchant_name,expected_amount_minor,currency,frequency,next_expected_at,
			category_id,space_id,status,source,detection_key,confidence_basis_points,
			occurrence_count
		) VALUES($1,'Synthetic insurance payment',10000,'CAD','MONTHLY','2026-09-01',
		         $2,$3,'ACTIVE','DETECTED',$4,9000,3)
		RETURNING id::text
	`, string(ownerID), categoryID, spaceID, "CAD:outflow:synthetic-insurance-"+suffix).Scan(&subscriptionID); err != nil {
		t.Fatalf("create recurring subscription: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO inbox_items(user_id,item_type,priority,entity_type,entity_id,payload)
		VALUES($1,'POSSIBLE_SUBSCRIPTION','NORMAL',$2,$3,'{}'::jsonb)
		RETURNING id::text
	`, string(ownerID), string(entities.Subscription), subscriptionID).Scan(&inboxID); err != nil {
		t.Fatalf("create recurring inbox item: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO notifications(user_id,notification_type,title,body,entity_type,entity_id)
		VALUES($1,'NEW_RECURRING','Synthetic recurring','Synthetic recurring body',$2,$3)
		RETURNING id::text
	`, string(ownerID), string(entities.Subscription), subscriptionID).Scan(&notificationID); err != nil {
		t.Fatalf("create recurring notification: %v", err)
	}

	if err := Reclassify(ctx, db, otherID, subscriptionID, entities.Subscription); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant reclassification error = %v, want ErrNotFound", err)
	}
	var originCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE user_id=$1 AND id=$2`, string(ownerID), subscriptionID).Scan(&originCount); err != nil {
		t.Fatalf("verify cross-tenant origin: %v", err)
	}
	if originCount != 1 {
		t.Fatal("cross-tenant reclassification changed the owner subscription")
	}

	if err := Reclassify(ctx, db, ownerID, subscriptionID, entities.Subscription); err != nil {
		t.Fatalf("reclassify owner subscription: %v", err)
	}
	var billID, name, amountType, currency, frequency, nextDate, source, status, detectionKey string
	var amount int64
	var confidence, occurrence int
	var storedCategoryID, storedSpaceID string
	var userModified, kindConfirmed bool
	if err := db.QueryRow(ctx, `
		SELECT id::text,name,amount_type,expected_amount_minor,currency,frequency,
		       next_due_at::text,category_id::text,space_id::text,source,status,
		       detection_key,confidence_basis_points,occurrence_count,
		       user_modified_at IS NOT NULL,kind_confirmed_at IS NOT NULL
		FROM bills WHERE user_id=$1
	`, string(ownerID)).Scan(
		&billID, &name, &amountType, &amount, &currency, &frequency, &nextDate,
		&storedCategoryID, &storedSpaceID, &source, &status, &detectionKey,
		&confidence, &occurrence, &userModified, &kindConfirmed,
	); err != nil {
		t.Fatalf("load reclassified bill: %v", err)
	}
	if name != "Synthetic insurance payment" || amountType != "FIXED" || amount != 10000 ||
		currency != "CAD" || frequency != "MONTHLY" || nextDate != "2026-09-01" ||
		storedCategoryID != categoryID || storedSpaceID != spaceID || source != "DETECTED" ||
		status != "ACTIVE" || detectionKey == "" || confidence != 9000 || occurrence != 3 ||
		!userModified || !kindConfirmed {
		t.Fatalf("reclassified bill fields = %q %q %d %q %q %q %q %q %q %q %q %d %d %t %t", name, amountType, amount, currency, frequency, nextDate, storedCategoryID, storedSpaceID, source, status, detectionKey, confidence, occurrence, userModified, kindConfirmed)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM subscriptions WHERE user_id=$1`, string(ownerID)).Scan(&originCount); err != nil {
		t.Fatalf("verify removed subscription: %v", err)
	}
	if originCount != 0 {
		t.Fatal("reclassified subscription origin remains")
	}
	var inboxType, inboxEntityID, notificationType, notificationEntityID string
	if err := db.QueryRow(ctx, `SELECT entity_type,entity_id::text FROM inbox_items WHERE id=$1 AND user_id=$2`, inboxID, string(ownerID)).Scan(&inboxType, &inboxEntityID); err != nil {
		t.Fatalf("load reclassified inbox item: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT entity_type,entity_id::text FROM notifications WHERE id=$1 AND user_id=$2`, notificationID, string(ownerID)).Scan(&notificationType, &notificationEntityID); err != nil {
		t.Fatalf("load reclassified notification: %v", err)
	}
	if inboxType != string(entities.Bill) || inboxEntityID != billID ||
		notificationType != string(entities.Bill) || notificationEntityID != billID {
		t.Fatalf("repointed references = inbox %s/%s notification %s/%s", inboxType, inboxEntityID, notificationType, notificationEntityID)
	}
	var auditCount, outboxCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE actor_user_id=$1 AND event_type='RECURRING_RECLASSIFIED' AND entity_type=$2 AND entity_id=$3`, string(ownerID), string(entities.Bill), billID).Scan(&auditCount); err != nil {
		t.Fatalf("load recurring audit event: %v", err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE user_id=$1 AND event_type='FINANCIAL_MATERIALIZATION_REQUESTED'`, string(ownerID)).Scan(&outboxCount); err != nil {
		t.Fatalf("load recurring outbox event: %v", err)
	}
	if auditCount != 1 || outboxCount != 1 {
		t.Fatalf("reclassification side effects = audit %d outbox %d", auditCount, outboxCount)
	}
}
