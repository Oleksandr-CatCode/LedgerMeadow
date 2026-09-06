package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ledgermeadow/src/modules/subscriptions/models"
	"ledgermeadow/src/modules/subscriptions/validators"
	shared "ledgermeadow/src/shared/types"
)

type rejectingUpdateService struct{}

func (rejectingUpdateService) List(context.Context, shared.UserID) ([]models.Subscription, error) {
	return nil, nil
}

func (rejectingUpdateService) Detail(context.Context, shared.UserID, string) (models.Detail, error) {
	return models.Detail{}, nil
}

func (rejectingUpdateService) Create(context.Context, shared.UserID, models.Create) (string, error) {
	return "", nil
}

func (rejectingUpdateService) Update(context.Context, shared.UserID, string, models.Update) error {
	return validators.ErrInvalid
}

func TestUpdateRejectsMalformedReclassificationWithoutEchoingInput(t *testing.T) {
	malformed := "BILL' OR 1=1 --"
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/subscriptions/00000000-0000-4000-8000-000000000001",
		strings.NewReader(`{"reclassify":"`+malformed+`"}`),
	)
	request.SetPathValue("id", "00000000-0000-4000-8000-000000000001")
	response := httptest.NewRecorder()
	New(rejectingUpdateService{}).Update(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed reclassification status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if strings.Contains(response.Body.String(), malformed) {
		t.Fatal("malformed reclassification input was echoed in the response")
	}
}
