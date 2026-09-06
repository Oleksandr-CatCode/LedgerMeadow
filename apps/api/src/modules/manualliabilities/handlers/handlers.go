package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/manualliabilities/models"
	"ledgermeadow/src/modules/manualliabilities/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Liability, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	items, err := h.service.List(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Manual liabilities could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"liabilities": items})
}

func (h *Handler) Create(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name              string `json:"name"`
		Type              string `json:"type"`
		BalanceMinor      string `json:"balance_minor"`
		Currency          string `json:"currency"`
		IncludeInNetWorth bool   `json:"include_in_net_worth"`
	}
	if err := httpx.DecodeJSON(request, &input, 4<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The manual liability is invalid.")
		return
	}
	balance, err := strconv.ParseInt(input.BalanceMinor, 10, 64)
	if err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The liability balance is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	id, err := h.service.Create(request.Context(), userID, models.Create{
		Name: input.Name, Type: input.Type, BalanceMinor: balance,
		Currency: input.Currency, IncludeInNetWorth: input.IncludeInNetWorth,
	})
	if errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The manual liability is invalid.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The manual liability could not be created.")
		return
	}
	httpx.WriteJSON(response, http.StatusCreated, map[string]string{"id": id})
}
