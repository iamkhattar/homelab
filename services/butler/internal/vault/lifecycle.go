package vault

import (
	"context"
	"fmt"
	"log/slog"

	vaultapi "github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	initSecretName = "butler-vault-init"
	rootTokenKey   = "root-token"
	unsealKeyKey   = "unseal-key"
)

// EnsureReady makes sure Vault is initialized, unsealed, and the client is
// authenticated. It stores init credentials in a K8s Secret so subsequent
// restarts can unseal without manual intervention.
func (c *Client) EnsureReady(ctx context.Context, k8s kubernetes.Interface, namespace string) error {
	// 1. Check init status (no auth required).
	initStatus, err := c.raw.Sys().InitStatusWithContext(ctx)
	if err != nil {
		return fmt.Errorf("checking vault init status: %w", err)
	}

	if !initStatus {
		slog.Info("vault is not initialized, initializing")
		if err := c.initialize(ctx, k8s, namespace); err != nil {
			return err
		}
	} else {
		slog.Info("vault is already initialized")
		if err := c.loadToken(ctx, k8s, namespace); err != nil {
			return err
		}
	}

	// 2. Check seal status and unseal if needed.
	sealStatus, err := c.raw.Sys().SealStatusWithContext(ctx)
	if err != nil {
		return fmt.Errorf("checking vault seal status: %w", err)
	}

	if sealStatus.Sealed {
		slog.Info("vault is sealed, unsealing")
		if err := c.unseal(ctx, k8s, namespace); err != nil {
			return err
		}
	} else {
		slog.Info("vault is already unsealed")
	}

	return nil
}

// initialize calls sys/init with 1 key share / 1 threshold (suitable for a
// homelab single-node Vault) and persists the credentials as a K8s Secret.
func (c *Client) initialize(ctx context.Context, k8s kubernetes.Interface, namespace string) error {
	resp, err := c.raw.Sys().InitWithContext(ctx, &vaultapi.InitRequest{
		SecretShares:    1,
		SecretThreshold: 1,
	})
	if err != nil {
		return fmt.Errorf("initializing vault: %w", err)
	}

	if len(resp.KeysB64) == 0 {
		return fmt.Errorf("vault init returned no unseal keys")
	}

	unsealKey := resp.KeysB64[0]
	rootToken := resp.RootToken

	// Persist to K8s Secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      initSecretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			rootTokenKey: rootToken,
			unsealKeyKey: unsealKey,
		},
	}

	if _, err := k8s.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("storing vault init secret: %w", err)
	}

	slog.Info("vault initialized, credentials stored in k8s secret", "secret", initSecretName)
	c.SetToken(rootToken)
	return nil
}

// loadToken reads the root token from the K8s Secret created during init.
func (c *Client) loadToken(ctx context.Context, k8s kubernetes.Interface, namespace string) error {
	secret, err := k8s.CoreV1().Secrets(namespace).Get(ctx, initSecretName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("vault is initialized but %s secret not found — was it initialized by butler?", initSecretName)
		}
		return fmt.Errorf("reading %s secret: %w", initSecretName, err)
	}

	token, ok := secret.Data[rootTokenKey]
	if !ok {
		return fmt.Errorf("%s secret missing %s key", initSecretName, rootTokenKey)
	}

	c.SetToken(string(token))
	return nil
}

// unseal reads the unseal key from the K8s Secret and unseals Vault.
func (c *Client) unseal(ctx context.Context, k8s kubernetes.Interface, namespace string) error {
	secret, err := k8s.CoreV1().Secrets(namespace).Get(ctx, initSecretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading %s secret for unseal: %w", initSecretName, err)
	}

	key, ok := secret.Data[unsealKeyKey]
	if !ok {
		return fmt.Errorf("%s secret missing %s key", initSecretName, unsealKeyKey)
	}

	resp, err := c.raw.Sys().UnsealWithContext(ctx, string(key))
	if err != nil {
		return fmt.Errorf("unsealing vault: %w", err)
	}

	if resp.Sealed {
		return fmt.Errorf("vault still sealed after unseal attempt")
	}

	slog.Info("vault unsealed successfully")
	return nil
}

