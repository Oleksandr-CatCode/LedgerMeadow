package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr               string
	WebOrigin              string
	DatabaseURL            string
	DatabaseDirectURL      string
	ClerkSecretKey         string
	ClerkAuthorizedParties []string
	PlaidClientID          string
	PlaidSecret            string
	PlaidEnvironment       string
	PlaidWebhookURL        string
	PlaidRedirectURI       string
	FinancialEngineAddr    string
	TokenKeyFile           string
	WorkerPollInterval     time.Duration
}

func Load() (Config, error) {
	pollInterval, err := time.ParseDuration(valueOrDefault("WORKER_POLL_INTERVAL", "1s"))
	if err != nil || pollInterval < 100*time.Millisecond {
		return Config{}, errors.New("WORKER_POLL_INTERVAL must be a duration of at least 100ms")
	}

	config := Config{
		HTTPAddr:               valueOrDefault("HTTP_ADDR", "127.0.0.1:8080"),
		WebOrigin:              valueOrDefault("WEB_ORIGIN", "http://127.0.0.1:5173"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		DatabaseDirectURL:      os.Getenv("DATABASE_DIRECT_URL"),
		ClerkSecretKey:         os.Getenv("CLERK_SECRET_KEY"),
		ClerkAuthorizedParties: splitNonEmpty(os.Getenv("CLERK_AUTHORIZED_PARTIES")),
		PlaidClientID:          os.Getenv("PLAID_CLIENT_ID"),
		PlaidSecret:            os.Getenv("PLAID_SECRET"),
		PlaidEnvironment:       valueOrDefault("PLAID_ENV", "sandbox"),
		PlaidWebhookURL:        os.Getenv("PLAID_WEBHOOK_URL"),
		PlaidRedirectURI:       os.Getenv("PLAID_REDIRECT_URI"),
		FinancialEngineAddr:    os.Getenv("FINANCIAL_ENGINE_ADDR"),
		TokenKeyFile:           os.Getenv("TOKEN_ENCRYPTION_KEY_FILE"),
		WorkerPollInterval:     pollInterval,
	}

	switch config.PlaidEnvironment {
	case "sandbox", "production":
	default:
		return Config{}, errors.New("PLAID_ENV must be sandbox or production")
	}
	required := map[string]string{
		"DATABASE_URL":              config.DatabaseURL,
		"DATABASE_DIRECT_URL":       config.DatabaseDirectURL,
		"CLERK_SECRET_KEY":          config.ClerkSecretKey,
		"CLERK_AUTHORIZED_PARTIES":  strings.Join(config.ClerkAuthorizedParties, ","),
		"PLAID_CLIENT_ID":           config.PlaidClientID,
		"PLAID_SECRET":              config.PlaidSecret,
		"FINANCIAL_ENGINE_ADDR":     config.FinancialEngineAddr,
		"TOKEN_ENCRYPTION_KEY_FILE": config.TokenKeyFile,
	}
	for name, value := range required {
		if value == "" {
			return Config{}, fmt.Errorf("%s is required", name)
		}
	}

	return config, nil
}

func valueOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func splitNonEmpty(value string) []string {
	var values []string
	for _, item := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
