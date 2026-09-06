package validators

import (
	"errors"

	"ledgermeadow/src/shared/validate"
)

var ErrInvalid = errors.New("invalid notification preference")

func Preference(t string) error {
	switch t {
	case "BUDGET_75", "BUDGET_90", "BUDGET_EXCEEDED", "BILL_3_DAYS", "BILL_1_DAY", "BILL_AMOUNT_CHANGED", "NEW_RECURRING", "SUBSCRIPTION_PRICE_CHANGE", "UNUSUAL_CHARGE", "AVAILABLE_BELOW_THRESHOLD", "PROJECTED_NEGATIVE", "NEW_SHARED_EXPENSE", "SPLIT_REQUEST":
		return nil
	default:
		return ErrInvalid
	}
}

func NotificationID(id string) error {
	if !validate.UUID(id) {
		return ErrInvalid
	}
	return nil
}
