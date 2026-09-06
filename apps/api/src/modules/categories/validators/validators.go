package validators

import (
	"errors"
	"strings"

	"ledgermeadow/src/modules/categories/models"
	"ledgermeadow/src/shared/validate"
)

var ErrInvalid = errors.New("invalid category")

func Create(command models.Create) error {
	length := len(strings.TrimSpace(command.Name))
	if length < 1 || length > 100 {
		return ErrInvalid
	}
	if command.Type != "INCOME" && command.Type != "EXPENSE" && command.Type != "TRANSFER" {
		return ErrInvalid
	}
	if command.ParentID != nil && !validate.UUID(*command.ParentID) {
		return ErrInvalid
	}
	return nil
}
