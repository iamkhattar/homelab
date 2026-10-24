package recovery

import (
	"context"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/vault"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestAdvanceUnsealsVaultAfterRestart proves the supported operator path
// recovers a sealed Vault without replaying the bootstrap ceremony.
//
// Before this, the persisted "operational" phase made Advance a no-op, so a
// restart left Vault sealed with no supported way to unseal it.
func TestAdvanceUnsealsVaultAfterRestart(t *testing.T) {
	k8s := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: "security"},
		Data:       map[string]string{"phase": "operational"},
	})
	store := &recoveryVault{
		status:  vault.LifecycleStatus{Initialized: true, Sealed: true},
		secrets: map[string]map[string]interface{}{},
	}
	service := NewService(store, k8s, "security", bootstrapFunc(func(context.Context) error {
		t.Fatal("the bootstrap ceremony must not be replayed for an unseal")
		return nil
	}), nil, nil)

	if err := service.Advance(context.Background(), true); err != nil {
		t.Fatalf("Advance() error = %v", err)
	}
	if store.unseals != 1 {
		t.Fatalf("expected exactly one unseal attempt, got %d", store.unseals)
	}
	if store.status.Sealed {
		t.Fatal("vault must be unsealed after Advance")
	}
}

// TestAdvanceReportsUnsealFailure proves a failed unseal is surfaced instead
// of being reported as a successful advance.
func TestAdvanceReportsUnsealFailure(t *testing.T) {
	k8s := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: "security"},
		Data:       map[string]string{"phase": "operational"},
	})
	store := &recoveryVault{
		status:    vault.LifecycleStatus{Initialized: true, Sealed: true},
		secrets:   map[string]map[string]interface{}{},
		unsealErr: errSealedUnavailable,
	}
	service := NewService(store, k8s, "security", bootstrapFunc(func(context.Context) error {
		t.Fatal("the bootstrap ceremony must not be replayed for an unseal")
		return nil
	}), nil, nil)

	err := service.Advance(context.Background(), true)
	if err == nil || !strings.Contains(err.Error(), "unsealing vault after a restart") {
		t.Fatalf("Advance() error = %v, want an unseal failure", err)
	}
}

// TestUnconfirmedAdvanceDoesNotUnseal proves an unseal still needs an
// explicit operator confirmation.
func TestUnconfirmedAdvanceDoesNotUnseal(t *testing.T) {
	k8s := fake.NewSimpleClientset()
	store := &recoveryVault{
		status:  vault.LifecycleStatus{Initialized: true, Sealed: true},
		secrets: map[string]map[string]interface{}{},
	}
	service := NewService(store, k8s, "security", bootstrapFunc(func(context.Context) error {
		t.Fatal("an unconfirmed advance must not touch Vault")
		return nil
	}), nil, nil)

	if err := service.Advance(context.Background(), false); err == nil {
		t.Fatal("expected an error without confirmation")
	}
	if store.unseals != 0 {
		t.Fatalf("expected no unseal without confirmation, got %d", store.unseals)
	}
}

var errSealedUnavailable = &sealedUnavailableError{}

type sealedUnavailableError struct{}

func (*sealedUnavailableError) Error() string { return "vault is unreachable" }
