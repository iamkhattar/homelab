package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/access"
	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/operations"
	"github.com/iamkhattar/homelab/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

type serverCredentialIssuer struct{}

func (serverCredentialIssuer) IssueKubernetesCredential(_ context.Context, role, namespace string, ttl time.Duration) (vault.KubernetesCredential, error) {
	return vault.KubernetesCredential{Role: role, Namespace: namespace, ServiceAccount: "issued", Token: "secret-token", TTLSeconds: int64(ttl.Seconds()), ExpiresAt: time.Now().Add(ttl)}, nil
}

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

func TestRootReturnsPlainTextPointer(t *testing.T) {
	srv := newTestServer(t, "tok")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", contentType)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", rec.Header().Get("X-Content-Type-Options"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "homelabctl control") || !strings.Contains(body, "/api/v1") {
		t.Fatalf("pointer = %q, want homelabctl and API guidance", body)
	}
	for _, forbidden := range []string{"<html", "<script", "<style", "/assets/", "/auth/login"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("pointer contains browser surface %q", forbidden)
		}
	}
}

func TestBrowserSurfacesAreNotServed(t *testing.T) {
	srv := newTestServer(t, "tok")
	for _, path := range []string{"/assets/butler.css", "/assets/butler.js", "/auth/login", "/auth/callback"} {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		contentType := rec.Header().Get("Content-Type")
		for _, browserType := range []string{"text/html", "text/css", "text/javascript", "application/javascript"} {
			if strings.HasPrefix(contentType, browserType) {
				t.Fatalf("%s served %s", path, contentType)
			}
		}
		if rec.Code == http.StatusOK && !strings.HasPrefix(contentType, "text/plain") {
			t.Fatalf("%s status = %d, content type = %q", path, rec.Code, contentType)
		}
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

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}

	var resp operations.Operation
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}
	if resp.Kind != "reconcile" {
		t.Errorf("expected reconcile operation, got %#v", resp)
	}
}

func TestKubernetesCredentialEndpointDoesNotAuditToken(t *testing.T) {
	srv := newTestServer(t, "tok")
	service, err := access.NewCredentialService(serverCredentialIssuer{}, config.K8sIssuanceConfig{
		HostNamespace: "kube-system",
		Roles:         []config.K8sIssuanceRoleSpec{{Name: "homelab-viewer", MaxTTL: "1h"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.ConfigureControlPlane(nil, service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/access/kubernetes-credentials", bytes.NewBufferString(`{"role":"homelab-viewer","ttl":"15m"}`))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	for _, event := range srv.operations.Events() {
		if bytes.Contains([]byte(event.Message), []byte("secret-token")) {
			t.Fatalf("credential leaked into event: %#v", event)
		}
	}
}
