package validators

import (
	"errors"
	"strings"

	"ledgermeadow/src/modules/spaces/models"
)

var ErrInvalid = errors.New("invalid space")

func Create(command models.Create) error {
	length := len(strings.TrimSpace(command.Name))
	if length < 1 || length > 120 || command.MonthlyAllocationMinor < 0 {
		return ErrInvalid
	}
	switch command.Type {
	case "DAILY", "BILLS", "HOUSEHOLD", "SAVINGS", "GOAL", "CUSTOM":
	default:
		return ErrInvalid
	}
	if command.Currency != "CAD" && command.Currency != "USD" {
		return ErrInvalid
	}
	if command.Visibility != "PRIVATE" && command.Visibility != "HOUSEHOLD" {
		return ErrInvalid
	}
	return nil
}
