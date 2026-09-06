package config

import "testing"

func TestLoadAcceptsConfiguredPlaidEnvironments(t *testing.T) {
	for _, environment := range []string{"sandbox", "production"} {
		t.Run(environment, func(t *testing.T) {
			setRequiredEnvironment(t)
			t.Setenv("PLAID_ENV", environment)
			t.Setenv("PLAID_REDIRECT_URI", "https://app.example.com/plaid/oauth")

			loaded, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if loaded.PlaidEnvironment != environment {
				t.Fatalf("PlaidEnvironment = %q, want %q", loaded.PlaidEnvironment, environment)
			}
			if loaded.PlaidRedirectURI != "https://app.example.com/plaid/oauth" {
				t.Fatalf("PlaidRedirectURI = %q", loaded.PlaidRedirectURI)
			}
		})
	}
}

func TestLoadRejectsUnsupportedPlaidEnvironment(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("PLAID_ENV", "development")

	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted an unsupported PLAID_ENV")
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"DATABASE_URL":              "postgres://test",
		"DATABASE_DIRECT_URL":       "postgres://test",
		"CLERK_SECRET_KEY":          "clerk-test",
		"CLERK_AUTHORIZED_PARTIES":  "https://app.example.com",
		"PLAID_CLIENT_ID":           "plaid-client-test",
		"PLAID_SECRET":              "plaid-secret-test",
		"FINANCIAL_ENGINE_ADDR":     "127.0.0.1:50051",
		"TOKEN_ENCRYPTION_KEY_FILE": "test-key-file",
		"WORKER_POLL_INTERVAL":      "1s",
	} {
		t.Setenv(name, value)
	}
}
