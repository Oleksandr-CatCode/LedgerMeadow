package validators

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode"

	"ledgermeadow/src/modules/transactionimports/models"
	shared "ledgermeadow/src/shared/types"
)

const MaxFileBytes int64 = 2 << 20
const MaxRows = 10_000

var ErrInvalidFile = errors.New("invalid RBC CSV file")

var expectedHeader = []string{
	"Account Type", "Account Number", "Transaction Date", "Cheque Number",
	"Description 1", "Description 2", "CAD$", "USD$",
}

func ParseRBC(reader io.Reader) (models.ParsedFile, error) {
	limited := io.LimitReader(reader, MaxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return models.ParsedFile{}, invalid("read file")
	}
	if int64(len(data)) > MaxFileBytes {
		return models.ParsedFile{}, invalid("file exceeds 2 MiB")
	}
	parser := csv.NewReader(strings.NewReader(string(data)))
	parser.FieldsPerRecord = -1
	parser.ReuseRecord = false
	header, err := parser.Read()
	if err != nil {
		return models.ParsedFile{}, invalid("missing header")
	}
	if len(header) != len(expectedHeader) {
		return models.ParsedFile{}, invalid("unexpected columns")
	}
	header[0] = strings.TrimPrefix(header[0], "\ufeff")
	for index, expected := range expectedHeader {
		if strings.TrimSpace(header[index]) != expected {
			return models.ParsedFile{}, invalid("unexpected columns")
		}
	}

	var result models.ParsedFile
	var accountNumber string
	occurrences := make(map[string]int)
	for rowNumber := 2; ; rowNumber++ {
		record, readErr := parser.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return models.ParsedFile{}, invalid(fmt.Sprintf("malformed row %d", rowNumber))
		}
		if len(record) == len(expectedHeader)+1 && strings.TrimSpace(record[len(expectedHeader)]) == "" {
			record = record[:len(expectedHeader)]
		}
		if len(record) != len(expectedHeader) {
			return models.ParsedFile{}, invalid(fmt.Sprintf("unexpected columns on row %d", rowNumber))
		}
		if allBlank(record) {
			continue
		}
		if len(result.Transactions) >= MaxRows {
			return models.ParsedFile{}, invalid("file exceeds 10000 transactions")
		}

		accountType, err := normalizeAccountType(record[0])
		if err != nil {
			return models.ParsedFile{}, invalid(fmt.Sprintf("unsupported account type on row %d", rowNumber))
		}
		number := strings.TrimSpace(record[1])
		mask, err := accountMask(number)
		if err != nil {
			return models.ParsedFile{}, invalid(fmt.Sprintf("invalid account number on row %d", rowNumber))
		}
		if len(result.Transactions) == 0 {
			result.AccountType = accountType
			result.Mask = mask
			accountNumber = number
		} else if result.AccountType != accountType || accountNumber != number {
			return models.ParsedFile{}, invalid("file contains more than one account")
		}

		date, err := time.Parse("1/2/2006", strings.TrimSpace(record[2]))
		if err != nil {
			return models.ParsedFile{}, invalid(fmt.Sprintf("invalid date on row %d", rowNumber))
		}
		cheque := boundedText(record[3], 100)
		name := boundedText(record[4], 500)
		merchantText := boundedText(record[5], 500)
		if name == "" {
			name = merchantText
		}
		if name == "" {
			return models.ParsedFile{}, invalid(fmt.Sprintf("missing description on row %d", rowNumber))
		}
		if tooLong(record[3], 100) || tooLong(record[4], 500) || tooLong(record[5], 500) {
			return models.ParsedFile{}, invalid(fmt.Sprintf("text is too long on row %d", rowNumber))
		}

		money, err := parseAmount(record[6], record[7])
		if err != nil {
			return models.ParsedFile{}, invalid(fmt.Sprintf("invalid amount on row %d", rowNumber))
		}
		if len(result.Transactions) == 0 {
			result.Currency = money.Currency
		} else if result.Currency != money.Currency {
			return models.ParsedFile{}, invalid("file contains more than one currency")
		}
		if money.AmountMinor == math.MinInt64 {
			return models.ParsedFile{}, invalid(fmt.Sprintf("amount is out of range on row %d", rowNumber))
		}

		original := name
		var merchant *string
		if merchantText != "" {
			merchant = &merchantText
			if merchantText != name {
				original += " — " + merchantText
			}
		}
		base := strings.Join([]string{
			date.Format("2006-01-02"), cheque, name, merchantText,
			fmt.Sprintf("%d", money.AmountMinor), string(money.Currency),
		}, "\x1f")
		occurrences[base]++
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x1f%d", base, occurrences[base])))
		result.Transactions = append(result.Transactions, models.Transaction{
			Date: date, ChequeNumber: cheque, Name: name, MerchantName: merchant,
			OriginalDescription: original, AmountMinor: money.AmountMinor,
			Fingerprint: hex.EncodeToString(digest[:]),
		})
		if len(result.Transactions) == 1 || date.Before(result.DateFrom) {
			result.DateFrom = date
		}
		if len(result.Transactions) == 1 || date.After(result.DateTo) {
			result.DateTo = date
		}
		if money.AmountMinor > 0 {
			if result.PositiveTotalMinor > math.MaxInt64-money.AmountMinor {
				return models.ParsedFile{}, invalid("positive total is out of range")
			}
			result.PositiveTotalMinor += money.AmountMinor
		} else {
			outflow := -money.AmountMinor
			if result.OutflowTotalMinor > math.MaxInt64-outflow {
				return models.ParsedFile{}, invalid("outflow total is out of range")
			}
			result.OutflowTotalMinor += outflow
		}
	}
	if len(result.Transactions) == 0 {
		return models.ParsedFile{}, invalid("file contains no transactions")
	}
	return result, nil
}

func normalizeAccountType(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "VISA":
		return models.AccountTypeVisa, nil
	case "CHEQUING":
		return models.AccountTypeChequing, nil
	default:
		return "", ErrInvalidFile
	}
}

func accountMask(value string) (string, error) {
	digits := make([]rune, 0, len(value))
	for _, character := range value {
		if unicode.IsDigit(character) && character <= unicode.MaxASCII {
			digits = append(digits, character)
		} else if character != '-' && character != ' ' {
			return "", ErrInvalidFile
		}
	}
	if len(digits) < 4 || len(digits) > 24 {
		return "", ErrInvalidFile
	}
	return string(digits[len(digits)-4:]), nil
}

func parseAmount(cad string, usd string) (shared.Money, error) {
	cad = strings.TrimSpace(cad)
	usd = strings.TrimSpace(usd)
	if (cad == "") == (usd == "") {
		return shared.Money{}, ErrInvalidFile
	}
	if cad != "" {
		return shared.ParseProviderDecimal(cad, "CAD")
	}
	return shared.ParseProviderDecimal(usd, "USD")
}

func boundedText(value string, _ int) string { return strings.TrimSpace(value) }
func tooLong(value string, maximum int) bool { return len([]rune(strings.TrimSpace(value))) > maximum }
func allBlank(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
func invalid(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidFile, reason) }
