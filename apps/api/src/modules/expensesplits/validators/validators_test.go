package validators

import (
	"math"
	"testing"

	"ledgermeadow/src/modules/expensesplits/models"
	shared "ledgermeadow/src/shared/types"
)

func TestReplaceRejectsOverflowAndDuplicateParticipants(t *testing.T) {
	t.Parallel()
	validUserID := "11111111-1111-4111-8111-111111111111"

	tests := []struct {
		name    string
		command models.Replace
	}{
		{
			name: "overflow",
			command: models.Replace{PayerAmountMinor: math.MaxInt64, Participants: []models.Participant{{
				UserID: validUserID, AmountMinor: 1,
			}}},
		},
		{
			name: "duplicate participant",
			command: models.Replace{Participants: []models.Participant{
				{UserID: validUserID, AmountMinor: 1},
				{UserID: validUserID, AmountMinor: 1},
			}},
		},
		{
			name: "non-positive participant share",
			command: models.Replace{Participants: []models.Participant{{
				UserID: validUserID, AmountMinor: shared.MinorUnits(0),
			}}},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := Replace(test.command); err != ErrInvalid {
				t.Fatalf("Replace() error = %v, want %v", err, ErrInvalid)
			}
		})
	}
}

func TestReplaceAcceptsBoundedShares(t *testing.T) {
	t.Parallel()
	command := models.Replace{PayerAmountMinor: 500, Participants: []models.Participant{{
		UserID: "11111111-1111-4111-8111-111111111111", AmountMinor: 500,
	}}}
	if err := Replace(command); err != nil {
		t.Fatalf("Replace() error = %v, want nil", err)
	}
}
