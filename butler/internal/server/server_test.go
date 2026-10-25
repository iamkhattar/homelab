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

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/operations"
	"github.com/iamkhattar/homelab/butler/internal/orphans"
	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/reconciler"
	"github.com/iamkhattar/homelab/butler/internal/vault"
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

func TestOrphansReportsUnavailableWhenNotConfigured(t *testing.T) {
	srv := newTestServer(t, "tok")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orphans", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestOrphansReturnsProviderDriftJSON(t *testing.T) {
	srv := newTestServer(t, "tok")
	srv.ConfigureOrphans(orphans.New(stubDeclarations{}, stubPocketID{
		clients: []pocketid.OIDCClient{{Name: "retired-client"}},
	}, stubVault{
		policies: []string{"default", "root", "butler", "vso"},
	}, &config.Config{
		Namespace:   "security",
		OIDC:        config.OIDCConfig{Issuer: "https://auth.example.com"},
		K8sIssuance: config.K8sIssuanceConfig{Roles: []config.K8sIssuanceRoleSpec{{Name: "k8sadmin"}}},
	}))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/orphans", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var report orphans.Report
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decoding report: %v", err)
	}
	if len(report.Orphaned) != 1 || report.Orphaned[0].Name != "retired-client" {
		t.Fatalf("orphaned = %+v, want the retired client only", report.Orphaned)
	}
	for _, entry := range report.Orphaned {
		if entry.Kind != orphans.KindPocketIDClient {
			t.Fatalf("kind = %q", entry.Kind)
		}
	}
}

type stubDeclarations struct{}

func (stubDeclarations) ListPocketIDClients(context.Context) ([]platform.PocketIDClient, error) {
	return nil, nil
}

func (stubDeclarations) ListPocketIDGroups(context.Context) ([]platform.PocketIDGroup, error) {
	return nil, nil
}

func (stubDeclarations) ListVaultOIDCRoles(context.Context) ([]platform.VaultOIDCRole, error) {
	return nil, nil
}

type stubPocketID struct {
	clients []pocketid.OIDCClient
	groups  []pocketid.UserGroup
}

func (s stubPocketID) ListClients(context.Context) ([]pocketid.OIDCClient, error) {
	return s.clients, nil
}

func (s stubPocketID) ListGroups(context.Context) ([]pocketid.UserGroup, error) {
	return s.groups, nil
}

type stubVault struct {
	jwtRoles        []string
	kubernetesRoles []string
	policies        []string
}

func (s stubVault) ListJWTRoleNames(context.Context) ([]string, error) {
	return s.jwtRoles, nil
}

func (s stubVault) ListKubernetesRoleNames(context.Context) ([]string, error) {
	return s.kubernetesRoles, nil
}

func (s stubVault) ListACLPolicyNames(context.Context) ([]string, error) {
	return s.policies, nil
}

func TestKubernetesCredentialEndpointIsRemoved(t *testing.T) {
	// Cluster credentials are minted by Vault as the requesting user. Butler
	// must not expose a brokered credential route at all: a missing route is
	// the desired 404/405, never an authorization decision.
	srv := newTestServer(t, "tok")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/access/kubernetes-credentials", bytes.NewBufferString(`{"role":"k8sviewer","ttl":"15m"}`)))
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 404/405 for the removed credential endpoint", rec.Code)
	}
}
