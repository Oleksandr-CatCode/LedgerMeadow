package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/dashboard/models"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"net/http"
)

type Service interface {
	Get(context.Context, shared.UserID) (models.Dashboard, error)
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.Get(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Dashboard could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, v)
}
