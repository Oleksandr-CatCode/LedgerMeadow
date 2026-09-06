package changes

import (
	"context"
	"encoding/json"
	"errors"

	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
	"github.com/jackc/pgx/v5"
)

const channelName = "ledgermeadow_changes"

const (
	ResourceActivity      = "activity"
	ResourceInbox         = "inbox"
	ResourceNotifications = "notifications"
	ResourceDashboard     = "dashboard"
	ResourceAccounts      = "accounts"
)

var errInvalidEvent = errors.New("invalid change event")

type Event struct {
	UserID    shared.UserID `json:"user_id"`
	Resources []string      `json:"resources"`
}

type ChangeEvent struct {
	Resources []string `json:"resources"`
}

func Notify(ctx context.Context, tx pgx.Tx, userID shared.UserID, resources ...string) error {
	event := Event{UserID: userID, Resources: resources}
	if err := validateEvent(event); err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "SELECT pg_notify($1, $2)", channelName, string(payload))
	return err
}

func validateEvent(event Event) error {
	if !validate.UUID(string(event.UserID)) || len(event.Resources) == 0 || len(event.Resources) > 5 {
		return errInvalidEvent
	}
	seen := make(map[string]struct{}, len(event.Resources))
	for _, resource := range event.Resources {
		switch resource {
		case ResourceActivity, ResourceInbox, ResourceNotifications, ResourceDashboard, ResourceAccounts:
		default:
			return errInvalidEvent
		}
		if _, exists := seen[resource]; exists {
			return errInvalidEvent
		}
		seen[resource] = struct{}{}
	}
	return nil
}
