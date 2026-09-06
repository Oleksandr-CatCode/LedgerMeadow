package handlers

import (
	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/loans/models"
	loanrepo "ledgermeadow/src/modules/loans/repository"
	"ledgermeadow/src/modules/loans/services"
	"ledgermeadow/src/modules/loans/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"context"
	"errors"
	"net/http"
	"time"
)

type Service interface {
	List(context.Context, shared.UserID) ([]models.Loan, error)
	Detail(context.Context, shared.UserID, string) (models.Detail, error)
	Scenario(context.Context, shared.UserID, string, models.ScenarioInput) (models.Scenario, error)
	Create(context.Context, shared.UserID, models.Create) (string, error)
	Update(context.Context, shared.UserID, string, models.Update) error
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	command, ok := decodeLoanWrite(w, r)
	if !ok {
		return
	}
	userID, _ := auth.UserIDFromContext(r.Context())
	err := h.service.Update(r.Context(), userID, r.PathValue("id"), models.Update(command))
	if errors.Is(err, services.ErrInvalidID) || errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The loan is invalid.")
		return
	}
	if errors.Is(err, loanrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "LOAN_NOT_FOUND", "The loan was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The loan could not be updated.")
		return
	}
	w.WriteHeader(204)
}

type Handler struct{ service Service }

func New(s Service) *Handler { return &Handler{service: s} }
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	command, ok := decodeLoanWrite(w, r)
	if !ok {
		return
	}
	userID, _ := auth.UserIDFromContext(r.Context())
	id, err := h.service.Create(r.Context(), userID, command)
	if errors.Is(err, validators.ErrInvalid) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The loan is invalid.")
		return
	}
	if err != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The loan could not be created.")
		return
	}
	httpx.WriteJSON(w, 201, map[string]string{"id": id})
}

func decodeLoanWrite(w http.ResponseWriter, r *http.Request) (models.Create, bool) {
	var input struct {
		Name      string            `json:"name"`
		Principal shared.MinorUnits `json:"principal_remaining_minor"`
		Currency  string            `json:"currency"`
		Rate      int               `json:"interest_rate_basis_points"`
		Payment   shared.MinorUnits `json:"monthly_payment_minor"`
		Next      *string           `json:"next_payment_at"`
	}
	if err := httpx.DecodeJSON(r, &input, 4<<10); err != nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The loan is invalid.")
		return models.Create{}, false
	}
	var next *time.Time
	if input.Next != nil {
		value, err := time.Parse("2006-01-02", *input.Next)
		if err != nil {
			httpx.WriteError(w, 400, "INVALID_REQUEST", "The next payment date is invalid.")
			return models.Create{}, false
		}
		next = &value
	}
	return models.Create{Name: input.Name, PrincipalRemainingMinor: int64(input.Principal), Currency: input.Currency, InterestRateBasisPoints: input.Rate, MonthlyPaymentMinor: int64(input.Payment), NextPaymentAt: next}, true
}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.List(r.Context(), u)
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "Loans could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"loans": v})
}
func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.Detail(r.Context(), u, r.PathValue("id"))
	if errors.Is(e, services.ErrInvalidID) {
		httpx.WriteError(w, 400, "INVALID_LOAN_ID", "The loan identifier is invalid.")
		return
	}
	if errors.Is(e, loanrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "LOAN_NOT_FOUND", "The loan was not found.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The loan could not be loaded.")
		return
	}
	httpx.WriteJSON(w, 200, v)
}
func (h *Handler) Scenario(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExtraMonthlyPaymentMinor *shared.MinorUnits `json:"extra_monthly_payment_minor"`
		IncludeSchedule          *bool              `json:"include_schedule"`
	}
	if err := httpx.DecodeJSON(r, &input, 4<<10); err != nil || input.ExtraMonthlyPaymentMinor == nil || input.IncludeSchedule == nil {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The loan scenario is invalid.")
		return
	}
	u, _ := auth.UserIDFromContext(r.Context())
	v, e := h.service.Scenario(r.Context(), u, r.PathValue("id"), models.ScenarioInput{ExtraMonthlyPaymentMinor: int64(*input.ExtraMonthlyPaymentMinor), IncludeSchedule: *input.IncludeSchedule})
	if errors.Is(e, services.ErrInvalidID) {
		httpx.WriteError(w, 400, "INVALID_LOAN_ID", "The loan identifier is invalid.")
		return
	}
	if errors.Is(e, validators.ErrInvalidScenario) {
		httpx.WriteError(w, 400, "INVALID_REQUEST", "The loan scenario is invalid.")
		return
	}
	if errors.Is(e, loanrepo.ErrNotFound) {
		httpx.WriteError(w, 404, "LOAN_NOT_FOUND", "The loan was not found.")
		return
	}
	if errors.Is(e, services.ErrEngineUnavailable) {
		httpx.WriteError(w, 503, "FINANCIAL_ENGINE_UNAVAILABLE", "Loan scenarios require the configured Financial Engine transport.")
		return
	}
	if e != nil {
		httpx.WriteError(w, 500, "INTERNAL_ERROR", "The loan scenario could not be calculated.")
		return
	}
	httpx.WriteJSON(w, 200, v)
}
