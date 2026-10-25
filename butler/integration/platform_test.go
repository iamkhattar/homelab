//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// These tests deliberately target a real private platform. They are excluded
// from normal unit tests and require BUTLER_INTEGRATION=1 so a developer cannot
// accidentally point them at production-like services.
func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("BUTLER_INTEGRATION") != "1" {
		t.Skip("set BUTLER_INTEGRATION=1 to run real platform integration tests")
	}
}

func TestVaultRoundTrip(t *testing.T) {
	requireIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := vault.NewClient(requiredEnv(t, "VAULT_ADDR"))
	if err != nil {
		t.Fatal(err)
	}
	client.SetToken(requiredEnv(t, "VAULT_TOKEN"))
	name := fmt.Sprintf("integration/homelabctl-%d", time.Now().UnixNano())
	if err := client.WriteSecret(ctx, name, map[string]interface{}{"probe": "ok"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.Raw().Logical().Delete("secret/metadata/" + name)
	})
	data, err := client.ReadSecret(ctx, name)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if data["probe"] != "ok" {
		t.Fatalf("round trip = %#v", data)
	}
}

func TestPocketIDAdminAPI(t *testing.T) {
	requireIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := pocketid.NewClient(requiredEnv(t, "POCKET_ID_URL"), requiredEnv(t, "POCKET_ID_API_KEY"))
	if _, err := client.ListUserGroups(ctx); err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if _, err := client.ListClients(ctx); err != nil {
		t.Fatalf("list clients: %v", err)
	}
}

func TestKubernetesAPI(t *testing.T) {
	requireIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := clientcmd.BuildConfigFromFlags("", requiredEnv(t, "KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Discovery().ServerVersion(); err != nil {
		t.Fatalf("server version: %v", err)
	}
	if _, err := client.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{}); err != nil {
		t.Fatalf("read kube-system: %v", err)
	}
}

// TestVaultKubernetesCredential proves the human-facing path: a Vault token
// carrying the group's policy mints a short-lived Kubernetes credential. Butler
// is not involved; its brokered endpoint no longer exists.
func TestVaultKubernetesCredential(t *testing.T) {
	requireIntegration(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := vault.NewClient(requiredEnv(t, "VAULT_ADDR"))
	if err != nil {
		t.Fatal(err)
	}
	client.SetToken(requiredEnv(t, "VAULT_TOKEN"))
	secret, err := client.Raw().Logical().ReadWithContext(ctx, "kubernetes/creds/k8sviewer")
	if err != nil {
		t.Fatalf("reading Vault Kubernetes credentials: %v", err)
	}
	if secret == nil || secret.Data == nil {
		t.Fatal("Vault returned no Kubernetes credential")
	}
	token, _ := secret.Data["service_account_token"].(string)
	if token == "" {
		t.Fatal("Vault returned an empty Kubernetes token")
	}
	config, err := clientcmd.BuildConfigFromFlags("", requiredEnv(t, "KUBECONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	config.BearerToken = token
	config.BearerTokenFile = ""
	config.Username, config.Password = "", ""
	config.ExecProvider, config.AuthProvider = nil, nil
	config.TLSClientConfig.CertData, config.TLSClientConfig.KeyData = nil, nil
	config.TLSClientConfig.CertFile, config.TLSClientConfig.KeyFile = "", ""
	kube, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	review, err := kube.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authorizationv1.ResourceAttributes{Verb: "get", Resource: "pods"}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("issued token could not call Kubernetes: %v", err)
	}
	if !review.Status.Allowed {
		t.Fatalf("issued viewer credential was authenticated but lacked expected pod read access: %s", review.Status.Reason)
	}
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
