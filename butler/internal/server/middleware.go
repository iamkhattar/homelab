package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/iamkhattar/homelab/butler/internal/access"
)

// AuthMiddleware validates JWT bearer tokens against an OIDC issuer.
type AuthMiddleware struct {
	issuer   string
	audience string
	allowed  map[string]struct{}
	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	caChain  func(context.Context) (string, error)
}

// NewAuthMiddleware creates a middleware that validates JWTs from the given issuer.
// Returns nil if issuer is empty (auth disabled).
func NewAuthMiddleware(ctx context.Context, issuer, audience string, caChain func(context.Context) (string, error)) (*AuthMiddleware, error) {
	_ = ctx
	if issuer == "" {
		return nil, nil
	}
	// Discovery is deliberately lazy: Pocket ID is deployed after Butler and
	// cannot be a process-start dependency during first-cluster bootstrap.
	if audience == "" {
		return nil, fmt.Errorf("OIDC audience is required when issuer is configured")
	}
	return &AuthMiddleware{issuer: issuer, audience: audience, allowed: map[string]struct{}{audience: {}}, caChain: caChain}, nil
}

// AllowAudiences adds explicitly configured public clients that may call the
// same Butler resource API. Signatures, issuer and expiry remain verified.
func (a *AuthMiddleware) AllowAudiences(audiences ...string) {
	for _, audience := range audiences {
		if audience != "" {
			a.allowed[audience] = struct{}{}
		}
	}
}

func (a *AuthMiddleware) verifierFor(ctx context.Context) (*oidc.IDTokenVerifier, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.verifier != nil {
		return a.verifier, nil
	}
	ctx = oidc.ClientContext(ctx, a.httpClient(ctx))
	provider, err := oidc.NewProvider(ctx, a.issuer)
	if err != nil {
		return nil, fmt.Errorf("discovering Pocket ID: %w", err)
	}
	// Multiple first-party public clients call Butler. Verify the signature,
	// issuer and expiry here, then enforce the exact audience allowlist below.
	a.verifier = provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
	return a.verifier, nil
}

func (a *AuthMiddleware) httpClient(ctx context.Context) *http.Client {
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if a.caChain != nil {
		if chain, err := a.caChain(ctx); err == nil {
			pool.AppendCertsFromPEM([]byte(chain))
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}
}

// Wrap returns an http.Handler that requires a valid JWT.
func (a *AuthMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearer(r)
		if token == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		verifier, err := a.verifierFor(r.Context())
		if err != nil {
			http.Error(w, "identity provider unavailable", http.StatusServiceUnavailable)
			return
		}
		idToken, err := verifier.Verify(r.Context(), token)
		if err != nil {
			slog.Warn("jwt verification failed", "error", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if !a.allowsAudience(idToken.Audience) {
			http.Error(w, "invalid token audience", http.StatusUnauthorized)
			return
		}
		var claims struct {
			Subject string   `json:"sub"`
			Email   string   `json:"email"`
			Groups  []string `json:"groups"`
		}
		if err := idToken.Claims(&claims); err != nil || claims.Subject == "" {
			http.Error(w, "invalid identity claims", http.StatusUnauthorized)
			return
		}
		principal := access.FromClaims(claims.Subject, claims.Email, claims.Groups)
		next.ServeHTTP(w, r.WithContext(access.WithPrincipal(r.Context(), principal)))
	})
}

func (a *AuthMiddleware) allowsAudience(tokenAudiences []string) bool {
	for _, audience := range tokenAudiences {
		if _, ok := a.allowed[audience]; ok {
			return true
		}
	}
	return false
}

// extractBearer returns the credential from the Authorization header only.
// The API is bearer-only: no cookie is ever accepted as authentication.
func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}
