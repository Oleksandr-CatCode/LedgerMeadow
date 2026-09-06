package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/bills/models"
	billrepo "ledgermeadow/src/modules/bills/repository"
	"ledgermeadow/src/modules/bills/services"
	"ledgermeadow/src/modules/bills/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Bill, error)
	Detail(context.Context, shared.UserID, string) (models.Bill, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	item, err := h.service.Detail(r.Context(), u, r.PathValue("id"))
	if errors.Is(err, services.ErrInvalidID) {
		httpx.WriteError(w, 400, "INVALID_BILL_ID", "The bill identifier is invalid.")
		return
	}
	if errors.Is(err, billrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "BILL_NOT_FOUND", "The bill was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The bill could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, item)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reclassify          string            `json:"reclassify"`
		Name                string            `json:"name"`
		AmountType          string            `json:"amount_type"`
		ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
		Currency            string            `json:"currency"`
		Frequency           string            `json:"frequency"`
		NextDueAt           string            `json:"next_due_at"`
		CategoryID          *string           `json:"category_id"`
		SpaceID             *string           `json:"space_id"`
		Status              string            `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &input, 4<<10); err != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill is invalid.")
		return
	}
	var next time.Time
	var err error
	if input.Reclassify == "" {
		next, err = time.Parse("2006-01-02", input.NextDueAt)
		if err != nil {
			httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill date is invalid.")
			return
		}
	}
	u, _ := auth.UserIDFromContext(r.Context())
	err = h.service.Update(r.Context(), u, r.PathValue("id"), models.Update{
		Reclassify: input.Reclassify, Name: input.Name, AmountType: input.AmountType,
		ExpectedAmountMinor: int64(input.ExpectedAmountMinor), Currency: input.Currency,
		Frequency: input.Frequency, NextDueAt: next,
		CategoryID: input.CategoryID, SpaceID: input.SpaceID, Status: input.Status,
	})
	if errors.Is(err, services.ErrInvalidID) || errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill is invalid.")
		return
	}
	if errors.Is(err, billrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "BILL_NOT_FOUND", "The bill was not found.")
		return
	}
	if errors.Is(err, billrepo.ErrRelatedNotFound) {
		httpx.WriteError(w, 422, "RELATED_ENTITY_NOT_FOUND", "A selected relation was not found in the bill currency.")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The bill could not be updated.")
		return
	}
	w.WriteHeader(204)
}

type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.List(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Bills could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"bills": v})
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var i struct {
		Name                string  `json:"name"`
		AmountType          string  `json:"amount_type"`
		ExpectedAmountMinor string  `json:"expected_amount_minor"`
		Currency            string  `json:"currency"`
		Frequency           string  `json:"frequency"`
		NextDueAt           string  `json:"next_due_at"`
		CategoryID          *string `json:"category_id"`
		SpaceID             *string `json:"space_id"`
	}
	if e := httpx.DecodeJSON(r, &i, 4<<10); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill is invalid.")
		return
	}
	a, e := strconv.ParseInt(i.ExpectedAmountMinor, 10, 64)
	if e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill amount is invalid.")
		return
	}
	d, e := time.Parse("2006-01-02", i.NextDueAt)
	if e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill date is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	id, e := h.service.Create(r.Context(), u, models.Create{Name: i.Name, AmountType: i.AmountType, ExpectedAmountMinor: a, Currency: i.Currency, Frequency: i.Frequency, NextDueAt: d, CategoryID: i.CategoryID, SpaceID: i.SpaceID})
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The bill is invalid.")
		return
	}
	if errors.Is(e, billrepo.ErrRelatedNotFound) {
		httpx.WriteError(w, 422, "RELATED_ENTITY_NOT_FOUND", "The selected category or Space was not found.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The bill could not be created.")
		return
	}
	httpx.WriteJSON(w, 201, map[string]string{"id": id})
}
