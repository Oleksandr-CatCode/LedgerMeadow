package changes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"ledgermeadow/src/auth"
	"ledgermeadow/src/shared/httpx"
	"ledgermeadow/src/shared/validate"
)

const (
	keepaliveInterval  = 25 * time.Second
	streamLifetime     = 15 * time.Minute
	streamWriteTimeout = 30 * time.Second
)

type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	userID, ok := auth.UserIDFromContext(request.Context())
	if !ok || !validate.UUID(string(userID)) {
		httpx.WriteError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "Sign in is required.")
		return
	}
	stream, unsubscribe, err := h.hub.Subscribe(userID)
	if errors.Is(err, ErrUserStreamLimit) {
		httpx.WriteError(response, http.StatusTooManyRequests, "CHANGE_STREAM_LIMIT", "Too many change streams are open.")
		return
	}
	if errors.Is(err, ErrReplicaStreamLimit) {
		httpx.WriteError(response, http.StatusServiceUnavailable, "CHANGE_STREAM_UNAVAILABLE", "The change stream is temporarily unavailable.")
		return
	}
	defer unsubscribe()

	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-store")
	if err := writeSSE(response, ": connected\n\n"); err != nil {
		return
	}

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()
	lifetime := time.NewTimer(streamLifetime)
	defer lifetime.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-lifetime.C:
			return
		case <-keepalive.C:
			if err := writeSSE(response, ": keepalive\n\n"); err != nil {
				return
			}
		case event, open := <-stream:
			if !open {
				return
			}
			payload, err := json.Marshal(ChangeEvent{Resources: event.Resources})
			if err != nil {
				return
			}
			if err := writeSSE(response, fmt.Sprintf("data: %s\n\n", payload)); err != nil {
				return
			}
		}
	}
}

func writeSSE(response http.ResponseWriter, payload string) error {
	controller := http.NewResponseController(response)
	if err := controller.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	if _, err := response.Write([]byte(payload)); err != nil {
		return err
	}
	return controller.Flush()
}
