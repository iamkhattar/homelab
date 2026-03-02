package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iamkhattar/homelab/services/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/services/butler/internal/vault"
)

func newTestServer(t *testing.T, token string) *Server {
	t.Helper()
	vc, err := vault.NewClient("http://127.0.0.1:0") // no real connection needed
	if err != nil {
		t.Fatalf("creating vault client: %v", err)
	}
	if token != "" {
		vc.SetToken(token)
	}
	sched := reconciler.NewScheduler(time.Minute)
	return New(sched, vc, nil)
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected 'ok', got %q", rec.Body.String())
	}
}

func TestReadyz_WithToken(t *testing.T) {
	srv := newTestServer(t, "test-token")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestReadyz_NoToken(t *testing.T) {
	srv := newTestServer(t, "")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestStatus_ReturnsJSON(t *testing.T) {
	srv := newTestServer(t, "tok")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var statuses []reconciler.Status
	if err := json.NewDecoder(rec.Body).Decode(&statuses); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
}

func TestReconcile_Success(t *testing.T) {
	srv := newTestServer(t, "tok")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/reconcile", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}
