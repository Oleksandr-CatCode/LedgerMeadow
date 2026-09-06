package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/subscriptions/models"
	subscriptionrepo "ledgermeadow/src/modules/subscriptions/repository"
	"ledgermeadow/src/modules/subscriptions/services"
	"ledgermeadow/src/modules/subscriptions/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
	"time"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Subscription, error)
	Detail(context.Context, shared.UserID, string) (models.Detail, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFromContext(r.Context())
	item, err := h.service.Detail(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, services.ErrInvalidID) {
		httpx.WriteError(w, 400, "INVALID_SUBSCRIPTION_ID", "The subscription identifier is invalid.")
		return
	}
	if errors.Is(err, subscriptionrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "SUBSCRIPTION_NOT_FOUND", "The subscription was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The subscription could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, item)
}

type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.List(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Subscriptions could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"subscriptions": v})
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var i struct {
		MerchantName        string            `json:"merchant_name"`
		ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
		Currency            string            `json:"currency"`
		Frequency           string            `json:"frequency"`
		NextExpectedAt      string            `json:"next_expected_at"`
		CategoryID          *string           `json:"category_id"`
		SpaceID             *string           `json:"space_id"`
		PaymentAccountID    *string           `json:"payment_account_id"`
	}
	if e := httpx.DecodeJSON(r, &i, 4<<10); e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The subscription is invalid.")
		return
	}
	d, e := time.Parse("2006-01-02", i.NextExpectedAt)
	if e != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The subscription date is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	id, e := h.service.Create(r.Context(), u, models.Create{MerchantName: i.MerchantName, ExpectedAmountMinor: int64(i.ExpectedAmountMinor), Currency: i.Currency, Frequency: i.Frequency, NextExpectedAt: d, CategoryID: i.CategoryID, SpaceID: i.SpaceID, PaymentAccountID: i.PaymentAccountID})
	if errors.Is(e, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The subscription is invalid.")
		return
	}
	if errors.Is(e, subscriptionrepo.ErrRelatedNotFound) {
		httpx.WriteError(w, 422, "RELATED_ENTITY_NOT_FOUND", "A selected relation was not found.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The subscription could not be created.")
		return
	}
	httpx.WriteJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reclassify          string            `json:"reclassify"`
		MerchantName        string            `json:"merchant_name"`
		ExpectedAmountMinor shared.MinorUnits `json:"expected_amount_minor"`
		Currency            string            `json:"currency"`
		Frequency           string            `json:"frequency"`
		NextExpectedAt      string            `json:"next_expected_at"`
		CategoryID          *string           `json:"category_id"`
		SpaceID             *string           `json:"space_id"`
		PaymentAccountID    *string           `json:"payment_account_id"`
		Status              string            `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &input, 4<<10); err != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The subscription is invalid.")
		return
	}
	var next time.Time
	var err error
	if input.Reclassify == "" {
		next, err = time.Parse("2006-01-02", input.NextExpectedAt)
		if err != nil {
			httpx.WriteError(w, 400, "INVALID_REQUEST", "The subscription date is invalid.")
			return
		}
	}
	userID, _ := auth.UserIDFromContext(r.Context())
	err = h.service.Update(r.Context(), userID, r.PathValue("id"), models.Update{Reclassify: input.Reclassify, MerchantName: input.MerchantName, ExpectedAmountMinor: int64(input.ExpectedAmountMinor), Currency: input.Currency, Frequency: input.Frequency, NextExpectedAt: next, CategoryID: input.CategoryID, SpaceID: input.SpaceID, PaymentAccountID: input.PaymentAccountID, Status: input.Status})
	if errors.Is(err, services.ErrInvalidID) || errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The subscription is invalid.")
		return
	}
	if errors.Is(err, subscriptionrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "SUBSCRIPTION_NOT_FOUND", "The subscription was not found.")
		return
	}
	if errors.Is(err, subscriptionrepo.ErrRelatedNotFound) {
		httpx.WriteError(w, 422, "RELATED_ENTITY_NOT_FOUND", "A selected relation was not found in the subscription currency.")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The subscription could not be updated.")
		return
	}
	w.WriteHeader(204)
}
