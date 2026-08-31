package reconciler

import (
	"context"
	"log/slog"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/vault"
	"k8s.io/client-go/kubernetes"
)

// vaultOAuthClientPath is where the OAuthClients reconciler stores the
// vault client_id/client_secret. ensureJWTAuth uses these to configure
// OIDC discovery against Pocket-ID.
const vaultOAuthClientPath = "oauth/vault"

// VaultBootstrap ensures Vault is initialized, unsealed, and configured.
type VaultBootstrap struct {
	vault     *vault.Client
	k8s       kubernetes.Interface
	namespace string
	cfg       *config.Config
	lifecycle bool
}

// NewVaultBootstrap creates a new VaultBootstrap reconciler.
func NewVaultBootstrap(vc *vault.Client, k8s kubernetes.Interface, namespace string, cfg *config.Config) *VaultBootstrap {
	return &VaultBootstrap{vault: vc, k8s: k8s, namespace: namespace, cfg: cfg, lifecycle: true}
}

// NewVaultConfiguration creates the normal-runtime reconciler. It can
// configure Vault only after authenticating through the pod's projected
// Kubernetes token and can never initialize, unseal, or load the recovery
// root token.
func NewVaultConfiguration(vc *vault.Client, k8s kubernetes.Interface, namespace string, cfg *config.Config) *VaultBootstrap {
	return &VaultBootstrap{vault: vc, k8s: k8s, namespace: namespace, cfg: cfg}
}

// Name implements Reconciler.
func (r *VaultBootstrap) Name() string {
	if r.lifecycle {
		return "vault-bootstrap"
	}
	return "vault-configuration"
}

// Reconcile drives the full Vault bootstrap each pass: init + unseal, then
// the idempotent Bootstrap (KV, k8s auth, audit, jwt, policies, roles).
//
// Resources that aren't always available (OAuth client creds for OIDC, the
// PKI CA bundle) are looked up best-effort here and passed in via
// BootstrapInput. ensureJWTAuth degrades to a partial config when they're
// missing.
func (r *VaultBootstrap) Reconcile(ctx context.Context) error {
	if !r.lifecycle {
		if err := r.vault.LoginKubernetes(ctx, r.cfg.Vault.KubernetesAuth.Role, r.cfg.Vault.KubernetesAuth.TokenPath); err != nil {
			return err
		}
		return r.configure(ctx)
	}

	newlyInitialized, err := r.vault.EnsureInitializedAndUnsealed(ctx, r.k8s, r.namespace)
	if err != nil {
		return err
	}
	if !newlyInitialized {
		if err := r.vault.LoadBootstrapToken(ctx, r.k8s, r.namespace); err != nil {
			return err
		}
	}

	if err := r.configure(ctx); err != nil {
		return err
	}
	return r.vault.LoginKubernetes(ctx, "butler-recovery", r.cfg.Vault.KubernetesAuth.TokenPath)
}

func (r *VaultBootstrap) configure(ctx context.Context) error {
	in := vault.BootstrapInput{
		OIDCIssuer:             r.cfg.OIDC.Issuer,
		OIDCAudience:           r.cfg.OIDC.Audience,
		JWTRoles:               buildJWTRoles(r.cfg),
		K8sEngine:              buildK8sEngineConfig(r.cfg),
		ButlerRole:             r.cfg.Vault.KubernetesAuth.Role,
		ButlerServiceAccount:   "butler",
		ButlerNamespace:        r.namespace,
		RecoveryRole:           "butler-recovery",
		RecoveryServiceAccount: "butler-recovery",
		ConsumerRoles:          buildConsumerRoles(r.cfg),
		PublicDomain:           r.cfg.Certificates.Domain,
	}

	// Best-effort: read OAuth client creds for Vault from KV. If they
	// haven't been written yet (OAuthClients reconciler hasn't run, or
	// Pocket-ID isn't reachable), in.OIDCClientID stays empty and
	// ensureJWTAuth skips the OIDC client config.
	if r.cfg.OIDC.Issuer != "" {
		data, err := r.vault.ReadSecret(ctx, vaultOAuthClientPath)
		if err != nil {
			slog.Debug("vault oauth creds not yet available", "err", err)
		} else if data != nil {
			if cid, ok := data["client_id"].(string); ok {
				in.OIDCClientID = cid
			}
			if cs, ok := data["client_secret"].(string); ok {
				in.OIDCClientSecret = cs
			}
		}

	}

	if err := r.vault.Bootstrap(ctx, in); err != nil {
		return err
	}

	// Vault Kubernetes secrets engine: separate call because it needs k8s
	// access (to read the long-lived token-reviewer JWT). Soft-failing here
	// is fine — the function itself logs + returns nil on retry-worthy
	// conditions, and only returns a real error when something's
	// fundamentally wrong.
	if err := r.vault.BootstrapK8sEngine(ctx, r.k8s, in.K8sEngine); err != nil {
		return err
	}
	return nil
}

func buildConsumerRoles(cfg *config.Config) []vault.ConsumerRoleSpec {
	out := make([]vault.ConsumerRoleSpec, 0, len(cfg.Vault.KubernetesAuth.Consumers))
	for _, role := range cfg.Vault.KubernetesAuth.Consumers {
		out = append(out, vault.ConsumerRoleSpec{
			Name: role.Name, Namespace: role.Namespace,
			ServiceAccounts: role.ServiceAccounts, Paths: role.Paths,
		})
	}
	return out
}

func buildJWTRoles(cfg *config.Config) []vault.JWTRoleSpec {
	out := make([]vault.JWTRoleSpec, 0, len(cfg.K8sIssuance.Roles))
	for _, r := range cfg.K8sIssuance.Roles {
		out = append(out, vault.JWTRoleSpec{
			Name:          r.Name,
			PocketIDGroup: r.PocketIDGroup,
			Policies:      r.VaultPolicies,
			TTL:           r.TTL,
			MaxTTL:        r.MaxTTL,
		})
	}
	return out
}

func buildK8sEngineConfig(cfg *config.Config) vault.K8sEngineConfig {
	roles := make([]vault.K8sEngineRoleSpec, 0, len(cfg.K8sIssuance.Roles))
	for _, r := range cfg.K8sIssuance.Roles {
		roles = append(roles, vault.K8sEngineRoleSpec{
			Name:               r.Name,
			ServiceAccountName: r.ServiceAccountName,
			TTL:                r.TTL,
			MaxTTL:             r.MaxTTL,
		})
	}
	return vault.K8sEngineConfig{
		Enabled:                cfg.K8sIssuance.Enabled,
		HostNamespace:          cfg.K8sIssuance.HostNamespace,
		TokenReviewerNamespace: cfg.K8sIssuance.TokenReviewerNamespace,
		TokenReviewerSA:        cfg.K8sIssuance.TokenReviewerSA,
		TokenReviewerSecret:    cfg.K8sIssuance.TokenReviewerRef,
		Roles:                  roles,
	}
}
