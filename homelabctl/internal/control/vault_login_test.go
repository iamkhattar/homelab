package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifyVaultOIDCLoginUsesAndRevokesTemporaryToken(t *testing.T) {
	var redirectURI string
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/jwt/oidc/auth_url":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			redirectURI = body["redirect_uri"]
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]string{"auth_url": "https://identity.example.test/authorize"}})
		case "/v1/auth/jwt/oidc/callback":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"auth": map[string]interface{}{
				"client_token": "temporary-vault-token",
				"policies":     []string{"default", "vault-admin", "k8s-admin"},
			}})
		case "/v1/auth/token/lookup-self":
			if r.Header.Get("X-Vault-Token") != "temporary-vault-token" {
				t.Fatal("lookup omitted temporary Vault token")
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"policies": []string{"default", "vault-admin", "k8s-admin"}}})
		case "/v1/auth/token/revoke-self":
			if r.Header.Get("X-Vault-Token") != "temporary-vault-token" {
				t.Fatal("revoke omitted temporary Vault token")
			}
			revoked.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	evidence, err := VerifyVaultOIDCLogin(t.Context(), VaultOIDCOptions{
		Address:       server.URL,
		Role:          "homelab-admin",
		ListenAddress: "127.0.0.1:0",
		Timeout:       10 * time.Second,
		OpenURL: func(string) error {
			response, err := http.Get(redirectURI + "?state=test&code=code") // #nosec G107 -- test-only loopback URL.
			if err == nil {
				_ = response.Body.Close()
			}
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Policies) < 3 || !revoked.Load() {
		t.Fatalf("evidence = %#v, revoked = %t", evidence, revoked.Load())
	}
}

func TestVerifyAndRevokeVaultTokenRejectsUnderprivilegedOIDCLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/token/lookup-self" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{"policies": []string{"vault-read"}}})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	login := vaultSecret{Auth: &struct {
		ClientToken string            `json:"client_token"`
		Policies    []string          `json:"policies"`
		Metadata    map[string]string `json:"metadata"`
	}{ClientToken: "viewer", Policies: []string{"vault-read"}}}
	if _, err := verifyAndRevokeVaultToken(t.Context(), VaultOIDCOptions{Address: server.URL, HTTPClient: server.Client()}, login); err == nil {
		t.Fatal("underprivileged Vault login accepted")
	}
}
