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

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// clientFixture stubs the Pocket ID OIDC client admin API.
type clientFixture struct {
	clients map[string]pocketid.OIDCClient // keyed by client name
	byID    map[string]string              // provider client id -> name
	secrets map[string][]pocketid.OIDCClientSecret
	groups  map[string]pocketid.UserGroup // keyed by group name
	seq     int
}

// platformGroups is the provider group state the butler chart declares. The
// client reconciler resolves allowedUserGroups names against Pocket ID, so the
// fixture serves the same names the chart provisions.
func platformGroups() []pocketid.UserGroup {
	return []pocketid.UserGroup{
		{ID: "group-butleradmin", Name: "butleradmin", FriendlyName: "Homelab administrators"},
		{ID: "group-vaultadmin", Name: "vaultadmin", FriendlyName: "Homelab operators"},
		{ID: "group-butlerviewer", Name: "butlerviewer", FriendlyName: "Homelab viewers"},
		{ID: "group-household", Name: "household", FriendlyName: "Household members who use the shared applications"},
	}
}

func newClientServer(t *testing.T, log *callLog, clients ...pocketid.OIDCClient) *httptest.Server {
	t.Helper()
	return newClientServerWithGroups(t, log, platformGroups(), clients...)
}

func newClientServerWithGroups(t *testing.T, log *callLog, groups []pocketid.UserGroup, clients ...pocketid.OIDCClient) *httptest.Server {
	t.Helper()
	fixture := &clientFixture{
		clients: map[string]pocketid.OIDCClient{},
		byID:    map[string]string{},
		secrets: map[string][]pocketid.OIDCClientSecret{},
		groups:  map[string]pocketid.UserGroup{},
	}
	for _, c := range clients {
		fixture.clients[c.Name] = c
		fixture.byID[c.ID] = c.Name
	}
	for _, g := range groups {
		fixture.groups[g.Name] = g
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

		case r.Method == http.MethodGet && path == "/api/user-groups":
			log.add(http.MethodGet, "/api/user-groups", "")
			var data []pocketid.UserGroup
			for _, g := range fixture.groups {
				data = append(data, g)
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

		// Pocket ID sets the sign-in restriction on its own endpoint, so it
		// must be matched before the generic client update below.
		case r.Method == http.MethodPut && strings.HasSuffix(path, "/allowed-user-groups"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/api/oidc/clients/"), "/allowed-user-groups")
			var in struct {
				UserGroupIDs []string `json:"userGroupIds"`
			}
			_ = json.Unmarshal(raw, &in)
			if name, ok := fixture.byID[id]; ok {
				c := fixture.clients[name]
				c.AllowedUserGroups = nil
				for _, groupID := range in.UserGroupIDs {
					c.AllowedUserGroups = append(c.AllowedUserGroups, fixture.groupByID(groupID))
				}
				fixture.clients[name] = c
			}
			log.add(http.MethodPut, "/api/oidc/clients/"+id+"/allowed-user-groups", string(raw))
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPut && strings.HasPrefix(path, "/api/oidc/clients/"):
			id := strings.TrimPrefix(path, "/api/oidc/clients/")
			var in pocketid.OIDCClient
			_ = json.Unmarshal(raw, &in)
			in.ID = id
			if name, ok := fixture.byID[id]; ok {
				// The client update payload does not carry the restriction.
				in.AllowedUserGroups = fixture.clients[name].AllowedUserGroups
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

// groupByID resolves a provider group id back to its representation, so a
// subsequent read reflects what was written.
func (f *clientFixture) groupByID(id string) pocketid.UserGroup {
	for _, g := range f.groups {
		if g.ID == id {
			return g
		}
	}
	return pocketid.UserGroup{ID: id}
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

// clientCRWithAccess builds a PocketIDClient that declares a sign-in
// restriction. A nil slice omits spec.allowedUserGroups entirely, while a
// non-nil empty slice declares an explicit "no group may sign in" restriction.
func clientCRWithAccess(namespace, name, clientType, vaultPath string, allowedUserGroups []string, redirectURIs ...string) *unstructured.Unstructured {
	object := unstructuredClient(namespace, name, clientType, redirectURIs, vaultPath)
	if allowedUserGroups != nil {
		if err := unstructured.SetNestedStringSlice(object.Object, allowedUserGroups, "spec", "allowedUserGroups"); err != nil {
			panic(err)
		}
	}
	return object
}

// accessFixture is the provider and secret state shared by the allowed-group
// tests: a public client that already matches the declared redirect URIs, so
// only the sign-in restriction can produce a mutation.
type accessFixture struct {
	log    *callLog
	store  *platform.Store
	vault  *vault.Client
	pidURL string
}

func newAccessFixture(t *testing.T, current []pocketid.UserGroup, groups []pocketid.UserGroup, object *unstructured.Unstructured) *accessFixture {
	t.Helper()
	log := &callLog{}
	pidURL := newClientServerWithGroups(t, log, groups, pocketid.OIDCClient{
		ID: "client-1", Name: object.GetName(), IsPublic: true, PKCEEnabled: true,
		CallbackURLs: []string{"https://home.6940469.xyz/callback"}, AllowedUserGroups: current,
	}).URL
	vf := newVaultFixture()
	withMachineCredential(vf, "machine-key")
	vf.secrets["oauth/"+object.GetName()] = map[string]any{"client_id": "client-1"}
	return &accessFixture{log: log, store: newPlatformStore(object), vault: newVaultServer(t, vf.handler, log), pidURL: pidURL}
}

func (f *accessFixture) reconcile() error {
	return NewPocketIDClients(f.vault, f.store, f.pidURL).Reconcile(context.Background())
}

func (f *accessFixture) allowedGroupCalls() []providerCall {
	var out []providerCall
	for _, call := range f.log.snapshot() {
		if strings.HasSuffix(call.Path, "/allowed-user-groups") {
			out = append(out, call)
		}
	}
	return out
}

func TestPocketIDClientsPushesAllowedUserGroupsWhenTheyDrift(t *testing.T) {
	t.Parallel()
	f := newAccessFixture(t,
		[]pocketid.UserGroup{{ID: "group-butleradmin", Name: "butleradmin", FriendlyName: "Homelab administrators"}},
		platformGroups(),
		clientCRWithAccess("homepage", "homepage", "public", "oauth/homepage",
			[]string{"butleradmin", "butlerviewer"}, "https://home.6940469.xyz/callback"))

	if err := f.reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	calls := f.allowedGroupCalls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly one allowed-user-groups update, got:%v", f.log)
	}
	if calls[0].Method != http.MethodPut || calls[0].Path != "/api/oidc/clients/client-1/allowed-user-groups" {
		t.Fatalf("unexpected request: %#v", calls[0])
	}
	if want := `{"userGroupIds":["group-butleradmin","group-butlerviewer"]}`; calls[0].Body != want {
		t.Fatalf("body = %s, want %s", calls[0].Body, want)
	}
	if reason := conditionReason(statusOf(t, f.store, "PocketIDClient")["homepage/homepage"]); reason != "Reconciled" {
		t.Fatalf("expected Reconciled, got %q", reason)
	}
}

func TestPocketIDClientsSkipsAllowedUserGroupsWhenIDsMatchInAnyOrder(t *testing.T) {
	t.Parallel()
	f := newAccessFixture(t,
		// Provider order differs from the declared order; the comparison is
		// set-based, so nothing may be written.
		[]pocketid.UserGroup{
			{ID: "group-butlerviewer", Name: "butlerviewer", FriendlyName: "Homelab viewers"},
			{ID: "group-butleradmin", Name: "butleradmin", FriendlyName: "Homelab administrators"},
		},
		platformGroups(),
		clientCRWithAccess("homepage", "homepage", "public", "oauth/homepage",
			[]string{"butleradmin", "butlerviewer"}, "https://home.6940469.xyz/callback"))

	if err := f.reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if calls := f.allowedGroupCalls(); len(calls) != 0 {
		t.Fatalf("a matching restriction must not be rewritten, got:%#v", calls)
	}
	if got := f.log.mutations(); len(got) != 0 {
		t.Fatalf("a converged client must cause no mutation, got:%v", f.log)
	}
}

func TestPocketIDClientsFailsClosedOnUnknownAllowedGroup(t *testing.T) {
	t.Parallel()
	f := newAccessFixture(t,
		[]pocketid.UserGroup{{ID: "group-household", Name: "household", FriendlyName: "Household members who use the shared applications"}},
		platformGroups(),
		clientCRWithAccess("homepage", "homepage", "public", "oauth/homepage",
			// The typo must not remove or widen the existing restriction.
			[]string{"butleradmin", "householdd"}, "https://home.6940469.xyz/callback"))

	err := f.reconcile()
	if err == nil {
		t.Fatal("an unresolvable group name must fail reconciliation")
	}
	if !strings.Contains(err.Error(), `"householdd"`) {
		t.Fatalf("expected the unknown group name in the error, got: %v", err)
	}
	if calls := f.allowedGroupCalls(); len(calls) != 0 {
		t.Fatalf("a partial or empty list must never be sent, got:%#v", calls)
	}
	if reason := conditionReason(statusOf(t, f.store, "PocketIDClient")["homepage/homepage"]); reason != "ReconcileFailed" {
		t.Fatalf("expected ReconcileFailed, got %q", reason)
	}
}

func TestPocketIDClientsLeavesAllowedUserGroupsAloneWhenSpecOmitsThem(t *testing.T) {
	t.Parallel()
	f := newAccessFixture(t,
		[]pocketid.UserGroup{{ID: "group-household", Name: "household", FriendlyName: "Household members who use the shared applications"}},
		platformGroups(),
		// No spec.allowedUserGroups: "leave unrestricted" means "do not manage".
		clientCR("homepage", "homepage", "public", "oauth/homepage", "https://home.6940469.xyz/callback"))

	if err := f.reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if calls := f.allowedGroupCalls(); len(calls) != 0 {
		t.Fatalf("an omitted restriction must not touch the provider, got:%#v", calls)
	}
	if reason := conditionReason(statusOf(t, f.store, "PocketIDClient")["homepage/homepage"]); reason != "Reconciled" {
		t.Fatalf("expected Reconciled, got %q", reason)
	}
}

func TestPocketIDClientsClearsAllowedUserGroupsForAnEmptyList(t *testing.T) {
	t.Parallel()
	f := newAccessFixture(t,
		[]pocketid.UserGroup{{ID: "group-household", Name: "household", FriendlyName: "Household members who use the shared applications"}},
		platformGroups(),
		// An explicit empty list clears the restriction.
		clientCRWithAccess("homepage", "homepage", "public", "oauth/homepage",
			[]string{}, "https://home.6940469.xyz/callback"))

	if err := f.reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	calls := f.allowedGroupCalls()
	if len(calls) != 1 {
		t.Fatalf("expected the restriction to be cleared, got:%v", f.log)
	}
	if want := `{"userGroupIds":[]}`; calls[0].Body != want {
		t.Fatalf("body = %s, want %s", calls[0].Body, want)
	}
}

// A client metadata write does not carry the restriction, so the reconciler
// must not assume the provider kept it: the restriction is re-asserted.
func TestPocketIDClientsReassertsAllowedUserGroupsAfterAClientUpdate(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	current := []pocketid.UserGroup{{ID: "group-household", Name: "household", FriendlyName: "Household members who use the shared applications"}}
	pidURL := newClientServerWithGroups(t, log, platformGroups(), pocketid.OIDCClient{
		ID: "client-1", Name: "homepage", IsPublic: true, PKCEEnabled: true,
		CallbackURLs: []string{"https://old.6940469.xyz/callback"}, AllowedUserGroups: current,
	}).URL
	vf := newVaultFixture()
	withMachineCredential(vf, "machine-key")
	vf.secrets["oauth/homepage"] = map[string]any{"client_id": "client-1"}
	store := newPlatformStore(clientCRWithAccess("homepage", "homepage", "public", "oauth/homepage",
		[]string{"household"}, "https://home.6940469.xyz/callback"))

	if err := NewPocketIDClients(newVaultServer(t, vf.handler, log), store, pidURL).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	clientUpdate := log.indexOf(http.MethodPut, "/api/oidc/clients/client-1")
	restriction := log.indexOf(http.MethodPut, "/api/oidc/clients/client-1/allowed-user-groups")
	if clientUpdate < 0 || restriction < 0 {
		t.Fatalf("expected a client update followed by a restriction write, got:%v", log)
	}
	if clientUpdate > restriction {
		t.Fatalf("the client update must precede the restriction write, got:%v", log)
	}
}

// clientCRWithAccess must round-trip the nil/empty distinction through the
// dynamic fake, otherwise the reconciler could not tell "unmanaged" from
// "restricted to nobody".
func TestClientCRWithAccessPreservesNilVersusEmpty(t *testing.T) {
	t.Parallel()
	store := newPlatformStore(
		clientCR("a", "omitted", "public", "oauth/a", "https://a.6940469.xyz/callback"),
		clientCRWithAccess("b", "empty", "public", "oauth/b", []string{}, "https://b.6940469.xyz/callback"),
		clientCRWithAccess("c", "listed", "public", "oauth/c", []string{"household"}, "https://c.6940469.xyz/callback"),
	)
	items, err := store.ListPocketIDClients(t.Context())
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	got := map[string][]string{}
	for _, item := range items {
		got[item.Name] = item.Spec.AllowedUserGroups
	}
	if got["omitted"] != nil {
		t.Fatalf("omitted must decode to nil, got %#v", got["omitted"])
	}
	if got["empty"] == nil || len(got["empty"]) != 0 {
		t.Fatalf("empty must decode to an empty non-nil slice, got %#v", got["empty"])
	}
	if len(got["listed"]) != 1 || got["listed"][0] != "household" {
		t.Fatalf("listed = %#v", got["listed"])
	}
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
