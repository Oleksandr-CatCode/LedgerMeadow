package types

import (
	"encoding/json"
	"errors"
	"strconv"
)

type MinorUnits int64

func (amount MinorUnits) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(strconv.FormatInt(int64(amount), 10))), nil
}

func (amount *MinorUnits) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("minor units must be a decimal string")
	}
	digits := value
	if len(digits) > 0 && digits[0] == '-' {
		digits = digits[1:]
	}
	if digits == "" {
		return errors.New("minor units must match the decimal-string contract")
	}
	for _, character := range digits {
		if character < '0' || character > '9' {
			return errors.New("minor units must match the decimal-string contract")
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return errors.New("minor units are outside the signed 64-bit range")
	}
	*amount = MinorUnits(parsed)
	return nil
}
