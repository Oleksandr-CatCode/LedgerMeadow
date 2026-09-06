package changes

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ledgermeadow/src/auth"
	shared "ledgermeadow/src/shared/types"
)

func TestChangeStreamRejectsUnauthenticatedRequest(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler(NewHub()).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/changes", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestChangeStreamDoesNotDeliverAnotherUsersEvent(t *testing.T) {
	hub := NewHub()
	handler := NewHandler(hub)
	ctx, cancel := context.WithCancel(auth.ContextWithUserID(context.Background(), shared.UserID("00000000-0000-0000-0000-000000000001")))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/changes", nil).WithContext(ctx)
	response := newStreamRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(response, request)
	}()

	select {
	case <-response.flushed:
	case <-time.After(time.Second):
		t.Fatal("change stream did not open")
	}
	hub.Publish(Event{UserID: shared.UserID("00000000-0000-0000-0000-000000000002"), Resources: []string{ResourceInbox}})
	time.Sleep(20 * time.Millisecond)
	if body := response.bodyString(); strings.Contains(body, "data:") {
		t.Fatalf("another user's event reached the stream: %q", body)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("change stream did not close after cancellation")
	}
}

type streamRecorder struct {
	header  http.Header
	mu      sync.Mutex
	body    bytes.Buffer
	flushed chan struct{}
}

func newStreamRecorder() *streamRecorder {
	return &streamRecorder{header: make(http.Header), flushed: make(chan struct{}, 1)}
}

func (r *streamRecorder) Header() http.Header {
	return r.header
}

func (r *streamRecorder) WriteHeader(int) {}

func (r *streamRecorder) Write(body []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(body)
}

func (r *streamRecorder) Flush() {
	select {
	case r.flushed <- struct{}{}:
	default:
	}
}

func (r *streamRecorder) bodyString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}
