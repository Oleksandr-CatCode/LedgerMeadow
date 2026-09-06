package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/notifications/models"
	notificationrepo "ledgermeadow/src/modules/notifications/repository"
	"ledgermeadow/src/modules/notifications/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Notification, error)
	Preferences(context.Context, shared.UserID) ([]models.Preference, error)
	UpdatePreference(context.Context, shared.UserID, models.Preference) error
	MarkRead(context.Context, shared.UserID, string) error
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.List(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Notifications could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"notifications": v})
}
func (h *Handler) Preferences(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.Preferences(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Notification preferences could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"preferences": v})
}
func (h *Handler) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	var i models.Preference
	if e := httpx.DecodeJSON(r, &i, 2048); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The notification preference is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	e := h.service.UpdatePreference(r.Context(), u, i)
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The notification preference is invalid.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The notification preference could not be updated.")
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	e := h.service.MarkRead(r.Context(), u, r.PathValue("id"))
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, http.StatusBadRequest, "INVALID_REQUEST", "The notification is invalid.")
		return
	}
	if errors.Is(e, notificationrepo.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "NOTIFICATION_NOT_FOUND", "The notification was not found.")
		return
	}
	if e != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The notification could not be updated.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
