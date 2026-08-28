package reconciler

import (
	"context"
	"fmt"

	"github.com/iamkhattar/homelab/butler/internal/vault"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// CABundleConfigMapName is the well-known ConfigMap name where butler
// publishes the homelab PKI CA chain. Consumers (cert-manager, pods that
// need to TLS-verify against in-cluster services, and a future homelabctl trust command
// fallback) all read from this name.
const (
	CABundleConfigMapName = "homelab-ca-bundle"
	CABundleNamespace     = "kube-system"
	CABundleDataKey       = "ca-bundle.pem"
)

// CABundle is the reconciler that fetches the PKI CA chain from Vault and
// publishes it as a public ConfigMap. The CA chain is non-secret PEM and
// is safe to expose cluster-wide.
type CABundle struct {
	vault *vault.Client
	k8s   kubernetes.Interface
}

// NewCABundle creates a CABundle reconciler.
func NewCABundle(vc *vault.Client, k8s kubernetes.Interface) *CABundle {
	return &CABundle{vault: vc, k8s: k8s}
}

// Name implements Reconciler.
func (r *CABundle) Name() string { return "ca-bundle" }

// Reconcile fetches the current Vault PKI CA chain and ensures the
// homelab-ca-bundle ConfigMap matches it. Idempotent.
func (r *CABundle) Reconcile(ctx context.Context) error {
	chain, err := r.vault.CAChain(ctx)
	if err != nil {
		// PKI may not yet be bootstrapped on the very first reconcile pass
		// (vault-bootstrap runs immediately before this one, but if that
		// reconciler partially failed there might be no CA chain yet). We
		// don't want to return a hard error in that case — the next pass
		// will retry.
		return fmt.Errorf("fetching pki ca chain: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CABundleConfigMapName,
			Namespace: CABundleNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "butler",
				"homelab.io/ca-bundle":         "true",
			},
		},
		Data: map[string]string{
			CABundleDataKey: chain,
		},
	}

	existing, err := r.k8s.CoreV1().ConfigMaps(CABundleNamespace).Get(ctx, CABundleConfigMapName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err := r.k8s.CoreV1().ConfigMaps(CABundleNamespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating ca-bundle ConfigMap: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading existing ca-bundle ConfigMap: %w", err)
	}

	if existing.Data != nil && existing.Data[CABundleDataKey] == chain {
		// No drift.
		return nil
	}
	cm.ResourceVersion = existing.ResourceVersion
	if _, err := r.k8s.CoreV1().ConfigMaps(CABundleNamespace).Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating ca-bundle ConfigMap: %w", err)
	}
	return nil
}
