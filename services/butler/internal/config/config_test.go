package config

import (
	"testing"

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
