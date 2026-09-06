package services

import (
	"context"
	"encoding/json"
	"testing"

	"ledgermeadow/src/modules/dashboard/models"
	shared "ledgermeadow/src/shared/types"
)

type emptyDashboardRepository struct{}

func (emptyDashboardRepository) Get(context.Context, shared.UserID) (models.Dashboard, error) {
	return models.Dashboard{}, nil
}

func TestGetSerializesEmptyCollectionsAsArrays(t *testing.T) {
	service := New(emptyDashboardRepository{})
	dashboard, err := service.Get(context.Background(), shared.UserID("user-id"))
	if err != nil {
		t.Fatalf("get dashboard: %v", err)
	}

	encoded, err := json.Marshal(dashboard)
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}

	var response map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("unmarshal dashboard response: %v", err)
	}
	for _, field := range []string{"projections", "spaces", "budgets", "monthly_plans", "upcoming", "recent_transactions"} {
		if string(response[field]) != "[]" {
			t.Errorf("%s = %s, want []", field, response[field])
		}
	}
}
