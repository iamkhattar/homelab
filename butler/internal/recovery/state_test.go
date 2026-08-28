package recovery

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAdvanceRequiresExplicitConfirmation(t *testing.T) {
	service := &Service{}
	if err := service.Advance(context.Background(), false); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfirmIdentityRequiresBothRealLoginPolicySets(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: "security"},
		Data:       map[string]string{"phase": "awaiting-identity-verification"},
	})
	service := &Service{k8s: client, namespace: "security"}
	if err := service.ConfirmIdentity(t.Context(), IdentityEvidence{PocketIDSubject: "owner", ButlerRole: "admin", VaultPolicies: []string{"vault-admin"}}); err == nil {
		t.Fatal("accepted Vault login without Kubernetes administrator policy")
	}
	evidence := IdentityEvidence{PocketIDSubject: "owner", ButlerRole: "admin", VaultPolicies: []string{"default", "vault-admin", "k8s-admin"}}
	if err := service.ConfirmIdentity(t.Context(), evidence); err != nil {
		t.Fatal(err)
	}
	state, err := client.CoreV1().ConfigMaps("security").Get(t.Context(), stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Data["phase"] != "operational" || state.Data["butlerLoginVerified"] != "true" || state.Data["vaultLoginVerified"] != "true" {
		t.Fatalf("state = %#v", state.Data)
	}
}

func TestPocketIDImportRejectsEmptyCredential(t *testing.T) {
	service := &Service{}
	if err := service.ImportPocketIDAPIKey(context.Background(), " "); err == nil {
		t.Fatal("expected empty key rejection")
	}
}
