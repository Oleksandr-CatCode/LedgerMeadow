package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/spaces/models"
	spacerepo "ledgermeadow/src/modules/spaces/repository"
	"ledgermeadow/src/modules/spaces/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Space, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	items, err := h.service.List(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Spaces could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"spaces": items})
}

func (h *Handler) Create(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name                   string `json:"name"`
		Type                   string `json:"type"`
		Currency               string `json:"currency"`
		MonthlyAllocationMinor string `json:"monthly_allocation_minor"`
		Protected              bool   `json:"protected"`
		Visibility             string `json:"visibility"`
	}
	if err := httpx.DecodeJSON(request, &input, 4<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The Space is invalid.")
		return
	}
	allocation, err := strconv.ParseInt(input.MonthlyAllocationMinor, 10, 64)
	if err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The Space allocation is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	id, err := h.service.Create(request.Context(), userID, models.Create{
		Name: input.Name, Type: input.Type, Currency: input.Currency,
		MonthlyAllocationMinor: allocation, Protected: input.Protected, Visibility: input.Visibility,
	})
	if errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The Space is invalid.")
		return
	}
	if errors.Is(err, spacerepo.ErrHouseholdRequired) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "HOUSEHOLD_REQUIRED", "A household is required to create a shared Space.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The Space could not be created.")
		return
	}
	httpx.WriteJSON(response, http.StatusCreated, map[string]string{"id": id})
}
