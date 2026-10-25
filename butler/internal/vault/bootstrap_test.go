package vault

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestOIDCBootstrapReadyDefersUntilClientCredentialExists(t *testing.T) {
	ready, err := oidcBootstrapReady(BootstrapInput{OIDCIssuer: "https://id.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("OIDC bootstrap must wait for the Pocket ID client credential")
	}
}

func TestOIDCBootstrapReadyRequiresCompleteConfiguration(t *testing.T) {
	tests := []struct {
		name string
		in   BootstrapInput
		want string
	}{
		{name: "client without issuer", in: BootstrapInput{OIDCClientID: "vault", OIDCClientSecret: "secret"}, want: "require an issuer"},
		{name: "missing secret", in: BootstrapInput{OIDCIssuer: "https://id.example.test", OIDCClientID: "vault"}, want: "both client ID and client secret"},
		{name: "missing client id", in: BootstrapInput{OIDCIssuer: "https://id.example.test", OIDCClientSecret: "secret"}, want: "both client ID and client secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, err := oidcBootstrapReady(tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ready = %v, error = %v; want error containing %q", ready, err, tt.want)
			}
			if ready {
				t.Fatal("incomplete OIDC input reported ready")
			}
		})
	}
}

func TestOIDCBootstrapReadyAcceptsCompleteCredential(t *testing.T) {
	ready, err := oidcBootstrapReady(BootstrapInput{
		OIDCIssuer:       "https://id.example.test",
		OIDCClientID:     "vault",
		OIDCClientSecret: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("complete OIDC input did not report ready")
	}
}

func TestEnsureJWTAuthDoesNotContactVaultBeforePocketIDClientExists(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ensureJWTAuth(context.Background(), BootstrapInput{
		OIDCIssuer: "https://id.example.test",
	}); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("Vault received %d requests before the Pocket ID client existed", got)
	}
}

func TestRecoveryPolicyCanManageOnlyBootstrapIdentityCredentials(t *testing.T) {
	paths := []string{
		"security/pocket-id",
		"oauth/homelabctl",
		"oauth/vault",
	}
	for _, path := range paths {
		want := `path "secret/data/` + path + `" { capabilities = ["create", "read", "update"] }`
		if !strings.Contains(butlerRecoveryPolicy, want) {
			t.Errorf("recovery policy must manage exact bootstrap credential path %q", path)
		}
	}
	for _, forbidden := range []string{`secret/data/oauth/*`, `secret/data/*`} {
		if strings.Contains(butlerRecoveryPolicy, forbidden) {
			t.Errorf("recovery policy must not contain broad data path %q", forbidden)
		}
	}
}

func TestRecoveryPolicyCanCompleteOIDCRolesButNotPolicies(t *testing.T) {
	// The recovery ceremony writes the chart-declared auth/jwt roles after the
	// Pocket ID client exists. It must not be able to author policy bodies or
	// reconfigure the auth method.
	if !strings.Contains(butlerRecoveryPolicy, `path "auth/jwt/role/*"`) {
		t.Fatal("recovery must be able to converge declared auth/jwt roles")
	}
	for _, forbidden := range []string{`path "sys/policies`, `path "sys/auth`, `path "auth/jwt/config`} {
		if strings.Contains(butlerRecoveryPolicy, forbidden) {
			t.Errorf("recovery policy must not grant %q", forbidden)
		}
	}
}

func TestMetadataPolicyCannotReadSecretValues(t *testing.T) {
	// Match a real stanza, not the explanatory comment.
	if strings.Contains(vaultMetadataPolicy, `path "secret/data`) {
		t.Fatal("vault-metadata must not grant any access to secret values")
	}
	for _, want := range []string{`path "secret/metadata"`, `path "secret/metadata/*"`, `"list"`} {
		if !strings.Contains(vaultMetadataPolicy, want) {
			t.Errorf("vault-metadata policy must contain %s", want)
		}
	}
}

func TestSecretValueReadsAreScopedToVaultReadAndAdmin(t *testing.T) {
	// vault-read is the explicit secret-value grant; vault-admin is the
	// wildcard administrator. vault-metadata must never carry a data stanza.
	if !strings.Contains(vaultAdminPolicy, `path "*"`) {
		t.Fatal("vault-admin is expected to remain a wildcard policy")
	}
	if !strings.Contains(vaultReadPolicy, `path "secret/data/*"`) {
		t.Fatal("vault-read must grant read on secret values")
	}
	if strings.Contains(vaultMetadataPolicy, `path "secret/data`) {
		t.Fatal("vault-metadata must not read secret values")
	}
}

func TestEnsureJWTPoliciesWritesTheFullPlatformPolicySet(t *testing.T) {
	written := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Policy string `json:"policy"`
		}
		_ = json.Unmarshal(body, &payload)
		written[strings.TrimPrefix(r.URL.Path, "/v1/sys/policies/acl/")] = payload.Policy
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ensureJWTPolicies(context.Background(), "https://id.example.test"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vault-admin", "vault-read", "vault-metadata"} {
		if written[name] == "" {
			t.Errorf("policy %s was not written", name)
		}
	}
}
