package changes

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	shared "ledgermeadow/src/shared/types"
)

func TestHubEnforcesStreamLimits(t *testing.T) {
	t.Run("per user", func(t *testing.T) {
		hub := NewHub()
		for i := 0; i < maxStreamsPerUser; i++ {
			if _, _, err := hub.Subscribe(shared.UserID("user-a")); err != nil {
				t.Fatalf("Subscribe() at per-user slot %d error = %v", i, err)
			}
		}
		if _, _, err := hub.Subscribe(shared.UserID("user-a")); !errors.Is(err, ErrUserStreamLimit) {
			t.Fatalf("Subscribe() beyond per-user limit error = %v, want %v", err, ErrUserStreamLimit)
		}
	})

	t.Run("per replica", func(t *testing.T) {
		hub := NewHub()
		for i := 0; i < maxStreamsPerReplica; i++ {
			userID := shared.UserID(fmt.Sprintf("user-%d", i))
			if _, _, err := hub.Subscribe(userID); err != nil {
				t.Fatalf("Subscribe() at replica slot %d error = %v", i, err)
			}
		}
		if _, _, err := hub.Subscribe(shared.UserID("overflow")); !errors.Is(err, ErrReplicaStreamLimit) {
			t.Fatalf("Subscribe() beyond replica limit error = %v, want %v", err, ErrReplicaStreamLimit)
		}
	})
}

func TestHubCoalescesBufferedResources(t *testing.T) {
	hub := NewHub()
	userID := shared.UserID("user-a")
	stream, unsubscribe, err := hub.Subscribe(userID)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer unsubscribe()

	hub.Publish(Event{UserID: userID, Resources: []string{ResourceInbox, ResourceNotifications, ResourceActivity}})
	hub.Publish(Event{UserID: userID, Resources: []string{ResourceDashboard, ResourceActivity}})

	event := <-stream
	want := []string{ResourceInbox, ResourceNotifications, ResourceActivity, ResourceDashboard}
	if !slices.Equal(event.Resources, want) {
		t.Fatalf("coalesced resources = %v, want %v", event.Resources, want)
	}
}

func TestDecodeEventRejectsUnknownResource(t *testing.T) {
	payload := `{"user_id":"00000000-0000-0000-0000-000000000001","resources":["inbox'); SELECT 1; --"]}`
	if _, err := decodeEvent(payload); !errors.Is(err, errInvalidEvent) {
		t.Fatalf("decodeEvent() error = %v, want %v", err, errInvalidEvent)
	}
}
