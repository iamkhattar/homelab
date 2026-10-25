package vault

import (
	"context"
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

func TestAdminPolicyRemainsTheOnlySecretReader(t *testing.T) {
	// vault-admin is a wildcard policy; the two non-admin tiers must not be able
	// to read values. Reaching them requires an explicit secret/data stanza.
	if !strings.Contains(vaultAdminPolicy, `path "*"`) {
		t.Fatal("vault-admin is expected to remain a wildcard policy")
	}
	if strings.Contains(vaultMetadataPolicy, `path "secret/data`) {
		t.Fatal("non-admin tier must not read secret values")
	}
}
