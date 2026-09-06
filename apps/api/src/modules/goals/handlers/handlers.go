package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/goals/models"
	goalrepo "ledgermeadow/src/modules/goals/repository"
	"ledgermeadow/src/modules/goals/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Goal, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}
type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	items, err := h.service.List(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Goals could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"goals": items})
}

func (h *Handler) Create(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name         string  `json:"name"`
		TargetMinor  string  `json:"target_minor"`
		CurrentMinor string  `json:"current_minor"`
		Currency     string  `json:"currency"`
		TargetDate   *string `json:"target_date"`
		SpaceID      *string `json:"space_id"`
	}
	if err := httpx.DecodeJSON(request, &input, 4<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The goal is invalid.")
		return
	}
	target, targetErr := strconv.ParseInt(input.TargetMinor, 10, 64)
	current, currentErr := strconv.ParseInt(input.CurrentMinor, 10, 64)
	if targetErr != nil || currentErr != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The goal amounts are invalid.")
		return
	}
	var targetDate *time.Time
	if input.TargetDate != nil {
		value, err := time.Parse("2006-01-02", *input.TargetDate)
		if err != nil {
			httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The target date is invalid.")
			return
		}
		targetDate = &value
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	id, err := h.service.Create(request.Context(), userID, models.Create{Name: input.Name, TargetMinor: target, CurrentMinor: current, Currency: input.Currency, TargetDate: targetDate, SpaceID: input.SpaceID})
	if errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The goal is invalid.")
		return
	}
	if errors.Is(err, goalrepo.ErrSpaceNotFound) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "SPACE_NOT_FOUND", "The selected Space was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The goal could not be created.")
		return
	}
	httpx.WriteJSON(response, http.StatusCreated, map[string]string{"id": id})
}
