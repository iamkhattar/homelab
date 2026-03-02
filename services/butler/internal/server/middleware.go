package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// AuthMiddleware validates JWT bearer tokens against an OIDC issuer.
type AuthMiddleware struct {
	verifier *oidc.IDTokenVerifier
}

// NewAuthMiddleware creates a middleware that validates JWTs from the given issuer.
// Returns nil if issuer is empty (auth disabled).
func NewAuthMiddleware(ctx context.Context, issuer string) (*AuthMiddleware, error) {
	if issuer == "" {
		return nil, nil
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: true,
	})

	return &AuthMiddleware{verifier: verifier}, nil
}

// Wrap returns an http.Handler that requires a valid JWT.
func (a *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		if _, err := a.verifier.Verify(r.Context(), token); err != nil {
			slog.Warn("jwt verification failed", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}
