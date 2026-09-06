package validators

import (
	"ledgermeadow/src/modules/bills/models"
	"ledgermeadow/src/shared/validate"
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid bill")

func Create(c models.Create) error {
	n := len(strings.TrimSpace(c.Name))
	if n < 1 || n > 120 || c.ExpectedAmountMinor < 0 || (c.AmountType != "FIXED" && c.AmountType != "VARIABLE") || (c.Currency != "CAD" && c.Currency != "USD") {
		return ErrInvalid
	}
	switch c.Frequency {
	case "WEEKLY", "BIWEEKLY", "MONTHLY", "QUARTERLY", "ANNUALLY":
	default:
		return ErrInvalid
	}
	if c.CategoryID != nil && !validate.UUID(*c.CategoryID) {
		return ErrInvalid
	}
	if c.SpaceID != nil && !validate.UUID(*c.SpaceID) {
		return ErrInvalid
	}
	return nil
}

func Update(command models.Update) error {
	if command.Reclassify != "" {
		if command.Reclassify == "SUBSCRIPTION" {
			return nil
		}
		return ErrInvalid
	}
	if err := Create(models.Create{
		Name: command.Name, AmountType: command.AmountType,
		ExpectedAmountMinor: command.ExpectedAmountMinor, Currency: command.Currency,
		Frequency: command.Frequency, NextDueAt: command.NextDueAt,
		CategoryID: command.CategoryID, SpaceID: command.SpaceID,
	}); err != nil {
		return err
	}
	switch command.Status {
	case "ACTIVE", "PAUSED", "CANCELLED", "UNKNOWN":
		return nil
	default:
		return ErrInvalid
	}
}
