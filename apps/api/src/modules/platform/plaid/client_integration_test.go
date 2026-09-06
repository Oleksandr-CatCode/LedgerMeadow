package plaid

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestCreateLinkTokenIntegration(t *testing.T) {
	if os.Getenv("PLAID_INTEGRATION") != "1" {
		t.Skip("PLAID_INTEGRATION is not enabled")
	}
	clientID := os.Getenv("PLAID_CLIENT_ID")
	secret := os.Getenv("PLAID_SECRET")
	environment := os.Getenv("PLAID_ENV")
	if clientID == "" || secret == "" || environment == "" {
		t.Fatal("Plaid integration credentials and environment are not configured")
	}
	client, err := NewClient(clientID, secret, environment)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	token, err := client.CreateLinkToken(ctx, "integration-test-user", "", os.Getenv("PLAID_REDIRECT_URI"))
	if err != nil {
		t.Fatalf("CreateLinkToken() error = %v", err)
	}
	if token.LinkToken == "" || token.Expiration.Before(time.Now()) {
		t.Fatal("Plaid returned an invalid Link token")
	}
}
