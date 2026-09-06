package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/modules/expensesplits/models"
	expensesplitrepo "ledgermeadow/src/modules/expensesplits/repository"
	"ledgermeadow/src/modules/expensesplits/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	Get(context.Context, shared.UserID, shared.TransactionID) (models.Split, error)
	Replace(context.Context, shared.UserID, shared.TransactionID, models.Replace) (models.Split, error)
	Clear(context.Context, shared.UserID, shared.TransactionID) error
}

func (h *Handler) Get(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	split, err := h.service.Get(
		request.Context(), userID, shared.TransactionID(request.PathValue("id")),
	)
	if writeSplitError(response, err) {
		return
	}
	httpx.WriteJSON(response, http.StatusOK, split)
}

type Handler struct {
	service Service
}

func New(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Replace(response http.ResponseWriter, request *http.Request) {
	var input struct {
		PayerAmountMinor string `json:"payer_amount_minor"`
		Participants     []struct {
			UserID      string `json:"user_id"`
			AmountMinor string `json:"amount_minor"`
		} `json:"participants"`
	}
	if err := httpx.DecodeJSON(request, &input, 16<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_EXPENSE_SPLIT", "The expense split is invalid.")
		return
	}
	payerAmount, err := strconv.ParseInt(input.PayerAmountMinor, 10, 64)
	if err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_EXPENSE_SPLIT", "The payer share is invalid.")
		return
	}
	command := models.Replace{PayerAmountMinor: payerAmount, Participants: make([]models.Participant, 0, len(input.Participants))}
	for _, participant := range input.Participants {
		amount, err := strconv.ParseInt(participant.AmountMinor, 10, 64)
		if err != nil {
			httpx.WriteError(response, http.StatusBadRequest, "INVALID_EXPENSE_SPLIT", "A participant share is invalid.")
			return
		}
		command.Participants = append(command.Participants, models.Participant{
			UserID: participant.UserID, AmountMinor: shared.MinorUnits(amount),
		})
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	split, err := h.service.Replace(
		request.Context(), userID, shared.TransactionID(request.PathValue("id")), command,
	)
	if writeSplitError(response, err) {
		return
	}
	httpx.WriteJSON(response, http.StatusOK, split)
}

func (h *Handler) Clear(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Clear(
		request.Context(), userID, shared.TransactionID(request.PathValue("id")),
	)
	if writeSplitError(response, err) {
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func writeSplitError(response http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, validators.ErrInvalid):
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_EXPENSE_SPLIT", "The expense split is invalid.")
	case errors.Is(err, expensesplitrepo.ErrNotFound):
		httpx.WriteError(response, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "The transaction was not found.")
	case errors.Is(err, expensesplitrepo.ErrHouseholdRequired):
		httpx.WriteError(response, http.StatusUnprocessableEntity, "HOUSEHOLD_REQUIRED", "A household is required to split this transaction.")
	case errors.Is(err, expensesplitrepo.ErrInvalidParticipants):
		httpx.WriteError(response, http.StatusUnprocessableEntity, "INVALID_SPLIT_PARTICIPANTS", "Every participant must belong to your household.")
	case errors.Is(err, expensesplitrepo.ErrAmountsMismatch):
		httpx.WriteError(response, http.StatusUnprocessableEntity, "SPLIT_AMOUNT_MISMATCH", "The shares must equal the transaction amount.")
	case errors.Is(err, expensesplitrepo.ErrNotExpense):
		httpx.WriteError(response, http.StatusUnprocessableEntity, "TRANSACTION_NOT_SPLITTABLE", "Only an expense transaction can be split.")
	case errors.Is(err, expensesplitrepo.ErrExistingBound):
		httpx.WriteError(response, http.StatusConflict, "EXPENSE_SPLIT_BOUND_EXCEEDED", "The stored expense split exceeds the supported participant limit.")
	case err != nil:
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The expense split could not be saved.")
	default:
		return false
	}
	return true
}
