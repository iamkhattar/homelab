package reconciler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func vaultOIDCRoleCR(namespace, name, group string, policies []string, ttl, maxTTL string) *unstructured.Unstructured {
	spec := map[string]interface{}{"group": group}
	rawPolicies := make([]interface{}, 0, len(policies))
	for _, policy := range policies {
		rawPolicies = append(rawPolicies, policy)
	}
	spec["policies"] = rawPolicies
	if ttl != "" {
		spec["ttl"] = ttl
	}
	if maxTTL != "" {
		spec["maxTTL"] = maxTTL
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": platform.Group + "/" + platform.Version,
		"kind":       "VaultOIDCRole",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace, "generation": int64(1)},
		"spec":       spec,
	}}
}

// oauthVaultSecret is what the PocketIDClient reconciler stores for Vault.
func oauthVaultSecret(clientID string) []byte {
	return kvSecret(map[string]any{"client_id": clientID, "client_secret": "secret"})
}

func TestVaultOIDCRolesWritesDeclaredRole(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		switch {
		case method == http.MethodGet && path == "oauth/vault":
			return http.StatusOK, oauthVaultSecret("vault-client")
		case method == http.MethodGet && path == "auth/jwt/role/k8sviewer":
			return http.StatusNotFound, nil
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultOIDCRoleCR("security", "k8sviewer", "k8sviewer", []string{"k8s-viewer"}, "", ""))
	r := NewVaultOIDCRoles(vc, store, "example.test", "security")
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}

	mutations := log.mutations()
	if len(mutations) != 1 {
		t.Fatalf("mutations = %d, want exactly one role write:%v", len(mutations), log)
	}
	if mutations[0].Path != "auth/jwt/role/k8sviewer" {
		t.Fatalf("path = %q, want auth/jwt/role/k8sviewer", mutations[0].Path)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(mutations[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]string{
		"role_type":    "oidc",
		"user_claim":   "sub",
		"groups_claim": "groups",
		"ttl":          "1h",
		"max_ttl":      "8h",
	} {
		if got, _ := body[field].(string); got != want {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if got := vaultStringSlice(body["bound_audiences"]); len(got) != 1 || got[0] != "vault-client" {
		t.Errorf("bound_audiences = %v, want the Vault OIDC client ID", got)
	}
	if got := vaultBoundGroups(body["bound_claims"]); len(got) != 1 || got[0] != "k8sviewer" {
		t.Errorf("bound_claims.groups = %v, want [k8sviewer]", got)
	}
	if got := vaultStringSlice(body["policies"]); len(got) != 1 || got[0] != "k8s-viewer" {
		t.Errorf("policies = %v, want [k8s-viewer]", got)
	}
	scopes := vaultStringSlice(body["oidc_scopes"])
	if len(scopes) != 4 || scopes[3] != "groups" {
		t.Errorf("oidc_scopes = %v", scopes)
	}
	redirects := vaultStringSlice(body["allowed_redirect_uris"])
	if len(redirects) != 2 ||
		redirects[0] != "http://localhost:8250/oidc/callback" ||
		redirects[1] != "https://vault.example.test/ui/vault/auth/jwt/oidc/callback" {
		t.Errorf("allowed_redirect_uris = %v", redirects)
	}
}

func TestVaultOIDCRolesRepairsDrift(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		switch {
		case method == http.MethodGet && path == "oauth/vault":
			return http.StatusOK, oauthVaultSecret("vault-client")
		case method == http.MethodGet && path == "auth/jwt/role/vaultadmin":
			// A role that exists with the wrong policy must be rewritten.
			return http.StatusOK, []byte(`{"data":{"role_type":"oidc","user_claim":"sub","groups_claim":"groups","oidc_scopes":["openid","profile","email","groups"],"bound_audiences":["vault-client"],"bound_claims":{"groups":["vaultadmin"]},"token_policies":["default"],"ttl":3600,"max_ttl":28800,"allowed_redirect_uris":["http://localhost:8250/oidc/callback","https://vault.example.test/ui/vault/auth/jwt/oidc/callback"]}}`)
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultOIDCRoleCR("security", "vaultadmin", "vaultadmin", []string{"vault-admin"}, "2h", "8h"))
	r := NewVaultOIDCRoles(vc, store, "example.test", "security")
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	mutations := log.mutations()
	if len(mutations) != 1 {
		t.Fatalf("drift was not repaired, mutations = %d:%v", len(mutations), log)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(mutations[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	if got, _ := body["ttl"].(string); got != "2h" {
		t.Fatalf("ttl = %q, want 2h", got)
	}
}

func TestVaultOIDCRolesLeavesMatchingRoleAlone(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		switch {
		case method == http.MethodGet && path == "oauth/vault":
			return http.StatusOK, oauthVaultSecret("vault-client")
		case method == http.MethodGet && path == "auth/jwt/role/vaultread":
			return http.StatusOK, []byte(`{"data":{"role_type":"oidc","user_claim":"sub","groups_claim":"groups","oidc_scopes":["openid","profile","email","groups"],"bound_audiences":["vault-client"],"bound_claims":{"groups":["vaultread"]},"token_policies":["vault-read"],"ttl":3600,"max_ttl":28800,"allowed_redirect_uris":["http://localhost:8250/oidc/callback","https://vault.example.test/ui/vault/auth/jwt/oidc/callback"]}}`)
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultOIDCRoleCR("security", "vaultread", "vaultread", []string{"vault-read"}, "", ""))
	r := NewVaultOIDCRoles(vc, store, "example.test", "security")
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("a converged role must not be rewritten:%v", log)
	}
}

func TestVaultOIDCRolesDefersUntilTheOIDCClientExists(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "oauth/vault" {
			return http.StatusNotFound, nil
		}
		t.Fatalf("unexpected %s %s before the Vault OIDC client existed", method, path)
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultOIDCRoleCR("security", "k8sviewer", "k8sviewer", []string{"k8s-viewer"}, "", ""))
	r := NewVaultOIDCRoles(vc, store, "example.test", "security")
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("roles were written without an audience:%v", log)
	}
}

func TestVaultOIDCRolesIgnoresForeignNamespace(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "oauth/vault" {
			return http.StatusOK, oauthVaultSecret("vault-client")
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultOIDCRoleCR("default", "escalate", "vaultadmin", []string{"vault-admin"}, "", ""))
	r := NewVaultOIDCRoles(vc, store, "example.test", "security")
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("a role declared outside the Vault namespace was written:%v", log)
	}
}

func TestVaultOIDCRolesRejectsDuplicateGroups(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		if method == http.MethodGet && path == "oauth/vault" {
			return http.StatusOK, oauthVaultSecret("vault-client")
		}
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(
		vaultOIDCRoleCR("security", "first", "k8sviewer", []string{"k8s-viewer"}, "", ""),
		vaultOIDCRoleCR("security", "second", "k8sviewer", []string{"k8s-admin"}, "", ""),
	)
	r := NewVaultOIDCRoles(vc, store, "example.test", "security")
	if err := r.Reconcile(t.Context()); err == nil {
		t.Fatal("two declarations for one group must fail instead of last-writer-wins")
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("ambiguous authority was written:%v", log)
	}
}
