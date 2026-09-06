package plaid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	shared "ledgermeadow/src/shared/types"
)

func TestWebhookHandlerRequiresConfiguredEnvironment(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name               string
		clientEnvironment  string
		webhookEnvironment string
		status             int
		lookupCalls        int
		enqueueCalls       int
	}{
		{
			name: "sandbox", clientEnvironment: "sandbox", webhookEnvironment: "sandbox",
			status: http.StatusNoContent, lookupCalls: 1, enqueueCalls: 1,
		},
		{
			name: "production", clientEnvironment: "production", webhookEnvironment: "production",
			status: http.StatusNoContent, lookupCalls: 1, enqueueCalls: 1,
		},
		{
			name: "mismatch", clientEnvironment: "production", webhookEnvironment: "sandbox",
			status: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
			body := []byte(`{"webhook_type":"TRANSACTIONS","webhook_code":"SYNC_UPDATES_AVAILABLE","item_id":"item-test","environment":"` + test.webhookEnvironment + `"}`)
			client := newTestClient(t, test.clientEnvironment)
			verification := signWebhook(t, client, body, now)
			repository := &recordingWebhookRepository{}
			handler := NewWebhookHandler(client, repository)
			handler.now = func() time.Time { return now }

			request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/plaid", strings.NewReader(string(body)))
			request.Header.Set("Plaid-Verification", verification)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if repository.lookupCalls != test.lookupCalls || repository.enqueueCalls != test.enqueueCalls {
				t.Fatalf(
					"repository calls = lookup %d, enqueue %d; want lookup %d, enqueue %d",
					repository.lookupCalls, repository.enqueueCalls, test.lookupCalls, test.enqueueCalls,
				)
			}
		})
	}
}

type recordingWebhookRepository struct {
	lookupCalls  int
	enqueueCalls int
}

func (r *recordingWebhookRepository) ConnectionByProviderItem(
	context.Context,
	string,
) (shared.BankConnectionID, shared.UserID, error) {
	r.lookupCalls++
	return "connection-test", "user-test", nil
}

func (r *recordingWebhookRepository) EnqueueWebhookSync(
	context.Context,
	shared.BankConnectionID,
	shared.UserID,
	string,
) error {
	r.enqueueCalls++
	return nil
}
