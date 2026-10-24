package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// vaultLifecycleStub answers the unauthenticated lifecycle endpoints.
type vaultLifecycleStub struct {
	initialized bool
	sealed      bool
	unseals     []string
}

func (s *vaultLifecycleStub) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sys/init":
		_ = json.NewEncoder(w).Encode(map[string]any{"initialized": s.initialized})
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sys/seal-status":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sealed": s.sealed, "t": 1, "n": 1, "p": 1, "version": "1.15.0", "cluster_name": "vault-0",
		})
	case r.Method == http.MethodPut && r.URL.Path == "/v1/sys/unseal":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.unseals = append(s.unseals, body["key"])
		s.sealed = false
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sealed": false, "t": 1, "n": 1, "p": 1, "version": "1.15.0", "cluster_name": "vault-0",
		})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func recoverySecret(t *testing.T, key string) *fake.Clientset {
	t.Helper()
	return fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "butler-vault-init", Namespace: "security"},
		Data:       map[string][]byte{unsealKeyKey: []byte(key)},
	})
}

func TestEnsureUnsealedUsesTheStoredRecoveryKey(t *testing.T) {
	stub := &vaultLifecycleStub{initialized: true, sealed: true}
	server := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	k8s := recoverySecret(t, "stored-unseal-key")

	if err := client.EnsureUnsealed(context.Background(), k8s, "security"); err != nil {
		t.Fatalf("EnsureUnsealed() error = %v", err)
	}
	if len(stub.unseals) != 1 || stub.unseals[0] != "stored-unseal-key" {
		t.Fatalf("expected the stored recovery key to unseal Vault, got %#v", stub.unseals)
	}
}

func TestEnsureUnsealedIsANoOpWhenVaultIsAlreadyUnsealed(t *testing.T) {
	stub := &vaultLifecycleStub{initialized: true, sealed: false}
	server := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureUnsealed(context.Background(), recoverySecret(t, "key"), "security"); err != nil {
		t.Fatalf("EnsureUnsealed() error = %v", err)
	}
	if len(stub.unseals) != 0 {
		t.Fatalf("an unsealed Vault must not be unsealed again, got %#v", stub.unseals)
	}
}

func TestEnsureUnsealedRefusesAnUninitializedVault(t *testing.T) {
	stub := &vaultLifecycleStub{initialized: false, sealed: false}
	server := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = client.EnsureUnsealed(context.Background(), recoverySecret(t, "key"), "security")
	if err == nil {
		t.Fatal("expected an uninitialized Vault to be refused")
	}
	if len(stub.unseals) != 0 {
		t.Fatalf("an uninitialized Vault must not be unsealed, got %#v", stub.unseals)
	}
}

func TestEnsureUnsealedFailsWithoutTheRecoverySecret(t *testing.T) {
	stub := &vaultLifecycleStub{initialized: true, sealed: true}
	server := httptest.NewServer(http.HandlerFunc(stub.handler))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.EnsureUnsealed(context.Background(), fake.NewSimpleClientset(), "security"); err == nil {
		t.Fatal("expected a missing recovery Secret to fail the unseal")
	}
	if len(stub.unseals) != 0 {
		t.Fatalf("Vault must not be unsealed without the recovery key, got %#v", stub.unseals)
	}
}
