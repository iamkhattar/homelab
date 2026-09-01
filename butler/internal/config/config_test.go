package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestK8sIssuanceTokenReviewerReference(t *testing.T) {
	input := []byte(`
k8sIssuance:
  enabled: true
  tokenReviewerRef: vault-k8s-token-reviewer-token
`)

	var cfg Config
	if err := yaml.Unmarshal(input, &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got, want := cfg.K8sIssuance.TokenReviewerRef, "vault-k8s-token-reviewer-token"; got != want {
		t.Fatalf("TokenReviewerRef = %q, want %q", got, want)
	}
}

func TestLoadReconcileInterval(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "butler.yaml")
	config := `
vault: {address: "http://vault:8200"}
server: {port: "8080"}
reconcile: {interval: "1m"}
oidc: {adminURL: "http://pocket-id:1411"}
certificates:
  acmeDNSURL: "https://auth.acme-dns.io"
  domain: "example.test"
  credentialPath: "infrastructure/acme-dns"
  namespace: "networking"
  certificateName: "wildcard"
  tlsSecretName: "wildcard-tls"
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUTLER_CONFIG_PATH", configPath)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReconcileInterval != time.Minute {
		t.Fatalf("ReconcileInterval = %s, want 1m", cfg.ReconcileInterval)
	}
}

func TestLoadRejectsNonPositiveReconcileInterval(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "butler.yaml")
	config := `
vault: {address: "http://vault:8200"}
server: {port: "8080"}
reconcile: {interval: "0s"}
oidc: {adminURL: "http://pocket-id:1411"}
certificates:
  acmeDNSURL: "https://auth.acme-dns.io"
  domain: "example.test"
  credentialPath: "infrastructure/acme-dns"
  namespace: "networking"
  certificateName: "wildcard"
  tlsSecretName: "wildcard-tls"
`
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUTLER_CONFIG_PATH", configPath)
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must be positive") {
		t.Fatalf("Load() error = %v, want positive-interval validation", err)
	}
}
