package validators

import (
	"ledgermeadow/src/modules/loans/models"
	"errors"
	"testing"
)

func TestScenarioRejectsNegativeExtraPayment(t *testing.T) {
	err := Scenario(models.ScenarioInput{ExtraMonthlyPaymentMinor: -1})
	if !errors.Is(err, ErrInvalidScenario) {
		t.Fatalf("Scenario() error = %v, want ErrInvalidScenario", err)
	}
}

func TestScenarioAcceptsZeroAndPositiveExtraPayment(t *testing.T) {
	for _, amount := range []int64{0, 1, 25000} {
		if err := Scenario(models.ScenarioInput{ExtraMonthlyPaymentMinor: amount}); err != nil {
			t.Fatalf("Scenario(%d) error = %v", amount, err)
		}
	}
}
