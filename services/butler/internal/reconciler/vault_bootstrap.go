package reconciler

import (
	"context"

	"github.com/iamkhattar/homelab/services/butler/internal/vault"
	"k8s.io/client-go/kubernetes"
)

// VaultBootstrap ensures Vault is initialized, unsealed, and configured.
type VaultBootstrap struct {
	vault     *vault.Client
	k8s       kubernetes.Interface
	namespace string
}

// NewVaultBootstrap creates a new VaultBootstrap reconciler.
func NewVaultBootstrap(vc *vault.Client, k8s kubernetes.Interface, namespace string) *VaultBootstrap {
	return &VaultBootstrap{vault: vc, k8s: k8s, namespace: namespace}
}

func (r *VaultBootstrap) Name() string { return "vault-bootstrap" }

func (r *VaultBootstrap) Reconcile(ctx context.Context) error {
	if err := r.vault.EnsureReady(ctx, r.k8s, r.namespace); err != nil {
		return err
	}
	return r.vault.Bootstrap(ctx)
}
