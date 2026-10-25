package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadKubernetesCredentialUsesTheVaultTokenAndRole(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.Header.Get("X-Vault-Token") != "user-token" {
			t.Errorf("X-Vault-Token = %q", r.Header.Get("X-Vault-Token"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["kubernetes_namespace"] != "security" || body["ttl"] != "1h0m0s" {
			t.Errorf("request body = %#v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{
			"service_account_name":      "vault-managed-viewer",
			"service_account_namespace": "kube-system",
			"service_account_token":     "minted-token",
			"lease_id":                  "kubernetes/creds/k8sviewer/lease",
			"lease_duration":            3600,
		}})
	}))
	defer server.Close()

	credential, err := ReadKubernetesCredential(t.Context(), server.Client(), server.URL, "user-token", "k8sviewer", "security", time.Hour.String())
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/kubernetes/creds/k8sviewer" {
		t.Fatalf("path = %q", path)
	}
	if credential.Token != "minted-token" || credential.ServiceAccountName != "vault-managed-viewer" {
		t.Fatalf("credential = %#v", credential)
	}
	if credential.LeaseDuration != time.Hour {
		t.Fatalf("lease duration = %s", credential.LeaseDuration)
	}
}

func TestReadKubernetesCredentialSurfacesVaultPermissionDenialVerbatim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["1 error occurred:\n\t* permission denied on kubernetes/creds/k8sadmin"]}`))
	}))
	defer server.Close()

	_, err := ReadKubernetesCredential(t.Context(), server.Client(), server.URL, "viewer-token", "k8sadmin", "security", time.Hour.String())
	if err == nil {
		t.Fatal("a denied request must fail")
	}
	// The denial is the authorization failure, so it must not be replaced by a
	// generic message: the operator needs Vault's own words.
	if !strings.Contains(err.Error(), "permission denied on kubernetes/creds/k8sadmin") {
		t.Fatalf("error = %v, want Vault's permission error verbatim", err)
	}
}

func TestReadKubernetesCredentialRejectsAnEmptyToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]interface{}{}})
	}))
	defer server.Close()

	if _, err := ReadKubernetesCredential(t.Context(), server.Client(), server.URL, "token", "k8sviewer", "security", "1h"); err == nil {
		t.Fatal("an empty Vault token response must fail instead of emitting a blank credential")
	}
}
