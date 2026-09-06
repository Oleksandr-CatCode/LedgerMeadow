package validators

import (
	"ledgermeadow/src/modules/budgets/models"
	"ledgermeadow/src/shared/validate"
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid budget")

func Create(c models.Create) error {
	n := len(strings.TrimSpace(c.Name))
	if n < 1 || n > 120 || c.LimitMinor <= 0 || (c.Currency != "CAD" && c.Currency != "USD") || c.WarningThreshold < 1 || c.WarningThreshold > 100 || c.CriticalThreshold < c.WarningThreshold || c.CriticalThreshold > 100 {
		return ErrInvalid
	}
	if c.Period != "WEEKLY" && c.Period != "MONTHLY" && c.Period != "ANNUAL" {
		return ErrInvalid
	}
	if (c.CategoryID == nil) == (c.SpaceID == nil) {
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
