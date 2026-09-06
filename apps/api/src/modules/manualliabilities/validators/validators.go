package validators

import (
	"errors"
	"strings"

	"ledgermeadow/src/modules/manualliabilities/models"
)

var ErrInvalid = errors.New("invalid manual liability")

func Create(command models.Create) error {
	length := len(strings.TrimSpace(command.Name))
	if length < 1 || length > 120 || command.BalanceMinor < 0 {
		return ErrInvalid
	}
	switch command.Type {
	case "LOAN", "MORTGAGE", "CREDIT_CARD", "OTHER":
	default:
		return ErrInvalid
	}
	if command.Currency != "CAD" && command.Currency != "USD" {
		return ErrInvalid
	}
	return nil
}
