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
