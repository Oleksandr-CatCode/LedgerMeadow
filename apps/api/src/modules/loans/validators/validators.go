package validators

import (
	"ledgermeadow/src/modules/loans/models"
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid loan")
var ErrInvalidScenario = errors.New("invalid loan scenario")

func Create(command models.Create) error {
	length := len(strings.TrimSpace(command.Name))
	if length < 1 || length > 120 || command.PrincipalRemainingMinor < 0 ||
		command.InterestRateBasisPoints < 0 || command.InterestRateBasisPoints > 100000 ||
		command.MonthlyPaymentMinor <= 0 {
		return ErrInvalid
	}
	if command.Currency != "CAD" && command.Currency != "USD" {
		return ErrInvalid
	}
	return nil
}

func Update(command models.Update) error { return Create(models.Create(command)) }

func Scenario(input models.ScenarioInput) error {
	if input.ExtraMonthlyPaymentMinor < 0 {
		return ErrInvalidScenario
	}
	return nil
}
