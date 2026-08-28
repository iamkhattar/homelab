package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/iamkhattar/homelab/butler/internal/access"
	"golang.org/x/oauth2"
)

// AuthMiddleware validates JWT bearer tokens against an OIDC issuer.
type AuthMiddleware struct {
	issuer   string
	audience string
	allowed  map[string]struct{}
	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
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

func (a *AuthMiddleware) clientsFor(ctx context.Context) (*oidc.IDTokenVerifier, *oauth2.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.verifier != nil && a.oauth != nil {
		return a.verifier, a.oauth, nil
	}
	ctx = oidc.ClientContext(ctx, a.httpClient(ctx))
	provider, err := oidc.NewProvider(ctx, a.issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("discovering Pocket ID: %w", err)
	}
	// Multiple first-party public clients call Butler. Verify the signature,
	// issuer and expiry here, then enforce the exact audience allowlist below.
	a.verifier = provider.Verifier(&oidc.Config{SkipClientIDCheck: true})
	a.oauth = &oauth2.Config{
		ClientID:    a.audience,
		Endpoint:    provider.Endpoint(),
		RedirectURL: "https://butler.home.6940469.xyz/auth/callback",
		Scopes:      []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}
	return a.verifier, a.oauth, nil
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

		verifier, _, err := a.clientsFor(r.Context())
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

// Login starts Pocket ID's authorization-code flow with PKCE. The verifier
// and state are short-lived, HTTP-only cookies; no client secret is needed.
func (a *AuthMiddleware) Login(w http.ResponseWriter, r *http.Request) {
	_, cfg, err := a.clientsFor(r.Context())
	if err != nil {
		http.Error(w, "identity provider unavailable", http.StatusServiceUnavailable)
		return
	}
	state, err := randomURLToken(32)
	if err != nil {
		http.Error(w, "creating login state failed", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()
	setAuthCookie(w, "butler_oidc_state", state, 600)
	setAuthCookie(w, "butler_oidc_verifier", verifier, 600)
	http.Redirect(w, r, cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (a *AuthMiddleware) Callback(w http.ResponseWriter, r *http.Request) {
	stateCookie, stateErr := r.Cookie("butler_oidc_state")
	verifierCookie, verifierErr := r.Cookie("butler_oidc_verifier")
	state := r.URL.Query().Get("state")
	if stateErr != nil || verifierErr != nil || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(stateCookie.Value)) != 1 {
		http.Error(w, "invalid or expired OIDC state", http.StatusBadRequest)
		return
	}
	verifier, cfg, err := a.clientsFor(r.Context())
	if err != nil {
		http.Error(w, "identity provider unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx := oidc.ClientContext(r.Context(), a.httpClient(r.Context()))
	token, err := cfg.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifierCookie.Value))
	if err != nil {
		http.Error(w, "OIDC code exchange failed", http.StatusUnauthorized)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "OIDC response omitted id_token", http.StatusUnauthorized)
		return
	}
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		http.Error(w, "OIDC token verification failed", http.StatusUnauthorized)
		return
	}
	if !slices.Contains(idToken.Audience, a.audience) {
		http.Error(w, "OIDC token audience did not match Butler", http.StatusUnauthorized)
		return
	}
	maxAge := int(time.Until(idToken.Expiry).Seconds())
	if maxAge < 1 {
		http.Error(w, "OIDC token already expired", http.StatusUnauthorized)
		return
	}
	setAuthCookie(w, "butler_session", rawIDToken, maxAge)
	clearAuthCookie(w, "butler_oidc_state")
	clearAuthCookie(w, "butler_oidc_verifier")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *AuthMiddleware) Logout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookie(w, "butler_session")
	http.Redirect(w, r, "/", http.StatusFound)
}

func randomURLToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func setAuthCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func clearAuthCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if cookie, err := r.Cookie("butler_session"); err == nil {
		return cookie.Value
	}
	return ""
}
