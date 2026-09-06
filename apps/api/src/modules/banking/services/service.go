package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	bankingrepo "ledgermeadow/src/modules/banking/repository"
	banking "ledgermeadow/src/modules/banking/types"
	"ledgermeadow/src/modules/platform/plaid"
	shared "ledgermeadow/src/shared/types"
)

var (
	ErrInvalidExchange       = errors.New("invalid bank exchange request")
	ErrConnectionConflict    = errors.New("bank connection ownership conflict")
	ErrProviderUnavailable   = errors.New("bank provider unavailable")
	ErrConnectionPersistence = errors.New("bank connection persistence failed")
	ErrUnsupportedOperation  = errors.New("operation is not supported for this connection provider")
)

type Provider interface {
	CreateLinkToken(ctx context.Context, clientUserID string, webhookURL string, redirectURI string) (plaid.LinkToken, error)
	CreateUpdateLinkToken(ctx context.Context, clientUserID string, webhookURL string, redirectURI string, accessToken string) (plaid.LinkToken, error)
	ExchangePublicToken(ctx context.Context, publicToken string) (plaid.ExchangeResult, error)
	RemoveItem(ctx context.Context, accessToken string) error
}

type TokenCipher interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
}

type Repository interface {
	StoreConnection(
		ctx context.Context,
		userID shared.UserID,
		providerItemID string,
		institutionID string,
		institutionName string,
		encryptedToken []byte,
		dedupeKey string,
	) (shared.BankConnectionID, error)
	ListConnectionStatus(ctx context.Context, userID shared.UserID) ([]banking.ConnectionStatus, error)
	ListAccounts(ctx context.Context, userID shared.UserID) ([]banking.Account, error)
	AccountDetail(context.Context, shared.UserID, shared.AccountID) (banking.AccountDetail, error)
	EnqueueManualSync(context.Context, shared.UserID, shared.BankConnectionID, string) error
	LoadOwnedConnection(context.Context, shared.UserID, shared.BankConnectionID) (banking.OwnedConnection, error)
	Disconnect(context.Context, shared.UserID, shared.BankConnectionID) error
}

type Service struct {
	provider    Provider
	cipher      TokenCipher
	repository  Repository
	webhookURL  string
	redirectURI string
	now         func() time.Time
}

func New(provider Provider, cipher TokenCipher, repository Repository, webhookURL string, redirectURI string) *Service {
	return &Service{
		provider: provider, cipher: cipher, repository: repository,
		webhookURL: webhookURL, redirectURI: redirectURI, now: time.Now,
	}
}

func (s *Service) CreateLinkToken(ctx context.Context, userID shared.UserID) (plaid.LinkToken, error) {
	return s.provider.CreateLinkToken(ctx, string(userID), s.webhookURL, s.redirectURI)
}

func (s *Service) CreateUpdateLinkToken(ctx context.Context, userID shared.UserID, connectionID shared.BankConnectionID) (plaid.LinkToken, error) {
	connection, err := s.repository.LoadOwnedConnection(ctx, userID, connectionID)
	if err != nil {
		return plaid.LinkToken{}, err
	}
	if connection.Provider != "PLAID" {
		return plaid.LinkToken{}, ErrUnsupportedOperation
	}
	accessToken, err := s.cipher.Decrypt(connection.EncryptedToken)
	if err != nil {
		return plaid.LinkToken{}, fmt.Errorf("%w: decrypt provider token", ErrConnectionPersistence)
	}
	token, err := s.provider.CreateUpdateLinkToken(ctx, string(userID), s.webhookURL, s.redirectURI, accessToken)
	if err != nil {
		return plaid.LinkToken{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	return token, nil
}

type ExchangeCommand struct {
	PublicToken     string
	InstitutionID   string
	InstitutionName string
}

func (s *Service) Exchange(ctx context.Context, userID shared.UserID, command ExchangeCommand) (shared.BankConnectionID, error) {
	command.PublicToken = strings.TrimSpace(command.PublicToken)
	command.InstitutionID = strings.TrimSpace(command.InstitutionID)
	command.InstitutionName = strings.TrimSpace(command.InstitutionName)
	if command.PublicToken == "" || len(command.PublicToken) > 4096 {
		return "", ErrInvalidExchange
	}
	if len(command.InstitutionID) > 128 || len(command.InstitutionName) > 200 {
		return "", ErrInvalidExchange
	}

	exchange, err := s.provider.ExchangePublicToken(ctx, command.PublicToken)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	encryptedToken, err := s.cipher.Encrypt(exchange.AccessToken)
	if err != nil {
		return "", fmt.Errorf("%w: encrypt provider token", ErrConnectionPersistence)
	}
	digest := sha256.Sum256([]byte(command.PublicToken))
	dedupeKey := "exchange:" + string(userID) + ":" + hex.EncodeToString(digest[:])
	connectionID, err := s.repository.StoreConnection(
		ctx, userID, exchange.ItemID, command.InstitutionID, command.InstitutionName,
		encryptedToken, dedupeKey,
	)
	if err != nil {
		if errors.Is(err, bankingrepo.ErrConnectionOwnershipConflict) {
			return "", ErrConnectionConflict
		}
		return "", fmt.Errorf("%w: %v", ErrConnectionPersistence, err)
	}
	return connectionID, nil
}

func (s *Service) ConnectionStatus(ctx context.Context, userID shared.UserID) ([]banking.ConnectionStatus, error) {
	return s.repository.ListConnectionStatus(ctx, userID)
}

func (s *Service) Accounts(ctx context.Context, userID shared.UserID) ([]banking.Account, error) {
	return s.repository.ListAccounts(ctx, userID)
}

func (s *Service) AccountDetail(ctx context.Context, userID shared.UserID, accountID shared.AccountID) (banking.AccountDetail, error) {
	return s.repository.AccountDetail(ctx, userID, accountID)
}

func (s *Service) Refresh(ctx context.Context, userID shared.UserID, connectionID shared.BankConnectionID) error {
	connection, err := s.repository.LoadOwnedConnection(ctx, userID, connectionID)
	if err != nil {
		return err
	}
	if connection.Provider != "PLAID" {
		return ErrUnsupportedOperation
	}
	dedupeKey := fmt.Sprintf("manual-sync:%s:%s:%d", userID, connectionID, s.now().UTC().Unix()/60)
	return s.repository.EnqueueManualSync(ctx, userID, connectionID, dedupeKey)
}

func (s *Service) Disconnect(ctx context.Context, userID shared.UserID, connectionID shared.BankConnectionID) error {
	connection, err := s.repository.LoadOwnedConnection(ctx, userID, connectionID)
	if err != nil {
		return err
	}
	if connection.Provider == "PLAID" {
		accessToken, decryptErr := s.cipher.Decrypt(connection.EncryptedToken)
		if decryptErr != nil {
			return fmt.Errorf("%w: decrypt provider token", ErrConnectionPersistence)
		}
		if removeErr := s.provider.RemoveItem(ctx, accessToken); removeErr != nil {
			return fmt.Errorf("%w: %v", ErrProviderUnavailable, removeErr)
		}
	} else if connection.Provider != "FILE_IMPORT" {
		return ErrUnsupportedOperation
	}
	if err := s.repository.Disconnect(ctx, userID, connectionID); err != nil {
		if errors.Is(err, bankingrepo.ErrNotFound) {
			return err
		}
		return fmt.Errorf("%w: %v", ErrConnectionPersistence, err)
	}
	return nil
}
