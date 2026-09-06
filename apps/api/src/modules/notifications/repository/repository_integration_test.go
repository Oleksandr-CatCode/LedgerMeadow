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

func TestEmptyNotificationCollectionsAreNonNil(t *testing.T) {
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

	var userID shared.UserID
	clerkUserID := fmt.Sprintf("notification-empty-%d", time.Now().UnixNano())
	if err := db.QueryRow(ctx, `
		INSERT INTO users (clerk_user_id) VALUES ($1) RETURNING id::text
	`, clerkUserID).Scan(&userID); err != nil {
		t.Fatalf("create notification test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, string(userID))
	})

	repository := New(db)
	notifications, err := repository.List(ctx, userID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if notifications == nil || len(notifications) != 0 {
		t.Fatalf("List() = %#v, want a non-nil empty collection", notifications)
	}
	preferences, err := repository.Preferences(ctx, userID)
	if err != nil {
		t.Fatalf("Preferences() error = %v", err)
	}
	if preferences == nil || len(preferences) != 0 {
		t.Fatalf("Preferences() = %#v, want a non-nil empty collection", preferences)
	}
}

func TestMarkReadRejectsAnotherUsersNotification(t *testing.T) {
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

	suffix := time.Now().UnixNano()
	var ownerID, otherID shared.UserID
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("notification-owner-%d", suffix)).Scan(&ownerID); err != nil {
		t.Fatalf("create notification owner: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, fmt.Sprintf("notification-other-%d", suffix)).Scan(&otherID); err != nil {
		t.Fatalf("create other user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=ANY($1::uuid[])`, []string{string(ownerID), string(otherID)})
	})

	var notificationID string
	if err := db.QueryRow(ctx, `
		INSERT INTO notifications(user_id,notification_type,title,body)
		VALUES($1,'NEW_RECURRING','Synthetic notification','Synthetic notification body')
		RETURNING id::text
	`, string(ownerID)).Scan(&notificationID); err != nil {
		t.Fatalf("create notification: %v", err)
	}

	repository := New(db)
	if err := repository.MarkRead(ctx, otherID, notificationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("MarkRead() error = %v, want ErrNotFound", err)
	}
	var read bool
	if err := db.QueryRow(ctx, `SELECT read_at IS NOT NULL FROM notifications WHERE id=$1 AND user_id=$2`, notificationID, string(ownerID)).Scan(&read); err != nil {
		t.Fatalf("load notification read state: %v", err)
	}
	if read {
		t.Fatal("another user marked the notification read")
	}
}
