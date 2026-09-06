package handlers

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"ledgermeadow/src/auth"
	bankingrepo "ledgermeadow/src/modules/banking/repository"
	"ledgermeadow/src/modules/banking/services"
	banking "ledgermeadow/src/modules/banking/types"
	"ledgermeadow/src/modules/platform/plaid"
	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"ledgermeadow/src/shared/validate"
)

type Service interface {
	CreateLinkToken(ctx context.Context, userID shared.UserID) (plaid.LinkToken, error)
	CreateUpdateLinkToken(ctx context.Context, userID shared.UserID, connectionID shared.BankConnectionID) (plaid.LinkToken, error)
	Exchange(ctx context.Context, userID shared.UserID, command services.ExchangeCommand) (shared.BankConnectionID, error)
	ConnectionStatus(ctx context.Context, userID shared.UserID) ([]banking.ConnectionStatus, error)
	Accounts(ctx context.Context, userID shared.UserID) ([]banking.Account, error)
	AccountDetail(context.Context, shared.UserID, shared.AccountID) (banking.AccountDetail, error)
	Refresh(context.Context, shared.UserID, shared.BankConnectionID) error
	Disconnect(context.Context, shared.UserID, shared.BankConnectionID) error
}

func (h *Handler) CreateUpdateLinkToken(response http.ResponseWriter, request *http.Request) {
	connectionID := request.PathValue("id")
	if !validate.UUID(connectionID) {
		httpx.WriteError(response, 400, "INVALID_CONNECTION_ID", "The bank connection identifier is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	token, err := h.service.CreateUpdateLinkToken(request.Context(), userID, shared.BankConnectionID(connectionID))
	if errors.Is(err, bankingrepo.ErrNotFound) {
		httpx.WriteError(response, 404, "BANK_CONNECTION_NOT_FOUND", "The bank connection was not found.")
		return
	}
	if errors.Is(err, services.ErrProviderUnavailable) {
		httpx.WriteError(response, 502, "BANK_PROVIDER_UNAVAILABLE", "Bank reconnection is temporarily unavailable.")
		return
	}
	if errors.Is(err, services.ErrUnsupportedOperation) {
		httpx.WriteError(response, http.StatusConflict, "UNSUPPORTED_CONNECTION_OPERATION", "This action is not available for an imported account.")
		return
	}
	if err != nil {
		httpx.WriteError(response, 500, "INTERNAL_ERROR", "The bank reconnection could not be initialized.")
		return
	}
	httpx.WriteJSON(response, 200, linkTokenResponse{LinkToken: token.LinkToken, Expiration: token.Expiration})
}

type accountTransactionResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	MerchantName *string `json:"merchant_name"`
	AmountMinor  string  `json:"amount_minor"`
	Currency     string  `json:"currency"`
	Date         string  `json:"date"`
	Status       string  `json:"status"`
}

func (h *Handler) AccountDetail(response http.ResponseWriter, request *http.Request) {
	accountID := request.PathValue("id")
	if !validate.UUID(accountID) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_ACCOUNT_ID", "The account identifier is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	detail, err := h.service.AccountDetail(request.Context(), userID, shared.AccountID(accountID))
	if errors.Is(err, bankingrepo.ErrNotFound) {
		httpx.WriteError(response, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "The account was not found.")
		return
	}
	if errors.Is(err, bankingrepo.ErrResponseBound) {
		httpx.WriteError(response, http.StatusConflict, "RESOURCE_LIMIT_EXCEEDED", "The account transaction limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The account could not be loaded.")
		return
	}
	transactions := make([]accountTransactionResponse, 0, len(detail.Transactions))
	for _, transaction := range detail.Transactions {
		status := "POSTED"
		if transaction.IsPending {
			status = "PENDING"
		}
		transactions = append(transactions, accountTransactionResponse{
			ID: string(transaction.ID), Name: transaction.Name, MerchantName: transaction.MerchantName,
			AmountMinor: strconv.FormatInt(transaction.AmountMinor, 10), Currency: string(transaction.Currency),
			Date: transaction.Date.Format("2006-01-02"), Status: status,
		})
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{
		"account": mapAccount(detail.Account), "connection_id": string(detail.ConnectionID),
		"transactions": transactions,
	})
}

func (h *Handler) Refresh(response http.ResponseWriter, request *http.Request) {
	connectionID := request.PathValue("id")
	if !validate.UUID(connectionID) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_CONNECTION_ID", "The bank connection identifier is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Refresh(request.Context(), userID, shared.BankConnectionID(connectionID))
	if errors.Is(err, bankingrepo.ErrNotFound) {
		httpx.WriteError(response, http.StatusNotFound, "BANK_CONNECTION_NOT_FOUND", "The bank connection was not found.")
		return
	}
	if errors.Is(err, services.ErrUnsupportedOperation) {
		httpx.WriteError(response, http.StatusConflict, "UNSUPPORTED_CONNECTION_OPERATION", "Import another CSV file to refresh this account.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The account refresh could not be queued.")
		return
	}
	httpx.WriteJSON(response, http.StatusAccepted, map[string]string{"status": "SYNC_PENDING"})
}

func (h *Handler) Disconnect(response http.ResponseWriter, request *http.Request) {
	connectionID := request.PathValue("id")
	if !validate.UUID(connectionID) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_CONNECTION_ID", "The bank connection identifier is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	err := h.service.Disconnect(request.Context(), userID, shared.BankConnectionID(connectionID))
	if errors.Is(err, bankingrepo.ErrNotFound) {
		httpx.WriteError(response, http.StatusNotFound, "BANK_CONNECTION_NOT_FOUND", "The bank connection was not found.")
		return
	}
	if errors.Is(err, services.ErrProviderUnavailable) {
		httpx.WriteError(response, http.StatusBadGateway, "BANK_PROVIDER_UNAVAILABLE", "The bank connection could not be revoked.")
		return
	}
	if errors.Is(err, services.ErrUnsupportedOperation) {
		httpx.WriteError(response, http.StatusConflict, "UNSUPPORTED_CONNECTION_OPERATION", "This connection provider cannot be disconnected here.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The bank connection could not be disconnected.")
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

type Handler struct {
	service Service
}

func New(service Service) *Handler {
	return &Handler{service: service}
}

type linkTokenResponse struct {
	LinkToken  string    `json:"link_token"`
	Expiration time.Time `json:"expiration"`
}

func (h *Handler) CreateLinkToken(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	token, err := h.service.CreateLinkToken(request.Context(), userID)
	if err != nil {
		httpx.WriteError(response, http.StatusBadGateway, "BANK_PROVIDER_UNAVAILABLE", "Bank connection is temporarily unavailable.")
		return
	}
	httpx.WriteJSON(response, http.StatusOK, linkTokenResponse{LinkToken: token.LinkToken, Expiration: token.Expiration})
}

type exchangeRequest struct {
	PublicToken     string `json:"public_token"`
	InstitutionID   string `json:"institution_id"`
	InstitutionName string `json:"institution_name"`
}

func (h *Handler) Exchange(response http.ResponseWriter, request *http.Request) {
	var input exchangeRequest
	if err := httpx.DecodeJSON(request, &input, 8<<10); err != nil {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The bank exchange request is invalid.")
		return
	}
	userID, _ := auth.UserIDFromContext(request.Context())
	connectionID, err := h.service.Exchange(request.Context(), userID, services.ExchangeCommand{
		PublicToken: input.PublicToken, InstitutionID: input.InstitutionID, InstitutionName: input.InstitutionName,
	})
	if errors.Is(err, services.ErrInvalidExchange) {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_REQUEST", "The bank exchange request is invalid.")
		return
	}
	if errors.Is(err, services.ErrConnectionConflict) {
		httpx.WriteError(response, http.StatusConflict, "BANK_CONNECTION_CONFLICT", "This bank connection is already assigned.")
		return
	}
	if errors.Is(err, services.ErrProviderUnavailable) {
		httpx.WriteError(response, http.StatusBadGateway, "BANK_CONNECTION_FAILED", "The bank connection could not be completed.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "The bank connection could not be stored.")
		return
	}
	httpx.WriteJSON(response, http.StatusAccepted, map[string]string{
		"connection_id": string(connectionID), "status": "SYNC_PENDING",
	})
}

type connectionResponse struct {
	ID              string     `json:"id"`
	Provider        string     `json:"provider"`
	InstitutionName string     `json:"institution_name"`
	Status          string     `json:"status"`
	LastSyncAt      *time.Time `json:"last_sync_at"`
	LastErrorCode   *string    `json:"last_error_code"`
}

func (h *Handler) ConnectionStatus(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	statuses, err := h.service.ConnectionStatus(request.Context(), userID)
	if errors.Is(err, bankingrepo.ErrResponseBound) {
		httpx.WriteError(response, http.StatusConflict, "RESOURCE_LIMIT_EXCEEDED", "The bank connection limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Connection status could not be loaded.")
		return
	}
	items := make([]connectionResponse, 0, len(statuses))
	for _, status := range statuses {
		items = append(items, connectionResponse{
			ID: string(status.ID), Provider: status.Provider, InstitutionName: status.InstitutionName, Status: status.Status,
			LastSyncAt: status.LastSyncAt, LastErrorCode: status.LastErrorCode,
		})
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"connections": items})
}

type accountResponse struct {
	ID                    string  `json:"id"`
	Provider              string  `json:"provider"`
	Name                  string  `json:"name"`
	OfficialName          *string `json:"official_name"`
	Mask                  *string `json:"mask"`
	Type                  string  `json:"type"`
	Subtype               *string `json:"subtype"`
	BalanceMinor          string  `json:"balance_minor"`
	AvailableBalanceMinor *string `json:"available_balance_minor"`
	Currency              string  `json:"currency"`
}

type totalResponse struct {
	AmountMinor string `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func (h *Handler) Accounts(response http.ResponseWriter, request *http.Request) {
	userID, _ := auth.UserIDFromContext(request.Context())
	accounts, err := h.service.Accounts(request.Context(), userID)
	if errors.Is(err, bankingrepo.ErrResponseBound) {
		httpx.WriteError(response, http.StatusConflict, "RESOURCE_LIMIT_EXCEEDED", "The account limit was exceeded.")
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "Accounts could not be loaded.")
		return
	}
	items := make([]accountResponse, 0, len(accounts))
	totals := make(map[shared.Currency]int64)
	for _, account := range accounts {
		var available *string
		if account.AvailableBalanceMinor != nil {
			value := strconv.FormatInt(*account.AvailableBalanceMinor, 10)
			available = &value
		}
		mapped := mapAccount(account)
		mapped.AvailableBalanceMinor = available
		items = append(items, mapped)
		current := totals[account.Currency]
		if (account.BalanceMinor > 0 && current > math.MaxInt64-account.BalanceMinor) ||
			(account.BalanceMinor < 0 && current < math.MinInt64-account.BalanceMinor) {
			httpx.WriteError(response, http.StatusInternalServerError, "MONEY_OVERFLOW", "Account totals could not be represented safely.")
			return
		}
		totals[account.Currency] = current + account.BalanceMinor
	}
	totalItems := make([]totalResponse, 0, len(totals))
	for _, currency := range []shared.Currency{shared.CurrencyCAD, shared.CurrencyUSD} {
		if amount, ok := totals[currency]; ok {
			totalItems = append(totalItems, totalResponse{AmountMinor: strconv.FormatInt(amount, 10), Currency: string(currency)})
		}
	}
	httpx.WriteJSON(response, http.StatusOK, map[string]any{"accounts": items, "totals": totalItems})
}

func mapAccount(account banking.Account) accountResponse {
	var available *string
	if account.AvailableBalanceMinor != nil {
		value := strconv.FormatInt(*account.AvailableBalanceMinor, 10)
		available = &value
	}
	return accountResponse{
		ID: string(account.ID), Provider: account.Provider, Name: account.Name, OfficialName: account.OfficialName,
		Mask: account.Mask, Type: account.Type, Subtype: account.Subtype,
		BalanceMinor: strconv.FormatInt(account.BalanceMinor, 10), AvailableBalanceMinor: available,
		Currency: string(account.Currency),
	}
}
