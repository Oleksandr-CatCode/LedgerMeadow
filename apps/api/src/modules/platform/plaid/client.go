package plaid

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const maxResponseBytes = 8 << 20

type Decimal string

func (d *Decimal) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || len(data) > 64 || bytes.Equal(data, []byte("null")) || data[0] == '"' {
		return errors.New("Plaid decimal must be a bounded JSON number")
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("decode Plaid decimal: %w", err)
	}
	*d = Decimal(number.String())
	return nil
}

type Client struct {
	baseURL     string
	environment string
	clientID    string
	secret      string
	http        *http.Client
	keyMu       sync.RWMutex
	keys        map[string]cachedVerificationKey
}

type cachedVerificationKey struct {
	key       any
	expiresAt time.Time
}

func NewClient(clientID string, secret string, environment string) (*Client, error) {
	var baseURL string
	switch environment {
	case "sandbox":
		baseURL = "https://sandbox.plaid.com"
	case "production":
		baseURL = "https://production.plaid.com"
	default:
		return nil, errors.New("Plaid environment must be sandbox or production")
	}
	return &Client{
		baseURL:     baseURL,
		environment: environment,
		clientID:    clientID,
		secret:      secret,
		http: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		keys: make(map[string]cachedVerificationKey),
	}, nil
}

type LinkToken struct {
	LinkToken  string
	Expiration time.Time
}

func (c *Client) CreateLinkToken(ctx context.Context, clientUserID string, webhookURL string, redirectURI string) (LinkToken, error) {
	return c.createLinkToken(ctx, clientUserID, webhookURL, redirectURI, "")
}

func (c *Client) CreateUpdateLinkToken(ctx context.Context, clientUserID string, webhookURL string, redirectURI string, accessToken string) (LinkToken, error) {
	if accessToken == "" {
		return LinkToken{}, errors.New("Plaid update mode requires an access token")
	}
	return c.createLinkToken(ctx, clientUserID, webhookURL, redirectURI, accessToken)
}

func (c *Client) createLinkToken(ctx context.Context, clientUserID string, webhookURL string, redirectURI string, accessToken string) (LinkToken, error) {
	request := struct {
		ClientName   string   `json:"client_name"`
		CountryCodes []string `json:"country_codes"`
		Language     string   `json:"language"`
		Products     []string `json:"products,omitempty"`
		User         struct {
			ClientUserID string `json:"client_user_id"`
		} `json:"user"`
		Webhook     string `json:"webhook,omitempty"`
		RedirectURI string `json:"redirect_uri,omitempty"`
		AccessToken string `json:"access_token,omitempty"`
	}{
		ClientName:   "LedgerMeadow",
		CountryCodes: []string{"CA", "US"},
		Language:     "en",
		Webhook:      webhookURL,
		RedirectURI:  redirectURI,
		AccessToken:  accessToken,
	}
	if accessToken == "" {
		request.Products = []string{"transactions"}
	}
	request.User.ClientUserID = clientUserID

	var response struct {
		LinkToken  string    `json:"link_token"`
		Expiration time.Time `json:"expiration"`
	}
	if err := c.post(ctx, "/link/token/create", request, &response); err != nil {
		return LinkToken{}, err
	}
	if response.LinkToken == "" {
		return LinkToken{}, errors.New("Plaid returned an empty link token")
	}
	return LinkToken{LinkToken: response.LinkToken, Expiration: response.Expiration}, nil
}

type ExchangeResult struct {
	AccessToken string
	ItemID      string
}

func (c *Client) ExchangePublicToken(ctx context.Context, publicToken string) (ExchangeResult, error) {
	request := struct {
		PublicToken string `json:"public_token"`
	}{PublicToken: publicToken}
	var response struct {
		AccessToken string `json:"access_token"`
		ItemID      string `json:"item_id"`
	}
	if err := c.post(ctx, "/item/public_token/exchange", request, &response); err != nil {
		return ExchangeResult{}, err
	}
	if response.AccessToken == "" || response.ItemID == "" {
		return ExchangeResult{}, errors.New("Plaid returned an incomplete token exchange")
	}
	return ExchangeResult{AccessToken: response.AccessToken, ItemID: response.ItemID}, nil
}

