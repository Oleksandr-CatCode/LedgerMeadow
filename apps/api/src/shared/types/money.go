package types

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode"
)

var ErrUnsupportedCurrency = errors.New("unsupported currency")

type Currency string

const (
	CurrencyCAD Currency = "CAD"
	CurrencyUSD Currency = "USD"
)

type Money struct {
	AmountMinor int64
	Currency    Currency
}

func ParseProviderDecimal(value string, currencyCode string) (Money, error) {
	currency, exponent, err := supportedCurrency(currencyCode)
	if err != nil {
		return Money{}, err
	}

	amountMinor, err := decimalToScaledInteger(value, exponent, false)
	if err != nil {
		return Money{}, fmt.Errorf("parse provider amount: %w", err)
	}

	return Money{AmountMinor: amountMinor, Currency: currency}, nil
}

// ParseProviderAccountBalance converts a provider balance to currency minor
// units using round-half-to-even when the provider supplies finer precision.
// Transaction amounts intentionally use ParseProviderDecimal and remain exact.
func ParseProviderAccountBalance(value string, currencyCode string) (Money, error) {
	currency, exponent, err := supportedCurrency(currencyCode)
	if err != nil {
		return Money{}, err
	}

	amountMinor, err := decimalToScaledInteger(value, exponent, true)
	if err != nil {
		return Money{}, fmt.Errorf("parse provider account balance: %w", err)
	}

	return Money{AmountMinor: amountMinor, Currency: currency}, nil
}

func supportedCurrency(code string) (Currency, int, error) {
	switch Currency(strings.ToUpper(code)) {
	case CurrencyCAD:
		return CurrencyCAD, 2, nil
	case CurrencyUSD:
		return CurrencyUSD, 2, nil
	default:
		return "", 0, fmt.Errorf("%w: %s", ErrUnsupportedCurrency, code)
	}
}

func decimalToScaledInteger(value string, targetScale int, roundHalfEven bool) (int64, error) {
	if value == "" {
		return 0, errors.New("empty decimal")
	}

	sign := 1
	if value[0] == '-' {
		sign = -1
		value = value[1:]
	} else if value[0] == '+' {
		return 0, errors.New("leading plus is not valid JSON number syntax")
	}

	mantissa, exponent, err := splitExponent(value)
	if err != nil {
		return 0, err
	}

	parts := strings.Split(mantissa, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid decimal mantissa")
	}
	whole := parts[0]
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" {
			return 0, errors.New("empty decimal fraction")
		}
	}
	if !allDigits(whole) || !allDigits(fraction) {
		return 0, errors.New("decimal contains a non-digit")
	}
	if len(whole) > 1 && whole[0] == '0' {
		return 0, errors.New("decimal has a leading zero")
	}

	digits := whole + fraction
	effectiveScale := len(fraction) - exponent
	coefficient := new(big.Int)
	if _, ok := coefficient.SetString(digits, 10); !ok {
		return 0, errors.New("invalid decimal digits")
	}

	shift := targetScale - effectiveScale
	if shift >= 0 {
		coefficient.Mul(coefficient, powerOfTen(shift))
	} else {
		divisor := powerOfTen(-shift)
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(coefficient, divisor, remainder)
		if remainder.Sign() != 0 {
			if !roundHalfEven {
				return 0, errors.New("amount cannot be represented in currency minor units without rounding")
			}
			twiceRemainder := new(big.Int).Lsh(new(big.Int).Set(remainder), 1)
			comparison := twiceRemainder.Cmp(divisor)
			if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
				quotient.Add(quotient, big.NewInt(1))
			}
		}
		coefficient = quotient
	}

	if sign < 0 {
		coefficient.Neg(coefficient)
	}
	if !coefficient.IsInt64() {
		return 0, errors.New("minor-unit amount exceeds int64")
	}
	return coefficient.Int64(), nil
}

func powerOfTen(exponent int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
}

func splitExponent(value string) (string, int, error) {
	index := strings.IndexAny(value, "eE")
	if index < 0 {
		return value, 0, nil
	}
	if strings.IndexAny(value[index+1:], "eE") >= 0 {
		return "", 0, errors.New("multiple decimal exponents")
	}
	mantissa := value[:index]
	exponentText := value[index+1:]
	if exponentText == "" {
		return "", 0, errors.New("empty decimal exponent")
	}
	exponent, err := strconv.Atoi(exponentText)
	if err != nil || exponent < -18 || exponent > 18 {
		return "", 0, errors.New("decimal exponent is invalid or out of bounds")
	}
	return mantissa, exponent, nil
}

func allDigits(value string) bool {
	for _, character := range value {
		if !unicode.IsDigit(character) || character > unicode.MaxASCII {
			return false
		}
	}
	return true
}
