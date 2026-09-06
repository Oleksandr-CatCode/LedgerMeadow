package validators

import (
	"ledgermeadow/src/modules/subscriptions/models"
	"ledgermeadow/src/shared/validate"
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid subscription")

func Create(c models.Create) error {
	n := len(strings.TrimSpace(c.MerchantName))
	if n < 1 || n > 160 || c.ExpectedAmountMinor < 0 || (c.Currency != "CAD" && c.Currency != "USD") {
		return ErrInvalid
	}
	switch c.Frequency {
	case "WEEKLY", "BIWEEKLY", "MONTHLY", "QUARTERLY", "ANNUALLY":
	default:
		return ErrInvalid
	}
	for _, id := range []*string{c.CategoryID, c.SpaceID, c.PaymentAccountID} {
		if id != nil && !validate.UUID(*id) {
			return ErrInvalid
		}
	}
	return nil
}

func Update(command models.Update) error {
	if command.Reclassify != "" {
		if command.Reclassify == "BILL" {
			return nil
		}
		return ErrInvalid
	}
	if err := Create(models.Create{
		MerchantName:        command.MerchantName,
		ExpectedAmountMinor: command.ExpectedAmountMinor,
		Currency:            command.Currency,
		Frequency:           command.Frequency,
		NextExpectedAt:      command.NextExpectedAt,
		CategoryID:          command.CategoryID,
		SpaceID:             command.SpaceID,
		PaymentAccountID:    command.PaymentAccountID,
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
