package validators

import (
	"errors"
	"strings"
	"time"

	"ledgermeadow/src/modules/goals/models"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalid = errors.New("invalid goal")

func Create(command models.Create, today time.Time) error {
	nameLength := len(strings.TrimSpace(command.Name))
	if nameLength < 1 || nameLength > 120 || command.TargetMinor <= 0 || command.CurrentMinor < 0 || command.CurrentMinor > command.TargetMinor {
		return ErrInvalid
	}
	if command.Currency != "CAD" && command.Currency != "USD" {
		return ErrInvalid
	}
	if command.TargetDate != nil && command.TargetDate.Before(today) {
		return ErrInvalid
	}
	if command.SpaceID != nil && !validate.UUID(*command.SpaceID) {
		return ErrInvalid
	}
	return nil
}
