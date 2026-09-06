package plaid

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestLinkTokenRequestsUseLedgerMeadowAndCorrectUpdateMode(t *testing.T) {
	t.Parallel()
	type requestBody struct {
		ClientName  string   `json:"client_name"`
		Products    []string `json:"products"`
		RedirectURI string   `json:"redirect_uri"`
		AccessToken string   `json:"access_token"`
	}
	requests := make(chan requestBody, 2)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var body requestBody
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode Link token request: %v", err)
		}
		requests <- body
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"link_token":"link-test","expiration":"2026-08-22T12:00:00Z"}`)
	}))
	defer server.Close()
	client := newTestClient(t, "sandbox")
	client.baseURL = server.URL
	if _, err := client.CreateLinkToken(context.Background(), "user", "", "https://app.example.com/plaid/oauth"); err != nil {
		t.Fatalf("CreateLinkToken() error = %v", err)
	}
	initial := <-requests
	if initial.ClientName != "LedgerMeadow" || len(initial.Products) != 1 || initial.Products[0] != "transactions" ||
		initial.RedirectURI != "https://app.example.com/plaid/oauth" || initial.AccessToken != "" {
		t.Fatalf("initial Link request = %#v", initial)
	}
	if _, err := client.CreateUpdateLinkToken(
		context.Background(), "user", "", "https://app.example.com/plaid/oauth", "access-test",
	); err != nil {
		t.Fatalf("CreateUpdateLinkToken() error = %v", err)
	}
	update := <-requests
	if update.ClientName != "LedgerMeadow" || len(update.Products) != 0 ||
		update.RedirectURI != "https://app.example.com/plaid/oauth" || update.AccessToken != "access-test" {
		t.Fatalf("update-mode Link request = %#v", update)
	}
}

func TestClientEnvironmentMapping(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		environment string
		baseURL     string
	}{
		{environment: "sandbox", baseURL: "https://sandbox.plaid.com"},
		{environment: "production", baseURL: "https://production.plaid.com"},
	} {
		t.Run(test.environment, func(t *testing.T) {
			t.Parallel()
			client, err := NewClient("client", "secret", test.environment)
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			if client.baseURL != test.baseURL || client.environment != test.environment {
				t.Fatalf("NewClient() = baseURL %q, environment %q", client.baseURL, client.environment)
			}
		})
	}

	if _, err := NewClient("client", "secret", "development"); err == nil {
		t.Fatal("NewClient() accepted an unsupported environment")
	}
}

func TestClientRejectsCredentialBearingRedirects(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			redirected := make(chan struct{}, 1)
			destination := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				redirected <- struct{}{}
				fmt.Fprint(response, `{"accounts":[]}`)
			}))
			defer destination.Close()
			provider := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				http.Redirect(response, request, destination.URL, status)
			}))
			defer provider.Close()
			client := newTestClient(t, "sandbox")
			client.baseURL = provider.URL

			if _, err := client.GetAccounts(context.Background(), "synthetic-access-token"); err == nil {
				t.Error("provider redirect was accepted")
			}
			select {
			case <-redirected:
				t.Error("credential-bearing request reached the redirect destination")
			default:
			}
		})
	}
}

func TestRemoveItemRequiresPlaidConfirmation(t *testing.T) {
	t.Parallel()

	removed := true
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/item/remove" {
			t.Errorf("item removal request = %s %s", request.Method, request.URL.Path)
		}
		var body struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode item removal request: %v", err)
		}
		if body.AccessToken != "access-test" {
			t.Error("item removal request did not contain the expected access token")
		}
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(response, `{"removed":%t}`, removed)
	}))
	defer server.Close()
	client := newTestClient(t, "sandbox")
	client.baseURL = server.URL

	if err := client.RemoveItem(context.Background(), "access-test"); err != nil {
		t.Fatalf("RemoveItem() error = %v", err)
	}
	removed = false
	if err := client.RemoveItem(context.Background(), "access-test"); err == nil {
		t.Fatal("RemoveItem() accepted an unconfirmed removal")
	}
	if err := client.RemoveItem(context.Background(), ""); err == nil {
		t.Fatal("RemoveItem() accepted an empty access token")
	}
}

func TestGetAccountsPreservesExactProviderDecimal(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{"accounts":[{"account_id":"a1","name":"Chequing","type":"depository","balances":{"current":9007199254740993.01,"available":100.25,"iso_currency_code":"CAD"}}]}`)
	}))
	defer server.Close()
	client := newTestClient(t, "sandbox")
	client.baseURL = server.URL

	accounts, err := client.GetAccounts(context.Background(), "access-token")
	if err != nil {
		t.Fatalf("GetAccounts() error = %v", err)
	}
	if got := string(*accounts[0].Balances.Current); got != "9007199254740993.01" {
		t.Fatalf("current balance = %s", got)
	}
}

func TestVerifyWebhookRejectsReplayAndTampering(t *testing.T) {
	t.Parallel()

	body := []byte(`{"webhook_type":"TRANSACTIONS"}`)
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	client := newTestClient(t, "sandbox")
	signed := signWebhook(t, client, body, now)

	if err := client.VerifyWebhook(context.Background(), signed, body, now); err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if err := client.VerifyWebhook(context.Background(), signed, []byte(`{}`), now); err == nil {
		t.Fatal("tampered webhook body was accepted")
	}
	if err := client.VerifyWebhook(context.Background(), signed, body, now.Add(6*time.Minute)); err == nil {
		t.Fatal("replayed webhook token was accepted")
	}
}

func newTestClient(t *testing.T, environment string) *Client {
	t.Helper()
	client, err := NewClient("client", "secret", environment)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func signWebhook(t *testing.T, client *Client, body []byte, now time.Time) string {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	digest := sha256.Sum256(body)
	claims := webhookClaims{
		RequestBodySHA256: hex.EncodeToString(digest[:]),
		RegisteredClaims:  jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now)},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("sign webhook token: %v", err)
	}
	client.keys["test-key"] = cachedVerificationKey{key: &privateKey.PublicKey, expiresAt: now.Add(time.Hour)}
	return signed
}
