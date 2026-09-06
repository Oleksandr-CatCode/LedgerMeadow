package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/manualassets/models"
	"ledgermeadow/src/modules/manualassets/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
	"strconv"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Asset, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
}
type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.List(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Manual assets could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"assets": v})
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var i struct {
		Name              string `json:"name"`
		Type              string `json:"type"`
		ValueMinor        string `json:"value_minor"`
		Currency          string `json:"currency"`
		IncludeInNetWorth bool   `json:"include_in_net_worth"`
	}
	if e := httpx.DecodeJSON(r, &i, 2048); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The asset is invalid.")
		return
	}
	v, e := strconv.ParseInt(i.ValueMinor, 10, 64)
	if e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The asset value is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	id, e := h.service.Create(r.Context(), u, models.Create{Name: i.Name, Type: i.Type, ValueMinor: v, Currency: i.Currency, IncludeInNetWorth: i.IncludeInNetWorth})
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The asset is invalid.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The asset could not be created.")
		return
	}
	httpx.WriteJSON(w, 201, map[string]string{"id": id})
}
