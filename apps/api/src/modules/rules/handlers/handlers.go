package handlers

import (
	"context"
	"errors"
	"net/http"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/rules/models"
	rulerepo "ledgermeadow/src/modules/rules/repository"
	"ledgermeadow/src/modules/rules/services"
	"ledgermeadow/src/modules/rules/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Rule, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
	Delete(context.Context, shared.UserID, string) error
	Duplicate(context.Context, shared.UserID, string) (string, error)
	Reorder(context.Context, shared.UserID, []string) error
}

func (h *Handler) Update(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name       *string             `json:"name"`
		Enabled    *bool               `json:"enabled"`
		Conditions *[]models.Condition `json:"conditions"`
		Actions    *[]models.Action    `json:"actions"`
	}
	if err := httpx.DecodeJSON(request, &input, 16<<10); err != nil {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The rule update is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Update(request.Context(), userID, request.PathValue("id"), models.Update(input))
	if errors.Is(err, services.ErrInvalidID) || errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The rule update is invalid.")
		return
	}
	if errors.Is(err, rulerepo.ErrNotFound) {
		httpx.WriteError(response, 404, "RULE_NOT_FOUND", "The rule was not found.")
		return
	}
	if errors.Is(err, rulerepo.ErrTooMany) {
		httpx.WriteError(response, 409, "RULE_LIMIT_EXCEEDED", "The rule limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The rule could not be updated.")
		return
	}
	response.WriteHeader(204)
}

func (h *Handler) Delete(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Delete(request.Context(), userID, request.PathValue("id"))
	if errors.Is(err, services.ErrInvalidID) {
		httpx.WriteError(response, 400, "INVALID_RULE_ID", "The rule identifier is invalid.")
		return
	}
	if errors.Is(err, rulerepo.ErrNotFound) {
		httpx.WriteError(response, 404, "RULE_NOT_FOUND", "The rule was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The rule could not be deleted.")
		return
	}
	response.WriteHeader(204)
}

func (h *Handler) Duplicate(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	id, err := h.service.Duplicate(request.Context(), userID, request.PathValue("id"))
	if errors.Is(err, services.ErrInvalidID) {
		httpx.WriteError(response, 400, "INVALID_RULE_ID", "The rule identifier is invalid.")
		return
	}
	if errors.Is(err, rulerepo.ErrNotFound) {
		httpx.WriteError(response, 404, "RULE_NOT_FOUND", "The rule was not found.")
		return
	}
	if errors.Is(err, rulerepo.ErrTooMany) {
		httpx.WriteError(response, 409, "RULE_LIMIT_EXCEEDED", "The rule limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The rule could not be duplicated.")
		return
	}
	httpx.WriteJSON(response, 201, map[string]string{"id": id})
}

func (h *Handler) Reorder(response http.ResponseWriter, request *http.Request) {
	var input struct {
		OrderedRuleIDs []string `json:"ordered_rule_ids"`
	}
	if err := httpx.DecodeJSON(request, &input, 8<<10); err != nil {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The rule order is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Reorder(request.Context(), userID, input.OrderedRuleIDs)
	if errors.Is(err, services.ErrInvalidOrder) {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The rule order is invalid.")
		return
	}
	if errors.Is(err, rulerepo.ErrInvalidOrder) {
		httpx.WriteError(response, 409, "RULE_SET_CHANGED", "The rule set changed; reload before reordering.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The rules could not be reordered.")
		return
	}
	response.WriteHeader(204)
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	items, err := h.service.List(request.Context(), userID)
	if errors.Is(err, rulerepo.ErrTooMany) {
		httpx.WriteError(response, 409, "RULE_LIMIT_EXCEEDED", "The rule limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Rules could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"rules": items})
}
func (h *Handler) Create(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name       string             `json:"name"`
		Enabled    bool               `json:"enabled"`
		Conditions []models.Condition `json:"conditions"`
		Actions    []models.Action    `json:"actions"`
	}
	if err := httpx.DecodeJSON(request, &input, 16<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The rule is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	id, err := h.service.Create(request.Context(), userID, models.Create(input))
	if errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The rule is invalid.")
		return
	}
	if errors.Is(err, rulerepo.ErrTooMany) {
		httpx.WriteError(response, 409, "RULE_LIMIT_EXCEEDED", "The rule limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The rule could not be created.")
		return
	}
	httpx.WriteJSON(response, http.StatusCreated, map[string]string{"id": id})
}
