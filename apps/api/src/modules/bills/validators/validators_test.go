package validators

import (
	"errors"
	"testing"

	"ledgermeadow/src/modules/bills/models"
)

func TestUpdateValidatesReclassificationTarget(t *testing.T) {
	if err := Update(models.Update{Reclassify: "SUBSCRIPTION"}); err != nil {
		t.Fatalf("valid reclassification target: %v", err)
	}
	if err := Update(models.Update{Reclassify: "SUBSCRIPTION; DROP TABLE bills"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("injection-shaped reclassification error = %v, want ErrInvalid", err)
	}
}
