package validators

import (
	"errors"
	"testing"

	"ledgermeadow/src/modules/subscriptions/models"
)

func TestUpdateValidatesReclassificationTarget(t *testing.T) {
	if err := Update(models.Update{Reclassify: "BILL"}); err != nil {
		t.Fatalf("valid reclassification target: %v", err)
	}
	if err := Update(models.Update{Reclassify: "BILL' OR 1=1 --"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("injection-shaped reclassification error = %v, want ErrInvalid", err)
	}
}
