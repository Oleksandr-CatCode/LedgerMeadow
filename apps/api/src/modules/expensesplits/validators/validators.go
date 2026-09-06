package validators

import (
	"errors"
	"math"

	"ledgermeadow/src/modules/expensesplits/models"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalid = errors.New("invalid expense split")

func Replace(command models.Replace) error {
	if command.PayerAmountMinor < 0 || len(command.Participants) < 1 || len(command.Participants) > 20 {
		return ErrInvalid
	}
	seen := make(map[string]struct{}, len(command.Participants))
	total := command.PayerAmountMinor
	for _, participant := range command.Participants {
		if !validate.UUID(participant.UserID) || participant.AmountMinor <= 0 {
			return ErrInvalid
		}
		if _, exists := seen[participant.UserID]; exists {
			return ErrInvalid
		}
		seen[participant.UserID] = struct{}{}
		amount := int64(participant.AmountMinor)
		if total > math.MaxInt64-amount {
			return ErrInvalid
		}
		total += amount
	}
	return nil
}
