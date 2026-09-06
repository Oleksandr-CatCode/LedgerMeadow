package services

import (
	"context"
	"errors"
	"testing"

	bankingrepo "ledgermeadow/src/modules/banking/repository"
	banking "ledgermeadow/src/modules/banking/types"
	"ledgermeadow/src/modules/platform/plaid"
	shared "ledgermeadow/src/shared/types"
)

func TestCreateLinkTokenPassesConfiguredPlaidURLs(t *testing.T) {
	t.Parallel()
	provider := &recordingLinkProvider{}
	service := New(
		provider, nil, nil,
		"https://api.example.com/api/v1/webhooks/plaid",
		"https://app.example.com/plaid/oauth",
	)

	if _, err := service.CreateLinkToken(context.Background(), "user-test"); err != nil {
		t.Fatalf("CreateLinkToken() error = %v", err)
	}
	if provider.clientUserID != "user-test" {
		t.Fatalf("client user ID = %q", provider.clientUserID)
	}
	if provider.webhookURL != "https://api.example.com/api/v1/webhooks/plaid" {
		t.Fatalf("webhook URL = %q", provider.webhookURL)
	}
	if provider.redirectURI != "https://app.example.com/plaid/oauth" {
		t.Fatalf("redirect URI = %q", provider.redirectURI)
	}
}

type recordingLinkProvider struct {
	clientUserID string
	webhookURL   string
	redirectURI  string
	removedToken string
	removeErr    error
}

func (p *recordingLinkProvider) CreateLinkToken(
	_ context.Context,
	clientUserID string,
	webhookURL string,
	redirectURI string,
) (plaid.LinkToken, error) {
	p.clientUserID = clientUserID
	p.webhookURL = webhookURL
	p.redirectURI = redirectURI
	return plaid.LinkToken{}, nil
}

func (p *recordingLinkProvider) CreateUpdateLinkToken(
	context.Context,
	string,
	string,
	string,
	string,
) (plaid.LinkToken, error) {
	return plaid.LinkToken{}, nil
}

func (p *recordingLinkProvider) ExchangePublicToken(context.Context, string) (plaid.ExchangeResult, error) {
	return plaid.ExchangeResult{}, nil
}

func (p *recordingLinkProvider) RemoveItem(_ context.Context, accessToken string) error {
	p.removedToken = accessToken
	return p.removeErr
}

func TestDisconnectRevokesProviderBeforeLocalTombstone(t *testing.T) {
	t.Parallel()
	provider := &recordingLinkProvider{}
	repository := &disconnectRepository{ciphertext: []byte("encrypted")}
	service := New(provider, disconnectCipher{}, repository, "", "")

	err := service.Disconnect(context.Background(), "user-test", "connection-test")
	if err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if provider.removedToken != "access-test" || !repository.disconnected {
		t.Fatal("Disconnect() did not revoke Plaid before completing local disconnect")
	}
}

func TestDisconnectDoesNotTombstoneWhenProviderRemovalFails(t *testing.T) {
	t.Parallel()
	provider := &recordingLinkProvider{removeErr: errors.New("provider unavailable")}
	repository := &disconnectRepository{ciphertext: []byte("encrypted")}
	service := New(provider, disconnectCipher{}, repository, "", "")

	err := service.Disconnect(context.Background(), "user-test", "connection-test")
	if !errors.Is(err, ErrProviderUnavailable) || repository.disconnected {
		t.Fatalf("Disconnect() error = %v, disconnected = %t", err, repository.disconnected)
	}
}

func TestDisconnectEnforcesOwnershipBeforeProviderCall(t *testing.T) {
	t.Parallel()
	provider := &recordingLinkProvider{}
	repository := &disconnectRepository{loadErr: bankingrepo.ErrNotFound}
	service := New(provider, disconnectCipher{}, repository, "", "")

	err := service.Disconnect(context.Background(), "user-test", "connection-test")
	if !errors.Is(err, bankingrepo.ErrNotFound) || provider.removedToken != "" || repository.disconnected {
		t.Fatalf("Disconnect() error = %v, provider called = %t", err, provider.removedToken != "")
	}
}

func TestDisconnectFileImportSkipsPlaidRemoval(t *testing.T) {
	t.Parallel()
	provider := &recordingLinkProvider{}
	repository := &disconnectRepository{provider: "FILE_IMPORT"}
	service := New(provider, disconnectCipher{}, repository, "", "")

	if err := service.Disconnect(context.Background(), "user-test", "connection-test"); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if provider.removedToken != "" || !repository.disconnected {
		t.Fatalf("file import disconnect called Plaid = %t, disconnected = %t", provider.removedToken != "", repository.disconnected)
	}
}

func TestRefreshFileImportIsRejectedBeforeQueue(t *testing.T) {
	t.Parallel()
	repository := &disconnectRepository{provider: "FILE_IMPORT"}
	service := New(&recordingLinkProvider{}, disconnectCipher{}, repository, "", "")

	err := service.Refresh(context.Background(), "user-test", "connection-test")
	if !errors.Is(err, ErrUnsupportedOperation) {
		t.Fatalf("Refresh() error = %v, want ErrUnsupportedOperation", err)
	}
}

type disconnectCipher struct{}

func (disconnectCipher) Encrypt(string) ([]byte, error) { return nil, nil }
func (disconnectCipher) Decrypt([]byte) (string, error) { return "access-test", nil }

type disconnectRepository struct {
	provider      string
	ciphertext    []byte
	loadErr       error
	disconnectErr error
	disconnected  bool
}

func (r *disconnectRepository) StoreConnection(context.Context, shared.UserID, string, string, string, []byte, string) (shared.BankConnectionID, error) {
	return "", nil
}
func (r *disconnectRepository) ListConnectionStatus(context.Context, shared.UserID) ([]banking.ConnectionStatus, error) {
	return nil, nil
}
func (r *disconnectRepository) ListAccounts(context.Context, shared.UserID) ([]banking.Account, error) {
	return nil, nil
}
func (r *disconnectRepository) AccountDetail(context.Context, shared.UserID, shared.AccountID) (banking.AccountDetail, error) {
	return banking.AccountDetail{}, nil
}
func (r *disconnectRepository) EnqueueManualSync(context.Context, shared.UserID, shared.BankConnectionID, string) error {
	return nil
}
func (r *disconnectRepository) LoadOwnedConnection(context.Context, shared.UserID, shared.BankConnectionID) (banking.OwnedConnection, error) {
	provider := r.provider
	if provider == "" {
		provider = "PLAID"
	}
	return banking.OwnedConnection{Provider: provider, EncryptedToken: r.ciphertext}, r.loadErr
}
func (r *disconnectRepository) Disconnect(context.Context, shared.UserID, shared.BankConnectionID) error {
	if r.disconnectErr != nil {
		return r.disconnectErr
	}
	r.disconnected = true
	return nil
}
