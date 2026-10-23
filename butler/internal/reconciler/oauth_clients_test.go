package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// clientFixture stubs the Pocket ID OIDC client admin API.
type clientFixture struct {
	clients map[string]pocketid.OIDCClient // keyed by client name
	byID    map[string]string              // provider client id -> name
	secrets map[string][]pocketid.OIDCClientSecret
	seq     int
}

func newClientServer(t *testing.T, log *callLog, clients ...pocketid.OIDCClient) *httptest.Server {
	t.Helper()
	fixture := &clientFixture{
		clients: map[string]pocketid.OIDCClient{},
		byID:    map[string]string{},
		secrets: map[string][]pocketid.OIDCClientSecret{},
	}
	for _, c := range clients {
		fixture.clients[c.Name] = c
		fixture.byID[c.ID] = c.Name
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && path == "/api/oidc/clients":
			log.add(http.MethodGet, "/api/oidc/clients", "")
			var data []pocketid.OIDCClient
			for _, c := range fixture.clients {
				data = append(data, c)
			}
			writeJSON(w, map[string]any{"data": data})

		case r.Method == http.MethodPost && path == "/api/oidc/clients":
			var in pocketid.OIDCClient
			_ = json.Unmarshal(raw, &in)
			fixture.seq++
			in.ID = fmt.Sprintf("client-%d", fixture.seq)
			fixture.clients[in.Name] = in
			fixture.byID[in.ID] = in.Name
			log.add(http.MethodPost, "/api/oidc/clients", string(raw))
			writeJSON(w, in)

		case r.Method == http.MethodPut && strings.HasPrefix(path, "/api/oidc/clients/"):
			id := strings.TrimPrefix(path, "/api/oidc/clients/")
			var in pocketid.OIDCClient
			_ = json.Unmarshal(raw, &in)
			in.ID = id
			if name, ok := fixture.byID[id]; ok {
				fixture.clients[name] = in
			}
			log.add(http.MethodPut, "/api/oidc/clients/"+id, string(raw))
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPost && strings.HasSuffix(path, "/secrets"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/oidc/clients/"), "/secrets")
			fixture.seq++
			secret := pocketid.OIDCClientSecret{ID: fmt.Sprintf("secret-%d", fixture.seq), Secret: "generated-secret-value", IsActive: true}
			fixture.secrets[id] = append(fixture.secrets[id], secret)
			log.add(http.MethodPost, "/api/oidc/clients/"+id+"/secrets", "")
			writeJSON(w, secret)

		case r.Method == http.MethodGet && strings.HasSuffix(path, "/secrets"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/oidc/clients/"), "/secrets")
			log.add(http.MethodGet, "/api/oidc/clients/"+id+"/secrets", "")
			writeJSON(w, fixture.secrets[id])

		case r.Method == http.MethodDelete && strings.Contains(path, "/secrets/"):
			rest := strings.TrimPrefix(path, "/api/oidc/clients/")
			id, secretID, _ := strings.Cut(rest, "/secrets/")
			var kept []pocketid.OIDCClientSecret
			for _, s := range fixture.secrets[id] {
				if s.ID != secretID {
					kept = append(kept, s)
				}
			}
			fixture.secrets[id] = kept
			log.add(http.MethodDelete, "/api/oidc/clients/"+id+"/secrets/"+secretID, "")
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// vaultFixture answers KV-v2 reads from a map and can fail specific writes, so
// compensating-cleanup paths can be exercised.
type vaultFixture struct {
	secrets    map[string]map[string]any
	failWrites map[string]bool
}

func newVaultFixture() *vaultFixture {
	return &vaultFixture{secrets: map[string]map[string]any{}, failWrites: map[string]bool{}}
}

func (f *vaultFixture) handler(method, path string, body []byte) (int, []byte) {
	switch method {
	case http.MethodGet:
		data, ok := f.secrets[path]
		if !ok {
			return http.StatusNotFound, nil
		}
		return http.StatusOK, kvSecret(data)
	case http.MethodPost, http.MethodPut:
		if f.failWrites[path] {
			return http.StatusInternalServerError, []byte(`{"errors":["vault is sealed"]}`)
		}
		// The KV-v2 client verifies a write by reading the path back, so the
		// fake has to retain what was written. The request body is either the
		// raw map or wrapped in a data envelope depending on the client
		// version, so accept both.
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return http.StatusBadRequest, nil
		}
		stored := raw
		if envelope, ok := raw["data"].(map[string]any); ok {
			stored = envelope
		}
		f.secrets[path] = stored
		// Real Vault answers a KV-v2 write with the updated secret document.
		// Returning an empty 204 makes the client treat the write as not found.
		response, _ := json.Marshal(map[string]any{
			"data": map[string]any{"data": stored, "metadata": map[string]any{"version": 1, "destroyed": false}},
		})
		return http.StatusOK, response
	default:
		return http.StatusMethodNotAllowed, nil
	}
}

func clientCR(namespace, name, clientType string, vaultPath string, redirectURIs ...string) *unstructured.Unstructured {
	return unstructuredClient(namespace, name, clientType, redirectURIs, vaultPath)
}

func withMachineCredential(f *vaultFixture, apiKey string) {
	f.secrets[pocketid.ManagementCredentialVaultPath] = map[string]any{pocketid.ManagementCredentialField: apiKey}
}

func TestPocketIDClientsCreatesConfidentialClientAndPersistsSecret(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")

	store := newPlatformStore(clientCR("vaultwarden", "vaultwarden", "confidential", "applications/vaultwarden/oidc", "https://vault.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	create := log.indexOf(http.MethodPost, "/api/oidc/clients")
	secret := log.indexOf(http.MethodPost, "/api/oidc/clients/client-1/secrets")
	write := log.indexOf("vault:PUT", "applications/vaultwarden/oidc")
	if create < 0 || secret < 0 || write < 0 {
		t.Fatalf("expected client creation, secret creation and a Vault write, got:%v", log)
	}
	if !(create < secret && secret < write) {
		t.Fatalf("the generated secret must be persisted before the run reports success, got:%v", log)
	}

	status := statusOf(t, store, "PocketIDClient")["vaultwarden/vaultwarden"]
	if conditionReason(status) != "Reconciled" {
		t.Fatalf("expected Reconciled, got %q", conditionReason(status))
	}
	if status.ProviderID != "client-1" {
		t.Fatalf("expected the provider ID in status, got %q", status.ProviderID)
	}
}

func TestPocketIDClientsRevokesNewSecretWhenVaultWriteFails(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")
	vault.failWrites["applications/homepage/oidc"] = true

	store := newPlatformStore(clientCR("homepage", "homepage", "confidential", "applications/homepage/oidc", "https://home.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected the Vault write failure to fail the run")
	}
	if log.indexOf(http.MethodDelete, "/api/oidc/clients/client-1/secrets/secret-2") < 0 {
		t.Fatalf("a credential that never reached Vault must be revoked, got:%v", log)
	}
	if reason := conditionReason(statusOf(t, store, "PocketIDClient")["homepage/homepage"]); reason != "ReconcileFailed" {
		t.Fatalf("expected ReconcileFailed, got %q", reason)
	}
}

func TestPocketIDClientsPersistsReplacementBeforeRevokingOlderSecrets(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log, pocketid.OIDCClient{
		ID: "client-9", Name: "grafana", IsPublic: false, PKCEEnabled: false,
		CallbackURLs: []string{"https://grafana.6940469.xyz/login/generic_oauth"},
	}).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")
	// The stored secret id no longer exists in Pocket ID, so a replacement must
	// be created. The stale older secret must only be revoked afterwards.
	vault.secrets["monitoring/grafana/oidc"] = map[string]any{"client_id": "client-9", "client_secret_id": "missing-secret"}

	store := newPlatformStore(clientCR("monitoring", "grafana", "confidential", "monitoring/grafana/oidc", "https://grafana.6940469.xyz/login/generic_oauth"))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	created := log.indexOf(http.MethodPost, "/api/oidc/clients/client-9/secrets")
	write := log.indexOf("vault:PUT", "monitoring/grafana/oidc")
	deleted := log.indexOf(http.MethodDelete, "/api/oidc/clients/client-9/secrets/secret-1")
	if created < 0 || write < 0 {
		t.Fatalf("expected a replacement secret and a Vault write, got:%v", log)
	}
	if deleted >= 0 && deleted < write {
		t.Fatalf("the previous secret must not be revoked before the replacement is persisted, got:%v", log)
	}
}

func TestPocketIDClientsPublicClientNeverStoresASecret(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")

	store := newPlatformStore(clientCR("homepage", "homepage", "public", "applications/homepage/oidc", "https://home.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	for _, call := range log.mutations() {
		if strings.HasSuffix(call.Path, "/secrets") && call.Method == http.MethodPost {
			t.Fatalf("a public client must not create a client secret, got:%v", log)
		}
	}
	for _, call := range log.snapshot() {
		if strings.Contains(call.Body, "client_secret") {
			t.Fatalf("a public client must not write a client secret to Vault: %s", call.Body)
		}
	}
}

func TestPocketIDClientsIsIdempotentWhenProviderStateMatches(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log, pocketid.OIDCClient{
		ID: "client-3", Name: "butler", IsPublic: true, PKCEEnabled: true,
		CallbackURLs: []string{"https://butler.6940469.xyz/callback"},
	}).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")
	// The stored credential already matches the provider exactly, so the run
	// must not create, update, revoke or rewrite anything.
	vault.secrets["security/butler/oidc"] = map[string]any{"client_id": "client-3"}

	store := newPlatformStore(clientCR("security", "butler", "public", "security/butler/oidc", "https://butler.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("a converged public client must cause no mutation, got:%v", log)
	}
}

func TestPocketIDClientsConvergesCallbackURLDrift(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log, pocketid.OIDCClient{
		ID: "client-7", Name: "vaultwarden", IsPublic: false,
		CallbackURLs: []string{"https://old.6940469.xyz/callback"},
	}).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")
	vault.secrets["applications/vaultwarden/oidc"] = map[string]any{"client_id": "client-7", "client_secret_id": "secret-8"}

	store := newPlatformStore(clientCR("vaultwarden", "vaultwarden", "confidential", "applications/vaultwarden/oidc", "https://new.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if log.indexOf(http.MethodPut, "/api/oidc/clients/client-7") < 0 {
		t.Fatalf("changed callback URLs must be pushed to the provider, got:%v", log)
	}
}

func TestPocketIDClientsRejectsDuplicateProviderName(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")

	store := newPlatformStore(
		clientCR("homepage", "shared", "public", "applications/homepage/oidc", "https://home.6940469.xyz/callback"),
		clientCR("vaultwarden", "shared", "public", "applications/vaultwarden/oidc", "https://vault.6940469.xyz/callback"),
	)
	vc := newVaultServer(t, vault.handler, log)

	err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected duplicate provider names to fail the run")
	}
	for _, status := range statusOf(t, store, "PocketIDClient") {
		if conditionReason(status) != "DuplicateProviderName" {
			t.Fatalf("expected DuplicateProviderName, got %q", conditionReason(status))
		}
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("a rejected resource must not mutate the provider, got:%v", log)
	}
}

func TestPocketIDClientsRejectsDuplicateVaultPath(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")

	store := newPlatformStore(
		clientCR("homepage", "homepage", "public", "applications/shared/oidc", "https://home.6940469.xyz/callback"),
		clientCR("vaultwarden", "vaultwarden", "public", "applications/shared/oidc", "https://vault.6940469.xyz/callback"),
	)
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err == nil {
		t.Fatal("expected duplicate Vault paths to fail the run")
	}
	for _, status := range statusOf(t, store, "PocketIDClient") {
		if conditionReason(status) != "DuplicateVaultPath" {
			t.Fatalf("expected DuplicateVaultPath, got %q", conditionReason(status))
		}
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("a rejected resource must not mutate the provider, got:%v", log)
	}
}

func TestPocketIDClientsRejectsInvalidSpec(t *testing.T) {
	t.Parallel()
	for name, object := range map[string]*unstructured.Unstructured{
		"unknown type": clientCR("homepage", "homepage", "confidential-x", "applications/homepage/oidc", "https://home.6940469.xyz/callback"),
		"no redirect":  clientCR("homepage", "homepage", "confidential", "applications/homepage/oidc"),
		"empty vault":  clientCR("homepage", "homepage", "confidential", "", "https://home.6940469.xyz/callback"),
	} {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			pidURL := newClientServer(t, log).URL
			vault := newVaultFixture()
			withMachineCredential(vault, "machine-key")
			vc := newVaultServer(t, vault.handler, log)

			if err := NewPocketIDClients(vc, newPlatformStore(object), pidURL).Reconcile(context.Background()); err == nil {
				t.Fatal("expected invalid spec to fail the run")
			}
			if got := log.mutations(); len(got) != 0 {
				t.Fatalf("invalid spec must not mutate the provider, got:%v", log)
			}
		})
	}
}

func TestPocketIDClientsWaitsForMachineCredential(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()

	store := newPlatformStore(clientCR("homepage", "homepage", "public", "applications/homepage/oidc", "https://home.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("a missing machine credential must not fail the run: %v", err)
	}
	if log.indexOf(http.MethodGet, "/api/oidc/clients") >= 0 {
		t.Fatalf("the provider must not be called without a machine credential, got:%v", log)
	}
	if reason := conditionReason(statusOf(t, store, "PocketIDClient")["homepage/homepage"]); reason != "AwaitingAPIKey" {
		t.Fatalf("expected AwaitingAPIKey, got %q", reason)
	}
}

func TestPocketIDClientsNeverLeaksSecretsIntoErrorsOrStatus(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	pidURL := newClientServer(t, log).URL
	vault := newVaultFixture()
	withMachineCredential(vault, "machine-key")
	vault.failWrites["applications/homepage/oidc"] = true

	store := newPlatformStore(clientCR("homepage", "homepage", "confidential", "applications/homepage/oidc", "https://home.6940469.xyz/callback"))
	vc := newVaultServer(t, vault.handler, log)

	err := NewPocketIDClients(vc, store, pidURL).Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected a failure")
	}
	if strings.Contains(err.Error(), "generated-secret-value") || strings.Contains(err.Error(), "machine-key") {
		t.Fatalf("error text leaked credential material: %v", err)
	}
	status := statusOf(t, store, "PocketIDClient")["homepage/homepage"]
	if msg := conditionMessage(status); strings.Contains(msg, "generated-secret-value") || strings.Contains(msg, "machine-key") {
		t.Fatalf("status message leaked credential material: %q", msg)
	}
}
