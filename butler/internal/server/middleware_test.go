package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/access"
)

func TestExtractBearer(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"empty header", "", ""},
		{"no bearer prefix", "Basic abc123", ""},
		{"bearer only", "Bearer ", ""},
		{"case sensitive", "bearer abc123", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := extractBearer(r)
			if got != tt.want {
				t.Errorf("extractBearer() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractBearerIgnoresCookies(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "butler_session", Value: "cookie-token"})
	if got := extractBearer(r); got != "" {
		t.Fatalf("extractBearer() = %q, want no cookie fallback", got)
	}
}

func TestWrapRejectsCookieOnlyRequest(t *testing.T) {
	// Regression guard: the API is bearer-only. A request authenticated only by
	// the removed butler_session cookie must never reach a handler.
	auth, err := NewAuthMiddleware(t.Context(), "https://issuer.example.test", []string{"homelabctl"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	handler := auth.Wrap(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.AddCookie(&http.Cookie{Name: "butler_session", Value: "cookie-token"})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	if called {
		t.Fatal("handler ran for a cookie-only request")
	}
}

func TestRequireEnforcesRoleHierarchy(t *testing.T) {
	srv := &Server{auth: nil}
	handler := srv.require(access.Admin, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(access.WithPrincipal(req.Context(), access.Principal{Subject: "viewer", Role: access.Viewer}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
}

func TestAuthMiddleware_NilPassesThrough(t *testing.T) {
	// When auth is nil, protected() should return the handler directly.
	srv := &Server{auth: nil}
	called := false
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	})

	wrapped := srv.protected(handler)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Error("handler should have been called when auth is nil")
	}
}

func TestAuthMiddlewareClientAllowlist(t *testing.T) {
	auth, err := NewAuthMiddleware(t.Context(), "https://issuer.example.test", []string{"homelabctl"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !auth.allowsAudience([]string{"homelabctl"}) {
		t.Fatal("configured client audience rejected")
	}
	// The butler client no longer exists. A token minted for any other client -
	// including one named after this service - must not authenticate here.
	for _, audience := range []string{"butler", "vault", "grafana", "untrusted-client"} {
		if auth.allowsAudience([]string{audience}) {
			t.Fatalf("audience %q accepted but not configured", audience)
		}
	}
}

func TestAuthMiddlewareRequiresAtLeastOneClient(t *testing.T) {
	if _, err := NewAuthMiddleware(t.Context(), "https://issuer.example.test", nil, nil); err == nil {
		t.Fatal("expected an error when no client is configured")
	}
}
