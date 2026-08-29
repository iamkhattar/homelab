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

type LifecycleStatus struct {
	Initialized bool `json:"initialized"`
	Sealed      bool `json:"sealed"`
}

// Status uses only Vault's unauthenticated lifecycle endpoints.
func (c *Client) Status(ctx context.Context) (LifecycleStatus, error) {
	initialized, err := c.raw.Sys().InitStatusWithContext(ctx)
	if err != nil {
		return LifecycleStatus{}, fmt.Errorf("checking vault init status: %w", err)
	}
	if !initialized {
		return LifecycleStatus{}, nil
	}
	seal, err := c.raw.Sys().SealStatusWithContext(ctx)
	if err != nil {
		return LifecycleStatus{}, fmt.Errorf("checking vault seal status: %w", err)
	}
	return LifecycleStatus{Initialized: true, Sealed: seal.Sealed}, nil
}

// EnsureReady makes sure Vault is initialized, unsealed, and the client is
// authenticated.
//
// Critical invariant: every Vault API call in this function is on an endpoint
// that does NOT require a token (sys/init, sys/seal-status, sys/unseal). The
// caller is expected to construct *Client with NewClient (no token) and only
// have a token after EnsureReady returns successfully. This means butler can
// bootstrap a brand-new Vault from a cold start with nothing but its own pod
// identity — there is no chicken-and-egg "where does the first token come
// from" problem.
//
// On success the client's token is set to the root token (either freshly
// minted during sys/init or loaded from the butler-vault-init Secret).
//
// Init credentials are stored as a K8s Secret in the butler namespace so
// subsequent pod restarts can read them and re-unseal without operator
// intervention.
func (c *Client) EnsureReady(ctx context.Context, k8s kubernetes.Interface, namespace string) error {
	initialized, err := c.EnsureInitializedAndUnsealed(ctx, k8s, namespace)
	if err != nil {
		return err
	}
	if initialized {
		return nil
	}
	return c.LoadBootstrapToken(ctx, k8s, namespace)
}

// EnsureInitializedAndUnsealed performs only Vault's unauthenticated lifecycle
// operations. It returns true when this call initialized Vault (and therefore
// the client already holds the newly issued root token). Existing Vaults do not
// load the root token here.
func (c *Client) EnsureInitializedAndUnsealed(ctx context.Context, k8s kubernetes.Interface, namespace string) (bool, error) {
	// 1. Check init status. sys/init is unauthenticated.
	initStatus, err := c.raw.Sys().InitStatusWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("checking vault init status: %w", err)
	}

	newlyInitialized := !initStatus
	if newlyInitialized {
		slog.Info("vault is not initialized, initializing")
		if err := c.initialize(ctx, k8s, namespace); err != nil {
			return false, err
		}
	} else {
		slog.Info("vault is already initialized")
	}

	// 2. Check seal status and unseal if needed. Both are unauthenticated.
	sealStatus, err := c.raw.Sys().SealStatusWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("checking vault seal status: %w", err)
	}

	if sealStatus.Sealed {
		slog.Info("vault is sealed, unsealing")
		if err := c.unseal(ctx, k8s, namespace); err != nil {
			return false, err
		}
	} else {
		slog.Info("vault is already unsealed")
	}

	return newlyInitialized, nil
}

// LoadBootstrapToken loads the recovery root token for the one-time migration
// path where Butler's Kubernetes auth role does not exist yet.
func (c *Client) LoadBootstrapToken(ctx context.Context, k8s kubernetes.Interface, namespace string) error {
	return c.loadToken(ctx, k8s, namespace)
}

// initialize calls sys/init with 1 key share / 1 threshold (suitable for a
// homelab single-node Vault) and persists the credentials as a K8s Secret.
//
// Idempotency: if a butler-vault-init Secret already exists in the namespace
// (e.g., from a previous partial run where init succeeded but Secret-create
// failed) we overwrite it with the freshly-minted credentials — the old
// keys are useless against the just-reinitialized Vault anyway.
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

	_, err = k8s.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		slog.Warn("butler-vault-init Secret already existed before init; overwriting with new credentials")
		existing, getErr := k8s.CoreV1().Secrets(namespace).Get(ctx, initSecretName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("reading existing vault init secret before update: %w", getErr)
		}
		secret.ResourceVersion = existing.ResourceVersion
		if _, err := k8s.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating existing vault init secret: %w", err)
		}
	} else if err != nil {
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
