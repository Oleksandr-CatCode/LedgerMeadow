package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/budgets/models"
	budgetrepo "ledgermeadow/src/modules/budgets/repository"
	"ledgermeadow/src/modules/budgets/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
	"strconv"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Budget, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.List(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Budgets could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"budgets": v})
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var i struct {
		Name              string  `json:"name"`
		CategoryID        *string `json:"category_id"`
		SpaceID           *string `json:"space_id"`
		Period            string  `json:"period"`
		LimitMinor        string  `json:"limit_minor"`
		Currency          string  `json:"currency"`
		WarningThreshold  int     `json:"warning_threshold"`
		CriticalThreshold int     `json:"critical_threshold"`
		CarryoverEnabled  bool    `json:"carryover_enabled"`
	}
	if e := httpx.DecodeJSON(r, &i, 4<<10); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The budget is invalid.")
		return
	}
	a, e := strconv.ParseInt(i.LimitMinor, 10, 64)
	if e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The budget limit is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	id, e := h.service.Create(r.Context(), u, models.Create{Name: i.Name, CategoryID: i.CategoryID, SpaceID: i.SpaceID, Period: i.Period, LimitMinor: a, Currency: i.Currency, WarningThreshold: i.WarningThreshold, CriticalThreshold: i.CriticalThreshold, CarryoverEnabled: i.CarryoverEnabled})
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The budget is invalid.")
		return
	}
	if errors.Is(e, budgetrepo.ErrRelatedNotFound) {
		httpx.WriteError(w, 422, "RELATED_ENTITY_NOT_FOUND", "The selected category or Space was not found.")
		return
	}
	if errors.Is(e, budgetrepo.ErrInitializationBound) {
		httpx.WriteError(w, http.StatusConflict, "BUDGET_INITIALIZATION_BOUND_EXCEEDED", "The current budget period contains too many transactions to initialize safely.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The budget could not be created.")
		return
	}
	httpx.WriteJSON(w, 201, map[string]string{"id": id})
}
