package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/recurringincome/models"
	recurringincomerepo "ledgermeadow/src/modules/recurringincome/repository"
	"ledgermeadow/src/modules/recurringincome/services"
	"ledgermeadow/src/modules/recurringincome/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.RecurringIncome, error)
	Detail(context.Context, shared.UserID, string) (models.RecurringIncome, error)
	Update(context.Context, shared.UserID, string, models.Update) error
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	items, err := h.service.List(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "Recurring income could not be loaded.")
		return
	}
	httpx.WriteJSON(response, 200, map[string]any{"recurring_income": items})
}

func (h *Handler) Detail(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	item, err := h.service.Detail(request.Context(), userID, request.PathValue("id"))
	if errors.Is(err, services.ErrInvalidID) {
		httpx.WriteError(response, 400, "INVALID_RECURRING_INCOME_ID", "The recurring income identifier is invalid.")
		return
	}
	if errors.Is(err, recurringincomerepo.ErrNotFound) {
		httpx.WriteError(response, 404, "RECURRING_INCOME_NOT_FOUND", "The recurring income was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "Recurring income could not be loaded.")
		return
	}
	httpx.WriteJSON(response, 200, item)
}

func (h *Handler) Update(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name                string            `json:"name"`
		ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
		Currency            string            `json:"currency"`
		Frequency           string            `json:"frequency"`
		NextExpectedAt      string            `json:"next_expected_at"`
		CategoryID          *string           `json:"category_id"`
		PaymentAccountID    *string           `json:"payment_account_id"`
		Status              string            `json:"status"`
	}
	if err := httpx.DecodeJSON(request, &input, 4<<10); err != nil {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The recurring income is invalid.")
		return
	}
	nextExpectedAt, err := time.Parse("2006-01-02", input.NextExpectedAt)
	if err != nil {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The recurring income date is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err = h.service.Update(request.Context(), userID, request.PathValue("id"), models.Update{
		Name: input.Name, ExpectedAmountMinor: int64(input.ExpectedAmountMinor),
		Currency: input.Currency, Frequency: input.Frequency,
		NextExpectedAt: nextExpectedAt, CategoryID: input.CategoryID,
		PaymentAccountID: input.PaymentAccountID, Status: input.Status,
	})
	if errors.Is(err, services.ErrInvalidID) || errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The recurring income is invalid.")
		return
	}
	if errors.Is(err, recurringincomerepo.ErrNotFound) {
		httpx.WriteError(response, 404, "RECURRING_INCOME_NOT_FOUND", "The recurring income was not found.")
		return
	}
	if errors.Is(err, recurringincomerepo.ErrRelatedNotFound) {
		httpx.WriteError(response, 422, "RELATED_ENTITY_NOT_FOUND", "A selected relation was not found in the recurring income currency.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The recurring income could not be updated.")
		return
	}
	response.WriteHeader(204)
}
