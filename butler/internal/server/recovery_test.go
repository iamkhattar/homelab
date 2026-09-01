package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRecoveryAuth struct{ actor string }

func (f fakeRecoveryAuth) Authorize(context.Context, string) (string, error) { return f.actor, nil }

func TestRecoveryRejectsUnauthenticatedRequestBeforeServiceAccess(t *testing.T) {
	srv := NewRecovery(fakeRecoveryAuth{}, nil)
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/recovery/status", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestRecoveryUIHasNoIngressAuthenticationDependency(t *testing.T) {
	srv := NewRecovery(fakeRecoveryAuth{}, nil)
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	for _, expected := range []string{"Butler break-glass recovery", "homelabctl control recovery ui", "browser never receives that token"} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("recovery UI omitted %q", expected)
		}
	}
	if strings.Contains(recorder.Body.String(), "Short-lived recovery token</label>") {
		t.Fatal("recovery UI asks the browser to hold the Kubernetes token")
	}
	if !strings.Contains(recorder.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Fatal("recovery UI omitted anti-framing Content-Security-Policy")
	}
}
