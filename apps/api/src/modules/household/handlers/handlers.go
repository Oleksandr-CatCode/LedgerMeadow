package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/household/models"
	householdrepo "ledgermeadow/src/modules/household/repository"
	"ledgermeadow/src/modules/household/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
)

type Service interface {
	Get(context.Context, shared.UserID) (*models.Household, error)
	Create(context.Context, shared.UserID, string) (string, error)
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.Get(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Household could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"household": v})
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var i struct {
		Name string `json:"name"`
	}
	if e := httpx.DecodeJSON(r, &i, 1024); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The household is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	id, e := h.service.Create(r.Context(), u, i.Name)
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The household is invalid.")
		return
	}
	if errors.Is(e, householdrepo.ErrAlreadyMember) {
		httpx.WriteError(w, 409, "ALREADY_IN_HOUSEHOLD", "You already belong to a household.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The household could not be created.")
		return
	}
	httpx.WriteJSON(w, 201, map[string]string{"id": id})
}
