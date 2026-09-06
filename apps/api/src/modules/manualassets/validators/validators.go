package validators

import (
	"ledgermeadow/src/modules/manualassets/models"
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid manual asset")

func Create(c models.Create) error {
	n := len(strings.TrimSpace(c.Name))
	if n < 1 || n > 120 || c.ValueMinor < 0 || (c.Currency != "CAD" && c.Currency != "USD") {
		return ErrInvalid
	}
	switch c.Type {
	case "PROPERTY", "VEHICLE", "INVESTMENT", "CASH", "OTHER":
		return nil
	default:
		return ErrInvalid
	}
}
