package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ledgermeadow/src/modules/transactionimports/models"
	importrepo "ledgermeadow/src/modules/transactionimports/repository"
	shared "ledgermeadow/src/shared/types"
)

const syntheticRBCFile = `Account Type,Account Number,Transaction Date,Cheque Number,Description 1,Description 2,CAD$,USD$
Chequing,00000-0000000,1/3/2000,,SYNTHETIC SERVICE DEBIT,SYNTHETIC ROW NOTE,-0.03,,`

func TestPreviewAutomaticallyMatchesExistingAccount(t *testing.T) {
	t.Parallel()
	repository := &recordingRepository{match: models.AccountMatch{
		Exists: true, AccountName: "Synthetic Checking Import",
	}}
	service := New(repository)

	preview, err := service.Preview(context.Background(), "synthetic-import-actor", strings.NewReader(syntheticRBCFile))
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if !preview.ExistingAccount || preview.AccountName != "Synthetic Checking Import" || repository.matchedMask != "0000" {
		t.Fatalf("preview = %#v, matched mask = %q", preview, repository.matchedMask)
	}
}

func TestImportGeneratesAccountNameAndAllowsExistingAccountWithoutBalance(t *testing.T) {
	t.Parallel()
	repository := &recordingRepository{result: models.ImportResult{ImportedCount: 1}}
	service := New(repository)

	result, err := service.Import(context.Background(), "synthetic-import-actor", nil, strings.NewReader(syntheticRBCFile))
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.ImportedCount != 1 || repository.command.AccountName != "RBC Chequing •••• 0000" || repository.command.CurrentBalanceMinor != nil {
		t.Fatalf("result = %#v, command = %#v", result, repository.command)
	}
}

func TestImportMapsMissingFirstBalanceToInvalidImport(t *testing.T) {
	t.Parallel()
	repository := &recordingRepository{importErr: importrepo.ErrCurrentBalanceRequired}
	service := New(repository)

	_, err := service.Import(context.Background(), "synthetic-import-actor", nil, strings.NewReader(syntheticRBCFile))
	if !errors.Is(err, ErrInvalidImport) {
		t.Fatalf("Import() error = %v, want ErrInvalidImport", err)
	}
}

type recordingRepository struct {
	match       models.AccountMatch
	matchErr    error
	matchedMask string
	command     models.ImportCommand
	result      models.ImportResult
	importErr   error
}

func (r *recordingRepository) Match(_ context.Context, _ shared.UserID, file models.ParsedFile) (models.AccountMatch, error) {
	r.matchedMask = file.Mask
	return r.match, r.matchErr
}

func (r *recordingRepository) Import(_ context.Context, _ shared.UserID, command models.ImportCommand) (models.ImportResult, error) {
	r.command = command
	return r.result, r.importErr
}
