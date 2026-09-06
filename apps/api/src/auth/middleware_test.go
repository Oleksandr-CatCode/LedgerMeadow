package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	shared "ledgermeadow/src/shared/types"
)

type unusedResolver struct{}

func (unusedResolver) ResolveByClerkID(context.Context, string) (shared.UserID, error) {
	panic("resolver must not be called without verified claims")
}

func TestRequireUserRejectsRequestWithoutVerifiedClaims(t *testing.T) {
	called := false
	middleware := &Middleware{
		resolver: unusedResolver{},
		clerk:    func(next http.Handler) http.Handler { return next },
	}
	handler := middleware.RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/v1/subscriptions/id", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if called {
		t.Fatal("unauthenticated request reached the protected handler")
	}
}
