package handlers

import (
	"context"
	"net/http"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/planning/models"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	Get(context.Context, shared.UserID) (models.Workspace, error)
}
type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Get(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	workspace, err := h.service.Get(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Planning could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, workspace)
}
