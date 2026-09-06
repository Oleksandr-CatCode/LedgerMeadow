package handlers

import (
	"context"
	"errors"
	"net/http"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/categories/models"
	categoryrepo "ledgermeadow/src/modules/categories/repository"
	"ledgermeadow/src/modules/categories/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Category, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}

type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }

func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	items, err := h.service.List(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Categories could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"categories": items})
}

func (h *Handler) Create(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Name     string  `json:"name"`
		Type     string  `json:"type"`
		ParentID *string `json:"parent_id"`
	}
	if err := httpx.DecodeJSON(request, &input, 2<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The category is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	id, err := h.service.Create(request.Context(), userID, models.Create(input))
	if errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The category is invalid.")
		return
	}
	if errors.Is(err, categoryrepo.ErrParentNotFound) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "PARENT_NOT_FOUND", "The parent category was not found.")
		return
	}
	if errors.Is(err, categoryrepo.ErrLimitReached) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "CATEGORY_LIMIT_REACHED", "The category limit has been reached.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The category could not be created.")
		return
	}
	httpx.WriteJSON(response, http.StatusCreated, map[string]string{"id": id})
}
