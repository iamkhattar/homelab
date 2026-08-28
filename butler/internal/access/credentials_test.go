package access

import (
	"context"
	"testing"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

type fakeIssuer struct{ called bool }

func (f *fakeIssuer) IssueKubernetesCredential(_ context.Context, role, namespace string, ttl time.Duration) (vault.KubernetesCredential, error) {
	f.called = true
	return vault.KubernetesCredential{Role: role, Namespace: namespace, TTLSeconds: int64(ttl.Seconds())}, nil
}

func TestCredentialServiceEnforcesRoleAndTTL(t *testing.T) {
	issuer := &fakeIssuer{}
	service, err := NewCredentialService(issuer, config.K8sIssuanceConfig{HostNamespace: "kube-system", Roles: []config.K8sIssuanceRoleSpec{{Name: "homelab-viewer", MaxTTL: "4h"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Issue(context.Background(), Principal{Role: Viewer}, "homelab-viewer", time.Hour); err == nil {
		t.Fatal("viewer issued a credential")
	}
	if _, err := service.Issue(context.Background(), Principal{Role: Admin}, "unknown", time.Hour); err == nil {
		t.Fatal("unknown role accepted")
	}
	if _, err := service.Issue(context.Background(), Principal{Role: Admin}, "homelab-viewer", 5*time.Hour); err == nil {
		t.Fatal("excessive TTL accepted")
	}
	credential, err := service.Issue(context.Background(), Principal{Role: Admin}, "homelab-viewer", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !issuer.called || credential.Namespace != "kube-system" {
		t.Fatalf("credential = %#v", credential)
	}
}
