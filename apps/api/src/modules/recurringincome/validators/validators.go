package validators

import (
	"errors"
	"strings"

	"ledgermeadow/src/modules/recurringincome/models"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalid = errors.New("invalid recurring income")

func Update(command models.Update) error {
	nameLength := len(strings.TrimSpace(command.Name))
	if nameLength < 1 || nameLength > 160 || command.ExpectedAmountMinor <= 0 ||
		(command.Currency != "CAD" && command.Currency != "USD") {
		return ErrInvalid
	}
	switch command.Frequency {
	case "WEEKLY", "BIWEEKLY", "MONTHLY", "QUARTERLY", "ANNUALLY":
	default:
		return ErrInvalid
	}
	switch command.Status {
	case "ACTIVE", "PAUSED", "CANCELLED", "UNKNOWN":
	default:
		return ErrInvalid
	}
	for _, id := range []*string{command.CategoryID, command.PaymentAccountID} {
		if id != nil && *id != "" && !validate.UUID(*id) {
			return ErrInvalid
		}
	}
	return nil
}
