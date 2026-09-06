package types

import (
	"errors"
	"testing"
)

func TestParseProviderDecimal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		currency string
		want     int64
		wantErr  error
	}{
		{name: "whole CAD", value: "5000", currency: "CAD", want: 500000},
		{name: "cents", value: "12.34", currency: "USD", want: 1234},
		{name: "negative", value: "-0.05", currency: "CAD", want: -5},
		{name: "exponent", value: "1.2e2", currency: "USD", want: 12000},
		{name: "exact extra zero", value: "1.230", currency: "CAD", want: 123},
		{name: "rounding rejected", value: "1.234", currency: "CAD", wantErr: errors.New("rounding")},
		{name: "unsupported currency", value: "1", currency: "JPY", wantErr: ErrUnsupportedCurrency},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			money, err := ParseProviderDecimal(test.value, test.currency)
			if test.wantErr != nil {
				if err == nil {
					t.Fatalf("expected an error")
				}
				if errors.Is(test.wantErr, ErrUnsupportedCurrency) && !errors.Is(err, ErrUnsupportedCurrency) {
					t.Fatalf("expected unsupported currency error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseProviderDecimal() error = %v", err)
			}
			if money.AmountMinor != test.want {
				t.Fatalf("amount = %d, want %d", money.AmountMinor, test.want)
			}
		})
	}
}

func TestParseProviderAccountBalanceRoundsHalfEven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  int64
	}{
		{name: "below midpoint", value: "10.0049", want: 1000},
		{name: "midpoint keeps even", value: "10.005", want: 1000},
		{name: "midpoint increments odd", value: "10.015", want: 1002},
		{name: "above midpoint", value: "10.0051", want: 1001},
		{name: "negative midpoint keeps even", value: "-10.005", want: -1000},
		{name: "negative midpoint increments odd magnitude", value: "-10.015", want: -1002},
		{name: "sub-cent rounds to zero", value: "0.001", want: 0},
		{name: "scientific notation", value: "1.0015e1", want: 1002},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			money, err := ParseProviderAccountBalance(test.value, "CAD")
			if err != nil {
				t.Fatalf("ParseProviderAccountBalance() error = %v", err)
			}
			if money.AmountMinor != test.want {
				t.Fatalf("amount = %d, want %d", money.AmountMinor, test.want)
			}
		})
	}
}
