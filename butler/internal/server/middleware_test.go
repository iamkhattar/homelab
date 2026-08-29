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

func TestAuthMiddlewareAudienceAllowlist(t *testing.T) {
	auth, err := NewAuthMiddleware(t.Context(), "https://issuer.example.test", "butler", nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.AllowAudiences("homelabctl")
	if !auth.allowsAudience([]string{"homelabctl"}) || !auth.allowsAudience([]string{"butler"}) {
		t.Fatal("configured first-party audience rejected")
	}
	if auth.allowsAudience([]string{"untrusted-client"}) {
		t.Fatal("unconfigured audience accepted")
	}
}
