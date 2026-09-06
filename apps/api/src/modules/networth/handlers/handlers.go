package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/networth/models"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"net/http"
)

type Service interface {
	Get(context.Context, shared.UserID) (models.NetWorth, error)
}
type Handler struct{ service Service }

func New(service Service) *Handler { return &Handler{service: service} }
func (h *Handler) Get(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	result, err := h.service.Get(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Net worth could not be loaded.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, result)
}
