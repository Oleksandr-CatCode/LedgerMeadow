package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/search/models"
	"ledgermeadow/src/modules/search/services"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
)

type Service interface {
	Prefix(context.Context, shared.UserID, string) ([]models.Result, error)
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) Prefix(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.Prefix(r.Context(), u, r.URL.Query().Get("q"))
	if errors.Is(e, services.ErrInvalidQuery) {
		httpx.WriteError(w, 400, "INVALID_SEARCH_QUERY", "Search requires a 2 to 80 character prefix.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Search could not be completed.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"results": v})
}
