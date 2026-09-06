package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"ledgermeadow/src/auth"
	transactionrepo "ledgermeadow/src/modules/transactions/repository"
	transactions "ledgermeadow/src/modules/transactions/types"
	"ledgermeadow/src/modules/transactions/validators"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
)

type Service interface {
	List(ctx context.Context, userID shared.UserID, cursor string, limit int, filters transactions.ListFilters) (transactionrepo.Page, error)
	Detail(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID) (transactions.Detail, error)
	Update(ctx context.Context, userID shared.UserID, transactionID shared.TransactionID, update transactions.Update) error
	BulkUpdate(ctx context.Context, userID shared.UserID, command transactions.BulkUpdate) error
}

type Handler struct {
	service Service
}

func New(service Service) *Handler {
	return &Handler{service: service}
}

type transactionResponse struct {
	ID             string  `json:"id"`
	AccountID      string  `json:"account_id"`
	AccountName    string  `json:"account_name"`
	Name           string  `json:"name"`
	MerchantName   *string `json:"merchant_name"`
	AmountMinor    string  `json:"amount_minor"`
	Currency       string  `json:"currency"`
	Date           string  `json:"date"`
	Status         string  `json:"status"`
	CategoryID     *string `json:"category_id"`
	CategoryName   *string `json:"category_name"`
	CategorySource string  `json:"category_source"`
	IsRecurring    bool    `json:"is_recurring"`
	SpaceID        *string `json:"space_id"`
	SpaceName      *string `json:"space_name"`
	ReviewStatus   string  `json:"review_status"`
	Visibility     string  `json:"visibility"`
}

func (h *Handler) List(response http.ResponseWriter, request *http.Request) {
	limit := 25
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			httpx.WriteError(response, http.StatusBadRequest, "INVALID_PAGE_LIMIT", "Limit must be between 1 and 100.")
			return
		}
		limit = parsed
	}
	filters := transactions.ListFilters{CurrentMonth: request.URL.Query().Get("current_month") == "true"}
	for key, destination := range map[string]**string{
		"account_id": &filters.AccountID, "category_id": &filters.CategoryID,
		"space_id": &filters.SpaceID, "review_status": &filters.ReviewStatus, "q": &filters.Query,
	} {
		if value := request.URL.Query().Get(key); value != "" {
			copy := value
			*destination = &copy
		}
	}
	if raw := request.URL.Query().Get("current_month"); raw != "" && raw != "true" && raw != "false" {
		httpx.WriteError(response, 400, "INVALID_FILTERS", "The transaction filters are invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	page, err := h.service.List(request.Context(), userID, request.URL.Query().Get("cursor"), limit, filters)
	if errors.Is(err, transactionrepo.ErrInvalidCursor) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_CURSOR", "The transaction cursor is invalid.")
		return
	}
	if errors.Is(err, validators.ErrInvalidFilters) {
		httpx.WriteError(response, 400, "INVALID_FILTERS", "The transaction filters are invalid.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Transactions could not be loaded.")
		return
	}

	items := make([]transactionResponse, 0, len(page.Transactions))
	for _, transaction := range page.Transactions {
		items = append(items, mapTransaction(transaction))
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{
		"transactions": items, "next_cursor": nullableCursor(page.NextCursor),
	})
}

