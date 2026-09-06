package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"

	shared "ledgermeadow/src/shared/types"
)

type UserResolver interface {
	ResolveByClerkID(ctx context.Context, clerkUserID string) (shared.UserID, error)
}

type userContextKey struct{}

type Middleware struct {
	resolver UserResolver
	clerk    func(http.Handler) http.Handler
}

func NewMiddleware(secretKey string, authorizedParties []string, resolver UserResolver) *Middleware {
	clerk.SetKey(secretKey)
	return &Middleware{
		resolver: resolver,
		clerk: clerkhttp.WithHeaderAuthorization(
			clerkhttp.AuthorizedPartyMatches(authorizedParties...),
		),
	}
}

func (m *Middleware) RequireUser(next http.Handler) http.Handler {
	resolveUser := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		claims, ok := clerk.SessionClaimsFromContext(request.Context())
		if !ok || claims == nil || claims.Subject == "" {
			writeAuthError(response)
			return
		}
		userID, err := m.resolver.ResolveByClerkID(request.Context(), claims.Subject)
		if err != nil {
			http.Error(response, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		contextWithUser := context.WithValue(request.Context(), userContextKey{}, userID)
		next.ServeHTTP(response, request.WithContext(contextWithUser))
	})
	return m.clerk(resolveUser)
}

func UserIDFromContext(ctx context.Context) (shared.UserID, bool) {
	userID, ok := ctx.Value(userContextKey{}).(shared.UserID)
	return userID, ok
}

func ContextWithUserID(ctx context.Context, userID shared.UserID) context.Context {
	return context.WithValue(ctx, userContextKey{}, userID)
}

func writeAuthError(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(response).Encode(map[string]string{
		"code":    "UNAUTHENTICATED",
		"message": "Sign in is required.",
	})
}
