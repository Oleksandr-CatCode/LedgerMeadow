package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"ledgermeadow/src/modules/rules/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRuleUpdateReorderAndDeleteCompaction(t *testing.T) {
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
	if err := db.QueryRow(ctx, `INSERT INTO users(clerk_user_id) VALUES($1) RETURNING id::text`, "rules-test-"+suffix).Scan(&userID); err != nil {
		t.Fatalf("create rule test user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM audit_events WHERE actor_user_id=$1`, string(userID))
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, string(userID))
	})

	repository := New(db)
	create := func(name string) string {
		id, err := repository.Create(ctx, userID, models.Create{
			Name: name, Enabled: true,
			Conditions: []models.Condition{{Field: "merchant", Operator: "contains", Value: name}},
			Actions:    []models.Action{{Type: "mark_reviewed"}},
		})
		if err != nil {
			t.Fatalf("create rule %q: %v", name, err)
		}
		return id
	}
	firstID := create("First")
	secondID := create("Second")
	thirdID := create("Third")

	updatedConditions := []models.Condition{{Field: "amount", Operator: "greater_than", Value: "1000"}}
	updatedActions := []models.Action{{Type: "create_notification", Value: "large"}}
	if err := repository.Update(ctx, userID, firstID, models.Update{Conditions: &updatedConditions, Actions: &updatedActions}); err != nil {
		t.Fatalf("update rule JSONB: %v", err)
	}
	var conditions, actions json.RawMessage
	if err := db.QueryRow(ctx, `SELECT conditions,actions FROM rules WHERE user_id=$1 AND id=$2`, string(userID), firstID).Scan(&conditions, &actions); err != nil {
		t.Fatalf("load updated rule JSONB: %v", err)
	}
	var decodedConditions []models.Condition
	var decodedActions []models.Action
	if err := json.Unmarshal(conditions, &decodedConditions); err != nil || len(decodedConditions) != 1 || decodedConditions[0] != updatedConditions[0] {
		t.Fatalf("updated conditions were not persisted: %s (%v)", conditions, err)
	}
	if err := json.Unmarshal(actions, &decodedActions); err != nil || len(decodedActions) != 1 || decodedActions[0] != updatedActions[0] {
		t.Fatalf("updated actions were not persisted: %s (%v)", actions, err)
	}

	if err := repository.Reorder(ctx, userID, []string{thirdID, firstID, secondID}); err != nil {
		t.Fatalf("swap rule priorities: %v", err)
	}
	items, err := repository.List(ctx, userID)
	if err != nil {
		t.Fatalf("list reordered rules: %v", err)
	}
	if len(items) != 3 || items[0].ID != thirdID || items[1].ID != firstID || items[2].ID != secondID || items[0].Priority != 1 || items[1].Priority != 2 || items[2].Priority != 3 {
		t.Fatalf("unexpected reordered rules: %#v", items)
	}

	if err := repository.Delete(ctx, userID, firstID); err != nil {
		t.Fatalf("delete middle rule: %v", err)
	}
	items, err = repository.List(ctx, userID)
	if err != nil {
		t.Fatalf("list compacted rules: %v", err)
	}
	if len(items) != 2 || items[0].ID != thirdID || items[1].ID != secondID || items[0].Priority != 1 || items[1].Priority != 2 {
		t.Fatalf("unexpected compacted rules: %#v", items)
	}
}