func (h *Handler) BulkUpdate(response http.ResponseWriter, request *http.Request) {
	var input struct {
		TransactionIDs []string `json:"transaction_ids"`
		CategoryID     *string  `json:"category_id"`
		SpaceID        *string  `json:"space_id"`
		ReviewStatus   *string  `json:"review_status"`
	}
	if err := httpx.DecodeJSON(request, &input, 8<<10); err != nil {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The bulk transaction update is invalid.")
		return
	}
	ids := make([]shared.TransactionID, len(input.TransactionIDs))
	for index, id := range input.TransactionIDs {
		ids[index] = shared.TransactionID(id)
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.BulkUpdate(request.Context(), userID, transactions.BulkUpdate{TransactionIDs: ids, Update: transactions.Update{CategoryID: input.CategoryID, SpaceID: input.SpaceID, ReviewStatus: input.ReviewStatus}})
	if errors.Is(err, validators.ErrInvalidBulkUpdate) {
		httpx.WriteError(response, 400, "INVALID_REQUEST", "The bulk transaction update is invalid.")
		return
	}
	if errors.Is(err, transactionrepo.ErrNotFound) {
		httpx.WriteError(response, 404, "TRANSACTION_NOT_FOUND", "At least one transaction was not found.")
		return
	}
	if errors.Is(err, transactionrepo.ErrRelatedEntityNotFound) {
		httpx.WriteError(response, 422, "RELATED_ENTITY_NOT_FOUND", "The selected category or Space was not found for every transaction currency.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The transactions could not be updated.")
		return
	}
	response.WriteHeader(204)
}

type allocationResponse struct {
	SpaceID     string `json:"space_id"`
	SpaceName   string `json:"space_name"`
	AmountMinor string `json:"amount_minor"`
}

type detailResponse struct {
	transactionResponse
	OriginalDescription *string              `json:"original_description"`
	AuthorizedDate      *string              `json:"authorized_date"`
	Allocations         []allocationResponse `json:"allocations"`
}

func (h *Handler) Detail(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	detail, err := h.service.Detail(request.Context(), userID, shared.TransactionID(request.PathValue("id")))
	if errors.Is(err, validators.ErrInvalidUpdate) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_TRANSACTION_ID", "The transaction identifier is invalid.")
		return
	}
	if errors.Is(err, transactionrepo.ErrNotFound) {
		httpx.WriteError(response, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "The transaction was not found.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The transaction could not be loaded.")
		return
	}
	allocations := make([]allocationResponse, 0, len(detail.Allocations))
	for _, allocation := range detail.Allocations {
		allocations = append(allocations, allocationResponse{
			SpaceID: allocation.SpaceID, SpaceName: allocation.SpaceName,
			AmountMinor: strconv.FormatInt(allocation.AmountMinor, 10),
		})
	}
	var authorizedDate *string
	if detail.AuthorizedDate != nil {
		value := detail.AuthorizedDate.Format("2006-01-02")
		authorizedDate = &value
	}
	httpx.WriteJSON(response, http.StatusOK, detailResponse{
		transactionResponse: mapTransaction(detail.Transaction),
		OriginalDescription: detail.OriginalDescription,
		AuthorizedDate:      authorizedDate,
		Allocations:         allocations,
	})
}

type updateRequest struct {
	CategoryID   *string `json:"category_id"`
	SpaceID      *string `json:"space_id"`
	ReviewStatus *string `json:"review_status"`
	Visibility   *string `json:"visibility"`
}

func (h *Handler) Update(response http.ResponseWriter, request *http.Request) {
	var input updateRequest
	if err := httpx.DecodeJSON(request, &input, 4<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The transaction update is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Update(request.Context(), userID, shared.TransactionID(request.PathValue("id")), transactions.Update{
		CategoryID: input.CategoryID, SpaceID: input.SpaceID,
		ReviewStatus: input.ReviewStatus, Visibility: input.Visibility,
	})
	if errors.Is(err, validators.ErrInvalidUpdate) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The transaction update is invalid.")
		return
	}
	if errors.Is(err, transactionrepo.ErrNotFound) {
		httpx.WriteError(response, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "The transaction was not found.")
		return
	}
	if errors.Is(err, transactionrepo.ErrRelatedEntityNotFound) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "RELATED_ENTITY_NOT_FOUND", "The selected category or Space was not found.")
		return
	}
	if errors.Is(err, transactionrepo.ErrHouseholdRequired) {
		httpx.WriteError(response, http.StatusUnprocessableEntity, "HOUSEHOLD_REQUIRED", "A household is required to share this transaction.")
		return
	}
	if errors.Is(err, transactionrepo.ErrExpenseSplitExists) {
		httpx.WriteError(response, http.StatusConflict, "EXPENSE_SPLIT_EXISTS", "The expense split must be removed before this transaction can be private.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The transaction could not be updated.")
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func mapTransaction(transaction transactions.Transaction) transactionResponse {
	status := "POSTED"
	if transaction.IsPending {
		status = "PENDING"
	}
	return transactionResponse{
		ID: string(transaction.ID), AccountID: string(transaction.AccountID),
		AccountName: transaction.AccountName, Name: transaction.Name,
		MerchantName: transaction.MerchantName,
		AmountMinor:  strconv.FormatInt(transaction.AmountMinor, 10),
		Currency:     string(transaction.Currency), Date: transaction.Date.Format("2006-01-02"),
		Status: status, CategoryID: transaction.CategoryID, CategoryName: transaction.CategoryName,
		CategorySource: transaction.CategorySource,
		IsRecurring:    transaction.IsRecurring,
		SpaceID:        transaction.SpaceID, SpaceName: transaction.SpaceName,
		ReviewStatus: transaction.ReviewStatus, Visibility: transaction.Visibility,
	}
}

func nullableCursor(cursor string) *string {
	if cursor == "" {
		return nil
	}
	return &cursor
}
