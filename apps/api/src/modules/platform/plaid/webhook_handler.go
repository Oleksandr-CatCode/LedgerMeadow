package plaid

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"ledgermeadow/src/shared/httpx"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
)

type WebhookRepository interface {
	ConnectionByProviderItem(ctx context.Context, providerItemID string) (shared.BankConnectionID, shared.UserID, error)
	EnqueueWebhookSync(ctx context.Context, connectionID shared.BankConnectionID, userID shared.UserID, dedupeKey string) error
}

type WebhookHandler struct {
	client     *Client
	repository WebhookRepository
	now        func() time.Time
}

func NewWebhookHandler(client *Client, repository WebhookRepository) *WebhookHandler {
	return &WebhookHandler{client: client, repository: repository, now: time.Now}
}

func (h *WebhookHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(io.LimitReader(request.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_WEBHOOK", "Webhook body is invalid.")
		return
	}
	if err := h.client.VerifyWebhook(
		request.Context(), request.Header.Get("Plaid-Verification"), body, h.now().UTC(),
	); err != nil {
		httpx.WriteError(response, http.StatusUnauthorized, "INVALID_WEBHOOK_SIGNATURE", "Webhook signature is invalid.")
		return
	}
	var webhook struct {
		Type        string `json:"webhook_type"`
		Code        string `json:"webhook_code"`
		ItemID      string `json:"item_id"`
		Environment string `json:"environment"`
	}
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(body), 1<<20))
	if err := decoder.Decode(&webhook); err != nil || webhook.Environment != h.client.environment {
		httpx.WriteError(response, http.StatusBadRequest, "INVALID_WEBHOOK", "Webhook body is invalid.")
		return
	}
	if webhook.Type != "TRANSACTIONS" || webhook.Code != "SYNC_UPDATES_AVAILABLE" || webhook.ItemID == "" {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	connectionID, userID, err := h.repository.ConnectionByProviderItem(request.Context(), webhook.ItemID)
	if errors.Is(err, pgx.ErrNoRows) {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "WEBHOOK_PROCESSING_FAILED", "Webhook could not be recorded.")
		return
	}
	digestInput := make([]byte, 0, len(request.Header.Get("Plaid-Verification"))+1+len(body))
	digestInput = append(digestInput, request.Header.Get("Plaid-Verification")...)
	digestInput = append(digestInput, '\n')
	digestInput = append(digestInput, body...)
	digest := sha256.Sum256(digestInput)
	dedupeKey := "plaid-webhook:" + hex.EncodeToString(digest[:])
	if err := h.repository.EnqueueWebhookSync(request.Context(), connectionID, userID, dedupeKey); err != nil {
		httpx.WriteError(response, http.StatusInternalServerError, "WEBHOOK_PROCESSING_FAILED", "Webhook could not be recorded.")
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
