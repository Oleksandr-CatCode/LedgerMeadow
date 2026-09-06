package validators

import (
	"ledgermeadow/src/shared/validate"
	"errors"
)

var ErrInvalid = errors.New("invalid inbox resolution")

func Resolve(id, resolution string) error {
	if !validate.UUID(id) {
		return ErrInvalid
	}
	switch resolution {
	case "CONFIRMED", "DISMISSED", "NOT_RECURRING", "LOOKS_CORRECT":
		return nil
	default:
		return ErrInvalid
	}
}