func (c *Client) RemoveItem(ctx context.Context, accessToken string) error {
	if accessToken == "" {
		return errors.New("Plaid item removal requires an access token")
	}
	request := struct {
		AccessToken string `json:"access_token"`
	}{AccessToken: accessToken}
	var response struct {
		Removed bool `json:"removed"`
	}
	if err := c.post(ctx, "/item/remove", request, &response); err != nil {
		return err
	}
	if !response.Removed {
		return errors.New("Plaid did not confirm item removal")
	}
	return nil
}

type Account struct {
	AccountID    string   `json:"account_id"`
	Name         string   `json:"name"`
	OfficialName *string  `json:"official_name"`
	Mask         *string  `json:"mask"`
	Type         string   `json:"type"`
	Subtype      *string  `json:"subtype"`
	Balances     Balances `json:"balances"`
}

type Balances struct {
	Available       *Decimal `json:"available"`
	Current         *Decimal `json:"current"`
	ISOCurrencyCode *string  `json:"iso_currency_code"`
}

func (c *Client) GetAccounts(ctx context.Context, accessToken string) ([]Account, error) {
	request := struct {
		AccessToken string `json:"access_token"`
	}{AccessToken: accessToken}
	var response struct {
		Accounts []Account `json:"accounts"`
	}
	if err := c.post(ctx, "/accounts/get", request, &response); err != nil {
		return nil, err
	}
	return response.Accounts, nil
}

type Transaction struct {
	TransactionID           string                   `json:"transaction_id"`
	PendingTransactionID    *string                  `json:"pending_transaction_id"`
	AccountID               string                   `json:"account_id"`
	Name                    string                   `json:"name"`
	MerchantName            *string                  `json:"merchant_name"`
	Amount                  Decimal                  `json:"amount"`
	ISOCurrencyCode         *string                  `json:"iso_currency_code"`
	Date                    string                   `json:"date"`
	AuthorizedDate          *string                  `json:"authorized_date"`
	Pending                 bool                     `json:"pending"`
	PersonalFinanceCategory *PersonalFinanceCategory `json:"personal_finance_category"`
}

type PersonalFinanceCategory struct {
	Primary  string `json:"primary"`
	Detailed string `json:"detailed"`
}

type RemovedTransaction struct {
	TransactionID string `json:"transaction_id"`
	AccountID     string `json:"account_id"`
}

type SyncPage struct {
	Added      []Transaction
	Modified   []Transaction
	Removed    []RemovedTransaction
	NextCursor string
	HasMore    bool
}

func (c *Client) SyncTransactions(ctx context.Context, accessToken string, cursor string, count int) (SyncPage, error) {
	request := struct {
		AccessToken string `json:"access_token"`
		Cursor      string `json:"cursor,omitempty"`
		Count       int    `json:"count"`
	}{AccessToken: accessToken, Cursor: cursor, Count: count}
	var response struct {
		Added      []Transaction        `json:"added"`
		Modified   []Transaction        `json:"modified"`
		Removed    []RemovedTransaction `json:"removed"`
		NextCursor string               `json:"next_cursor"`
		HasMore    bool                 `json:"has_more"`
	}
	if err := c.post(ctx, "/transactions/sync", request, &response); err != nil {
		return SyncPage{}, err
	}
	return SyncPage{
		Added: response.Added, Modified: response.Modified, Removed: response.Removed,
		NextCursor: response.NextCursor, HasMore: response.HasMore,
	}, nil
}

type APIError struct {
	Code string
}

func (e *APIError) Error() string {
	return "Plaid request failed: " + e.Code
}

func (c *Client) post(ctx context.Context, path string, payload any, destination any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Plaid request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Plaid request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("PLAID-CLIENT-ID", c.clientID)
	request.Header.Set("PLAID-SECRET", c.secret)

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("send Plaid request: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(responseBody) > maxResponseBytes {
		return errors.New("Plaid response exceeded the configured bound")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var providerError struct {
			ErrorCode string `json:"error_code"`
		}
		if err := json.Unmarshal(responseBody, &providerError); err != nil || providerError.ErrorCode == "" {
			return &APIError{Code: "PROVIDER_ERROR"}
		}
		return &APIError{Code: providerError.ErrorCode}
	}
	if err := json.Unmarshal(responseBody, destination); err != nil {
		return fmt.Errorf("decode Plaid response: %w", err)
	}
	return nil
}

type webhookClaims struct {
	RequestBodySHA256 string `json:"request_body_sha256"`
	jwt.RegisteredClaims
}

