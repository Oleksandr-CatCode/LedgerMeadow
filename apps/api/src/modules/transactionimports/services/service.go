package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"ledgermeadow/src/modules/transactionimports/models"
	importrepo "ledgermeadow/src/modules/transactionimports/repository"
	"ledgermeadow/src/modules/transactionimports/validators"
	shared "ledgermeadow/src/shared/types"
)

var ErrInvalidImport = errors.New("invalid transaction import")
var ErrAccountConflict = errors.New("imported account identity is ambiguous")

type Repository interface {
	Match(context.Context, shared.UserID, models.ParsedFile) (models.AccountMatch, error)
	Import(context.Context, shared.UserID, models.ImportCommand) (models.ImportResult, error)
}

type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) Preview(ctx context.Context, userID shared.UserID, reader io.Reader) (models.Preview, error) {
	parsed, err := validators.ParseRBC(reader)
	if err != nil {
		return models.Preview{}, fmt.Errorf("%w: %v", ErrInvalidImport, err)
	}
	match, err := s.repository.Match(ctx, userID, parsed)
	if errors.Is(err, importrepo.ErrAccountConflict) {
		return models.Preview{}, ErrAccountConflict
	}
	if err != nil {
		return models.Preview{}, fmt.Errorf("match imported account: %w", err)
	}
	accountName := defaultAccountName(parsed)
	if match.Exists {
		accountName = match.AccountName
	}
	return models.Preview{
		ParsedFile: parsed, AccountName: accountName, ExistingAccount: match.Exists,
	}, nil
}

func (s *Service) Import(ctx context.Context, userID shared.UserID, currentBalanceMinor *int64, reader io.Reader) (models.ImportResult, error) {
	if currentBalanceMinor != nil && *currentBalanceMinor < 0 {
		return models.ImportResult{}, ErrInvalidImport
	}
	parsed, err := validators.ParseRBC(reader)
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("%w: %v", ErrInvalidImport, err)
	}
	connectionOpaqueID, err := opaqueID()
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("generate connection identifier: %w", err)
	}
	accountOpaqueID, err := opaqueID()
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("generate account identifier: %w", err)
	}
	batchOpaqueID, err := opaqueID()
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("generate import identifier: %w", err)
	}
	result, err := s.repository.Import(ctx, userID, models.ImportCommand{
		AccountName: defaultAccountName(parsed), CurrentBalanceMinor: currentBalanceMinor,
		ConnectionOpaqueID: "file_" + connectionOpaqueID,
		AccountOpaqueID:    "file_" + accountOpaqueID,
		BatchOpaqueID:      batchOpaqueID, File: parsed,
	})
	if errors.Is(err, importrepo.ErrAccountConflict) {
		return models.ImportResult{}, ErrAccountConflict
	}
	if errors.Is(err, importrepo.ErrCurrentBalanceRequired) {
		return models.ImportResult{}, ErrInvalidImport
	}
	if err != nil {
		return models.ImportResult{}, fmt.Errorf("persist RBC CSV import: %w", err)
	}
	return result, nil
}

func defaultAccountName(file models.ParsedFile) string {
	label := "Chequing"
	if file.AccountType == models.AccountTypeVisa {
		label = "Visa"
	}
	return fmt.Sprintf("RBC %s •••• %s", label, file.Mask)
}

func opaqueID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
