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

func TestRecoveryRootReturnsPlainTextPointer(t *testing.T) {
	srv := NewRecovery(fakeRecoveryAuth{}, nil)
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", contentType)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", recorder.Header().Get("X-Content-Type-Options"))
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "homelabctl control recovery") || !strings.Contains(body, "/api/v1") {
		t.Fatalf("pointer = %q, want CLI and API guidance", body)
	}
	for _, forbidden := range []string{"<html", "<script", "<style", "Short-lived recovery token</label>"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("pointer contains browser surface %q", forbidden)
		}
	}
}
