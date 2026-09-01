package reconciler

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/iamkhattar/homelab/butler/internal/platform"
)

func TestSelectManagedCredentialsScopesRecoveryBootstrap(t *testing.T) {
	items := []platform.ManagedCredential{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "security", Name: "pocket-id-runtime"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "homepage", Name: "homepage-runtime"}},
	}

	selected := selectManagedCredentials(items, "security", "pocket-id-runtime")
	if len(selected) != 1 || selected[0].Namespace != "security" || selected[0].Name != "pocket-id-runtime" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestSelectManagedCredentialsLeavesNormalReconciliationUnfiltered(t *testing.T) {
	items := []platform.ManagedCredential{{}, {}}
	if selected := selectManagedCredentials(items, "", ""); len(selected) != len(items) {
		t.Fatalf("selected %d credentials, want %d", len(selected), len(items))
	}
}
