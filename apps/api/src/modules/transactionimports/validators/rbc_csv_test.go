package validators

import (
	"errors"
	"os"
	"strings"
	"testing"

	"ledgermeadow/src/modules/transactionimports/models"
	shared "ledgermeadow/src/shared/types"
)

const syntheticHeader = "Account Type,Account Number,Transaction Date,Cheque Number,Description 1,Description 2,CAD$,USD$"

func TestParseRBCVisaPreservesSignedAmountsAndMasksAccount(t *testing.T) {
	t.Parallel()
	input := syntheticHeader + "\n" +
		"Visa,0000000000000000,1/1/2000,,SYNTHETIC CREDIT,,1.01,\n" +
		"Visa,0000000000000000,1/2/2000,,SYNTHETIC DEBIT,,-0.02,"

	parsed, err := ParseRBC(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseRBC() error = %v", err)
	}
	if parsed.AccountType != models.AccountTypeVisa || parsed.Mask != "0000" || parsed.Currency != shared.CurrencyCAD {
		t.Fatalf("parsed account summary = %#v", parsed)
	}
	if len(parsed.Transactions) != 2 || parsed.Transactions[0].AmountMinor != 101 || parsed.Transactions[1].AmountMinor != -2 {
		t.Fatalf("parsed transactions = %#v", parsed.Transactions)
	}
	if parsed.PositiveTotalMinor != 101 || parsed.OutflowTotalMinor != 2 {
		t.Fatalf("parsed totals = positive %d, outflow %d", parsed.PositiveTotalMinor, parsed.OutflowTotalMinor)
	}
}

func TestParseRBCSyntheticTrailingEmptyFieldFixture(t *testing.T) {
	t.Parallel()
	file, err := os.Open("testdata/rbc_chequing.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	parsed, err := ParseRBC(file)
	if err != nil {
		t.Fatalf("ParseRBC() error = %v", err)
	}
	if parsed.AccountType != models.AccountTypeChequing || parsed.Mask != "0042" || len(parsed.Transactions) != 2 {
		t.Fatalf("parsed fixture = %#v", parsed)
	}
}

func TestParseRBCKeepsIdenticalRowsDistinctButStable(t *testing.T) {
	t.Parallel()
	row := "Chequing,00000-0000000,1/3/2000,,SYNTHETIC DUPLICATE,,-0.03,"
	input := syntheticHeader + "\n" + row + "\n" + row
	first, err := ParseRBC(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseRBC(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if first.Transactions[0].Fingerprint == first.Transactions[1].Fingerprint {
		t.Fatal("identical source rows did not receive occurrence-specific fingerprints")
	}
	if first.Transactions[0].Fingerprint != second.Transactions[0].Fingerprint || first.Transactions[1].Fingerprint != second.Transactions[1].Fingerprint {
		t.Fatal("fingerprints changed between identical parses")
	}
}

func TestParseRBCRejectsMultipleAccountsAndCurrencyColumns(t *testing.T) {
	t.Parallel()
	tests := []string{
		syntheticHeader + "\n" +
			"Chequing,00000-0000000,1/4/2000,,SYNTHETIC ACCOUNT A,,-0.04,\n" +
			"Chequing,00000-0000001,1/4/2000,,SYNTHETIC ACCOUNT B,,-0.05,",
		syntheticHeader + "\n" +
			"Chequing,00000-0000000,1/5/2000,,SYNTHETIC DUAL CURRENCY,,-0.06,-0.07",
	}
	for _, input := range tests {
		if _, err := ParseRBC(strings.NewReader(input)); !errors.Is(err, ErrInvalidFile) {
			t.Fatalf("ParseRBC() error = %v, want ErrInvalidFile", err)
		}
	}
}