func (c *Client) VerifyWebhook(ctx context.Context, verificationToken string, body []byte, now time.Time) error {
	if verificationToken == "" {
		return errors.New("missing Plaid webhook verification token")
	}

	unverified := &webhookClaims{}
	parsed, _, err := jwt.NewParser().ParseUnverified(verificationToken, unverified)
	if err != nil || parsed.Method.Alg() != jwt.SigningMethodES256.Alg() {
		return errors.New("invalid Plaid webhook token header")
	}
	kid, ok := parsed.Header["kid"].(string)
	if !ok || kid == "" {
		return errors.New("Plaid webhook token is missing kid")
	}

	key, err := c.verificationKey(ctx, kid, now)
	if err != nil {
		return err
	}
	claims := &webhookClaims{}
	token, err := jwt.ParseWithClaims(
		verificationToken,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodES256 {
				return nil, errors.New("unexpected Plaid webhook signing method")
			}
			return key, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}),
	)
	if err != nil || !token.Valid || claims.IssuedAt == nil {
		return errors.New("invalid Plaid webhook signature")
	}
	issuedAt := claims.IssuedAt.Time
	if issuedAt.Before(now.Add(-5*time.Minute)) || issuedAt.After(now.Add(time.Minute)) {
		return errors.New("Plaid webhook token is outside the replay window")
	}

	digest := sha256.Sum256(body)
	expected := hex.EncodeToString(digest[:])
	if len(expected) != len(claims.RequestBodySHA256) ||
		subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(claims.RequestBodySHA256))) != 1 {
		return errors.New("Plaid webhook body hash does not match")
	}
	return nil
}

func (c *Client) verificationKey(ctx context.Context, kid string, now time.Time) (any, error) {
	c.keyMu.RLock()
	cached, ok := c.keys[kid]
	c.keyMu.RUnlock()
	if ok && now.Before(cached.expiresAt) {
		return cached.key, nil
	}

	request := struct {
		KeyID string `json:"key_id"`
	}{KeyID: kid}
	var response struct {
		Key struct {
			Algorithm string `json:"alg"`
			Curve     string `json:"crv"`
			KeyID     string `json:"kid"`
			KeyType   string `json:"kty"`
			Use       string `json:"use"`
			X         string `json:"x"`
			Y         string `json:"y"`
			ExpiredAt *int64 `json:"expired_at"`
		} `json:"key"`
	}
	if err := c.post(ctx, "/webhook_verification_key/get", request, &response); err != nil {
		return nil, fmt.Errorf("get Plaid webhook verification key: %w", err)
	}
	providerKey := response.Key
	if providerKey.Algorithm != "ES256" || providerKey.Curve != "P-256" ||
		providerKey.KeyID != kid || providerKey.KeyType != "EC" || providerKey.Use != "sig" {
		return nil, errors.New("Plaid webhook verification key metadata is invalid")
	}
	x, err := base64.RawURLEncoding.DecodeString(providerKey.X)
	if err != nil {
		return nil, errors.New("Plaid webhook verification x-coordinate is invalid")
	}
	y, err := base64.RawURLEncoding.DecodeString(providerKey.Y)
	if err != nil {
		return nil, errors.New("Plaid webhook verification y-coordinate is invalid")
	}
	if len(x) != 32 || len(y) != 32 {
		return nil, errors.New("Plaid webhook verification coordinates have an invalid length")
	}
	point := make([]byte, 1+len(x)+len(y))
	point[0] = 4
	copy(point[1:], x)
	copy(point[1+len(x):], y)
	xCoordinate, yCoordinate := elliptic.Unmarshal(elliptic.P256(), point)
	if xCoordinate == nil || yCoordinate == nil {
		return nil, errors.New("Plaid webhook verification point is invalid")
	}
	publicKey := &ecdsa.PublicKey{Curve: elliptic.P256(), X: xCoordinate, Y: yCoordinate}
	expiresAt := now.Add(6 * time.Hour)
	if providerKey.ExpiredAt != nil {
		expiresAt = time.Unix(*providerKey.ExpiredAt, 0)
		if !expiresAt.After(now) {
			return nil, errors.New("Plaid webhook verification key is expired")
		}
	}
	c.keyMu.Lock()
	c.keys[kid] = cachedVerificationKey{key: publicKey, expiresAt: expiresAt}
	c.keyMu.Unlock()
	return publicKey, nil
}
