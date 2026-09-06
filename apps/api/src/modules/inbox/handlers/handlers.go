package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/inbox/models"
	inboxrepo "ledgermeadow/src/modules/inbox/repository"
	"ledgermeadow/src/modules/inbox/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
	"strconv"
)

type Service interface {
	List(context.Context, shared.UserID, string, int) (models.Page, error)
	Resolve(context.Context, shared.UserID, string, string) error
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > 100 {
			httpx.WriteError(w, 400, "INVALID_PAGE_LIMIT", "Limit must be between 1 and 100.")
			return
		}
		limit = v
	}
	u, _ := auth.UserIDFromContext(r.Context())
	p, e := h.service.List(r.Context(), u, r.URL.Query().Get("cursor"), limit)
	if errors.Is(e, inboxrepo.ErrInvalidCursor) {
		httpx.WriteError(w, 400, "INVALID_CURSOR", "The inbox cursor is invalid.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Inbox could not be loaded.")
		return
	}
	var next *string
	if p.NextCursor != "" {
		next = &p.NextCursor
	}
	httpx.WriteJSON(w, 200, map[string]any{"items": p.Items, "next_cursor": next})
}
func (h *Handler) Resolve(w http.ResponseWriter, r *http.Request) {
	var i struct {
		Resolution string `json:"resolution"`
	}
	if e := httpx.DecodeJSON(r, &i, 1024); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The inbox resolution is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	e := h.service.Resolve(r.Context(), u, r.PathValue("id"), i.Resolution)
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The inbox resolution is invalid.")
		return
	}
	if errors.Is(e, inboxrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "INBOX_ITEM_NOT_FOUND", "The inbox item was not found.")
		return
	}
	if errors.Is(e, inboxrepo.ErrUnsupportedResolution) {
		httpx.WriteError(w, http.StatusConflict, "UNSUPPORTED_RESOLUTION", "That action is not supported for this inbox item.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The inbox item could not be resolved.")
		return
	}
	w.WriteHeader(204)
}
