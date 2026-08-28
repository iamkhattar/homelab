package server

import (
	"context"
	"net/http"
	"net/http/httptest"
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
}
