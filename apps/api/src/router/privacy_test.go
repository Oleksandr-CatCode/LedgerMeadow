package router

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivacyHeadersCoverDeniedRequests(t *testing.T) {
	handler := privacyHeaders(cors("https://app.example.com", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })))
	for _, origin := range []string{"https://app.example.com", "https://untrusted.example.com"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/accounts", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		for key, want := range map[string]string{"Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY"} {
			if got := response.Header().Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
	}
}

func TestRequestLogsOmitIdentifiersAndUnmatchedPaths(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/accounts/{id}", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := requestLogging(logger, mux)
	for _, path := range []string{"/api/v1/accounts/synthetic-private-identifier?token=synthetic-secret", "/unknown/synthetic-private-identifier"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if strings.Contains(logs.String(), "synthetic-private-identifier") || strings.Contains(logs.String(), "synthetic-secret") {
		t.Fatal("request details leaked into logs")
	}
	if !strings.Contains(logs.String(), "/api/v1/accounts/{id}") || !strings.Contains(logs.String(), "unmatched") {
		t.Fatal("expected safe route labels in logs")
	}
}
