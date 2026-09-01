package vault

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//go:embed policies/vso.hcl
var vsoPolicy string

//go:embed policies/butler.hcl
var butlerPolicy string

//go:embed policies/butler_recovery.hcl
var butlerRecoveryPolicy string

//go:embed policies/vault_admin.hcl
var vaultAdminPolicy string

//go:embed policies/vault_read.hcl
var vaultReadPolicy string

//go:embed policies/cert_manager.hcl
var certManagerPolicy string

//go:embed policies/k8s_admin.hcl
var k8sAdminPolicy string

//go:embed policies/k8s_operator.hcl
var k8sOperatorPolicy string

//go:embed policies/k8s_viewer.hcl
var k8sViewerPolicy string

// BootstrapInput is the typed bag of inputs Bootstrap needs. Externalizing
// the args keeps the call site explicit and lets the reconciler lazily
// resolve fields that come from Vault itself (PKI CA bundle, OAuth creds).
type BootstrapInput struct {
	// OIDC config for the Vault auth/jwt mount.
	OIDCIssuer       string
	OIDCAudience     string
	OIDCClientID     string
	OIDCClientSecret string
	PublicDomain     string

	// JWT roles — bind Pocket-ID groups to Vault policies. Reused from
	// the K8sIssuance role specs so a single source of truth describes
	// 'homelab-admin' across auth/jwt and kubernetes/.
	JWTRoles []JWTRoleSpec

	// Kubernetes secrets engine config. Disabled when
	// K8sEngine.Enabled is false.
	K8sEngine K8sEngineConfig

	ButlerRole             string
	ButlerServiceAccount   string
	ButlerNamespace        string
	RecoveryRole           string
	RecoveryServiceAccount string
	ConsumerRoles          []ConsumerRoleSpec
}

type ConsumerRoleSpec struct {
	Name            string
	Namespace       string
	ServiceAccounts []string
	Paths           []string
}

// PKIConfig remains only for recognizing and cleaning up clusters created by
// the superseded private-PKI design. Bootstrap no longer calls ensurePKI.
type PKIConfig struct {
	RootCN         string
	IntCN          string
	Organization   string
	AllowedDomains []string
	RoleMaxTTL     string
}

// JWTRoleSpec is the subset of role config Bootstrap needs to create a
// Vault auth/jwt role.
type JWTRoleSpec struct {
	Name          string
	PocketIDGroup string
	Policies      []string
	TTL           string // default 1h if empty
	MaxTTL        string // default 8h if empty
}

// K8sEngineConfig configures the Vault Kubernetes secrets engine.
type K8sEngineConfig struct {
	Enabled                bool
	HostNamespace          string
	TokenReviewerNamespace string
	TokenReviewerSA        string
	TokenReviewerSecret    string
	Roles                  []K8sEngineRoleSpec
}

// K8sEngineRoleSpec configures one kubernetes/roles/<name> entry.
type K8sEngineRoleSpec struct {
	Name               string
	ServiceAccountName string
	TTL                string
	MaxTTL             string
}

const (
	pkiRootMount         = "pki"
	pkiIntMount          = "pki_int"
	pkiDefaultRole       = "homelab-default"
	pkiRootTTL           = "87600h"
	pkiIntTTL            = "26280h"
	auditDevicePath      = "file"
	auditDeviceFile      = "/vault/audit/audit.log"
	certManagerNamespace = "cert-manager"
	certManagerSA        = "cert-manager"
)

// Bootstrap ensures all Vault server-side configuration is in place.
// Every operation is idempotent.
//
// Steps that depend on resources that aren't always available (Pocket-ID
// OAuth credentials, the Vault Kubernetes engine token reviewer SA token)
// gracefully degrade — they no-op when their inputs are missing and the
// next reconcile pass re-tries.
func (c *Client) Bootstrap(ctx context.Context, in BootstrapInput) error {
	if err := c.ensureKVv2(ctx); err != nil {
		return err
	}
	if err := c.ensureKubernetesAuth(ctx); err != nil {
		return err
	}
	if err := c.ensureButlerAuth(ctx, in); err != nil {
		return err
	}
	if err := c.ensurePolicy(ctx); err != nil {
		return err
	}
	if err := c.ensureRole(ctx); err != nil {
		return err
	}
	if err := c.ensureConsumerRoles(ctx, in.ConsumerRoles); err != nil {
		return err
	}
	if err := c.ensureAuditDevice(ctx); err != nil {
		return err
	}
	if err := c.ensureJWTAuth(ctx, in); err != nil {
		return err
	}
	if err := c.ensureJWTPolicies(ctx, in.OIDCIssuer); err != nil {
		return err
	}
	if err := c.ensureJWTRoles(ctx, in); err != nil {
		return err
	}
	if err := c.ensureK8sPolicies(ctx, in.K8sEngine); err != nil {
		return err
	}
	return nil
}

func (c *Client) ensureButlerAuth(ctx context.Context, in BootstrapInput) error {
	if in.ButlerRole == "" || in.ButlerServiceAccount == "" || in.ButlerNamespace == "" {
		return fmt.Errorf("butler kubernetes auth requires role, service account, and namespace")
	}
	if err := c.raw.Sys().PutPolicyWithContext(ctx, "butler", butlerPolicy); err != nil {
		return fmt.Errorf("writing butler policy: %w", err)
	}
	_, err := c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/role/"+in.ButlerRole, map[string]interface{}{
		"bound_service_account_names":      []string{in.ButlerServiceAccount},
		"bound_service_account_namespaces": []string{in.ButlerNamespace},
		"bound_audiences":                  []string{"vault"},
		"policies":                         []string{"butler"},
		"ttl":                              "30m",
		"max_ttl":                          "1h",
	})
	if err != nil {
		return fmt.Errorf("writing butler kubernetes auth role: %w", err)
	}
	if in.RecoveryRole == "" || in.RecoveryServiceAccount == "" {
		return fmt.Errorf("butler recovery kubernetes auth requires role and service account")
	}
	if err := c.raw.Sys().PutPolicyWithContext(ctx, "butler-recovery", butlerRecoveryPolicy); err != nil {
		return fmt.Errorf("writing butler recovery policy: %w", err)
	}
	_, err = c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/role/"+in.RecoveryRole, map[string]interface{}{
		"bound_service_account_names":      []string{in.RecoveryServiceAccount},
		"bound_service_account_namespaces": []string{in.ButlerNamespace},
		"bound_audiences":                  []string{"vault"},
		"policies":                         []string{"butler-recovery"},
		"ttl":                              "10m",
		"max_ttl":                          "15m",
	})
	if err != nil {
		return fmt.Errorf("writing butler recovery kubernetes auth role: %w", err)
	}
	return nil
}

var safeVaultComponent = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var safeVaultPath = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

func (c *Client) ensureConsumerRoles(ctx context.Context, roles []ConsumerRoleSpec) error {
	for _, role := range roles {
		if !safeVaultComponent.MatchString(role.Name) || !safeVaultComponent.MatchString(role.Namespace) {
			return fmt.Errorf("invalid vault consumer role %q or namespace %q", role.Name, role.Namespace)
		}
		if len(role.ServiceAccounts) == 0 || len(role.Paths) == 0 {
			return fmt.Errorf("vault consumer role %s requires serviceAccounts and paths", role.Name)
		}
		var policy strings.Builder
		for _, path := range role.Paths {
			if !safeVaultPath.MatchString(path) || strings.Contains(path, "..") {
				return fmt.Errorf("invalid vault consumer path %q", path)
			}
			fmt.Fprintf(&policy, "path \"secret/data/%s\" { capabilities = [\"read\"] }\n", path)
		}
		if err := c.raw.Sys().PutPolicyWithContext(ctx, role.Name, policy.String()); err != nil {
			return fmt.Errorf("writing consumer policy %s: %w", role.Name, err)
		}
		_, err := c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/role/"+role.Name, map[string]interface{}{
			"bound_service_account_names":      role.ServiceAccounts,
			"bound_service_account_namespaces": []string{role.Namespace},
			"bound_audiences":                  []string{"vault"},
			"policies":                         []string{role.Name},
			"ttl":                              "15m",
			"max_ttl":                          "1h",
		})
		if err != nil {
			return fmt.Errorf("writing consumer role %s: %w", role.Name, err)
		}
	}
	return nil
}

// ensureKVv2 enables the KV-v2 secrets engine at "secret/" if not already present.
func (c *Client) ensureKVv2(ctx context.Context) error {
	mounts, err := c.raw.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("listing mounts: %w", err)
	}

	if _, ok := mounts["secret/"]; ok {
		slog.Debug("kv-v2 engine already mounted at secret/")
		return nil
	}

	err = c.raw.Sys().MountWithContext(ctx, "secret", &vaultapi.MountInput{
		Type: "kv",
		Options: map[string]string{
			"version": "2",
		},
	})
	if err != nil {
		return fmt.Errorf("mounting kv-v2 at secret/: %w", err)
	}

	slog.Info("enabled kv-v2 secrets engine at secret/")
	return nil
}

// ensureKubernetesAuth enables and configures the kubernetes auth method.
func (c *Client) ensureKubernetesAuth(ctx context.Context) error {
	auths, err := c.raw.Sys().ListAuthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("listing auth methods: %w", err)
	}

	if _, ok := auths["kubernetes/"]; !ok {
		err = c.raw.Sys().EnableAuthWithOptionsWithContext(ctx, "kubernetes", &vaultapi.EnableAuthOptions{
			Type: "kubernetes",
		})
		if err != nil {
			return fmt.Errorf("enabling kubernetes auth: %w", err)
		}
		slog.Info("enabled kubernetes auth method")
	} else {
		slog.Debug("kubernetes auth method already enabled")
	}

	// Configure kubernetes auth using in-cluster credentials.
	kubeHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubePort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if kubeHost == "" || kubePort == "" {
		return fmt.Errorf("KUBERNETES_SERVICE_HOST/PORT not set — are we running in-cluster?")
	}

	caCert, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return fmt.Errorf("reading service account ca.crt: %w", err)
	}

	_, err = c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/config", map[string]interface{}{
		"kubernetes_host":    fmt.Sprintf("https://%s:%s", kubeHost, kubePort),
		"kubernetes_ca_cert": string(caCert),
	})
	if err != nil {
		return fmt.Errorf("configuring kubernetes auth: %w", err)
	}

	slog.Info("configured kubernetes auth method")
	return nil
}

// ensurePolicy creates or updates the VSO read policy.
func (c *Client) ensurePolicy(ctx context.Context) error {
	err := c.raw.Sys().PutPolicyWithContext(ctx, "vso", vsoPolicy)
	if err != nil {
		return fmt.Errorf("writing vso policy: %w", err)
	}
	slog.Info("wrote vso policy")
	return nil
}

// ensureRole creates or updates the VSO kubernetes auth role.
func (c *Client) ensureRole(ctx context.Context) error {
	_, err := c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/role/vso", map[string]interface{}{
		"bound_service_account_names":      []string{"vault-secrets-operator", "vault-secrets-operator-controller-manager"},
		"bound_service_account_namespaces": []string{"security"},
		"policies":                         []string{"vso"},
		"ttl":                              "1h",
	})
	if err != nil {
		return fmt.Errorf("writing vso role: %w", err)
	}
	slog.Info("wrote vso kubernetes auth role")
	return nil
}

// ensureJWTAuth enables and configures the jwt auth method for OIDC-issued
// JWTs from Pocket-ID. If the issuer is empty, this is a no-op (typical
// during initial bootstrap when Pocket ID is not reachable yet).
//
// If the issuer is set but the OAuth client_id/client_secret aren't yet
// available (the PocketIDClient reconciler writes them to secret/oauth/vault
// on a later pass), we still mount auth/jwt and set the discovery URL —
// but skip configuring the OIDC client. That's enough for `vault login
// -method=jwt` (token-only JWTs), and is upgraded to full OIDC once the
// creds materialize.
func (c *Client) ensureJWTAuth(ctx context.Context, in BootstrapInput) error {
	if in.OIDCIssuer == "" {
		slog.Info("oidc issuer not configured, skipping vault jwt auth bootstrap")
		return nil
	}

	auths, err := c.raw.Sys().ListAuthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("listing auth methods: %w", err)
	}

	if _, ok := auths["jwt/"]; !ok {
		err = c.raw.Sys().EnableAuthWithOptionsWithContext(ctx, "jwt", &vaultapi.EnableAuthOptions{
			Type: "jwt",
		})
		if err != nil {
			return fmt.Errorf("enabling jwt auth: %w", err)
		}
		slog.Info("enabled jwt auth method")
	} else {
		slog.Debug("jwt auth method already enabled")
	}

	cfg := map[string]interface{}{
		"oidc_discovery_url": in.OIDCIssuer,
		"bound_issuer":       in.OIDCIssuer,
	}
	// Full OIDC flow (used by `vault login -method=oidc` and the Vault UI)
	// requires client credentials. Without them only the JWT mode works.
	if in.OIDCClientID != "" && in.OIDCClientSecret != "" {
		cfg["oidc_client_id"] = in.OIDCClientID
		cfg["oidc_client_secret"] = in.OIDCClientSecret
	}

	if _, err := c.raw.Logical().WriteWithContext(ctx, "auth/jwt/config", cfg); err != nil {
		return fmt.Errorf("configuring jwt auth: %w", err)
	}

	slog.Info("configured jwt auth method", "issuer", in.OIDCIssuer,
		"oidc_client_configured", in.OIDCClientID != "")
	return nil
}

// ensureJWTPolicies creates or updates policies used by JWT roles.
// If oidcIssuer is empty, JWT policy bootstrapping is skipped.
func (c *Client) ensureJWTPolicies(ctx context.Context, oidcIssuer string) error {
	if oidcIssuer == "" {
		return nil
	}

	if err := c.raw.Sys().PutPolicyWithContext(ctx, "vault-admin", vaultAdminPolicy); err != nil {
		return fmt.Errorf("writing vault-admin policy: %w", err)
	}
	slog.Info("wrote vault-admin policy")

	if err := c.raw.Sys().PutPolicyWithContext(ctx, "vault-read", vaultReadPolicy); err != nil {
		return fmt.Errorf("writing vault-read policy: %w", err)
	}
	slog.Info("wrote vault-read policy")

	return nil
}

// ensureJWTRoles creates or updates jwt roles mapping Pocket-ID groups to
// Vault policies. Roles come from BootstrapInput.JWTRoles (driven by the
// chart's k8sIssuance.roles list — they share a name/group/policy schema).
//
// If JWTRoles is empty or OIDC isn't configured, this is a no-op.
//
// Each role binds:
//   - audience = the vault OAuth client_id (or "vault" as a fallback when
//     OIDCClientID is empty, e.g. pre-OAuth-reconciler-pass).
//   - groups claim = "groups" (Pocket-ID's standard claim name).
//   - bound_claims.groups = [<spec.PocketIDGroup>].
func (c *Client) ensureJWTRoles(ctx context.Context, in BootstrapInput) error {
	if in.OIDCIssuer == "" || len(in.JWTRoles) == 0 {
		return nil
	}

	audience := in.OIDCClientID
	if audience == "" {
		audience = "vault"
	}

	for _, spec := range in.JWTRoles {
		ttl := spec.TTL
		if ttl == "" {
			ttl = "1h"
		}
		maxTTL := spec.MaxTTL
		if maxTTL == "" {
			maxTTL = "8h"
		}
		path := "auth/jwt/role/" + spec.Name
		_, err := c.raw.Logical().WriteWithContext(ctx, path, map[string]interface{}{
			"role_type":       "oidc",
			"user_claim":      "sub",
			"groups_claim":    "groups",
			"oidc_scopes":     []string{"openid", "profile", "email", "groups"},
			"bound_audiences": []string{audience},
			"bound_claims": map[string]interface{}{
				"groups": []string{spec.PocketIDGroup},
			},
			"policies": spec.Policies,
			"ttl":      ttl,
			"max_ttl":  maxTTL,
			"allowed_redirect_uris": []string{
				// Loopback for vault CLI; UI callback path for the Vault UI.
				"http://localhost:8250/oidc/callback",
				"https://vault." + firstDomain(in) + "/ui/vault/auth/jwt/oidc/callback",
			},
		})
		if err != nil {
			return fmt.Errorf("writing jwt role %s: %w", spec.Name, err)
		}
		slog.Info("wrote jwt role", "role", spec.Name, "group", spec.PocketIDGroup)
	}
	return nil
}

// ensureK8sPolicies writes the k8s_{admin,operator,viewer} HCL policies so
// the JWT roles can attach them. We always write all three (cheap, keeps
// drift detection trivial) regardless of whether the K8s engine is enabled.
func (c *Client) ensureK8sPolicies(ctx context.Context, _ K8sEngineConfig) error {
	policies := map[string]string{
		"k8s-admin":    k8sAdminPolicy,
		"k8s-operator": k8sOperatorPolicy,
		"k8s-viewer":   k8sViewerPolicy,
	}
	for name, body := range policies {
		if err := c.raw.Sys().PutPolicyWithContext(ctx, name, body); err != nil {
			return fmt.Errorf("writing vault policy %s: %w", name, err)
		}
		slog.Debug("wrote vault policy", "name", name)
	}
	return nil
}

// firstDomain pulls the first allowed PKI domain to use in role redirect
// URIs. Falls back to "shivlab.com" as a sane default when nothing is set.
//
// (We don't have direct access to PKIConfig here; the redirect URI is
// best-effort and harmless if it's slightly wrong since Vault checks the
// list on login.)
func firstDomain(in BootstrapInput) string {
	if in.PublicDomain != "" {
		return in.PublicDomain
	}
	return "6940469.xyz"
}

// BootstrapK8sEngine enables and configures Vault's kubernetes/ secrets
// engine. Vault becomes the broker that mints short-lived ServiceAccount
// tokens for the vault-managed-{admin,operator,viewer} SAs, fronted by the
// homelab-{admin,operator,viewer} roles.
//
// Soft-fails (returns nil with a warning log) when:
//   - K8sEngine.Enabled is false for disabled or local-development deployments.
//   - The token-reviewer Secret exists but hasn't yet been populated by
//     the kube-controller-manager (typical on the very first reconcile
//     pass right after the rbac-policies chart applied).
//
// Hard-fails (returns error) when:
//   - Config is malformed (missing namespaces / names).
//   - The Secret outright doesn't exist (the chart hasn't applied yet).
//   - Vault rejects the engine mount or config write.
//
// The caller (VaultBootstrap reconciler) should treat the error path as
// retry-worthy — the scheduler will re-tick and succeed on a later pass
// once everything has converged.
func (c *Client) BootstrapK8sEngine(ctx context.Context, k8s kubernetes.Interface, cfg K8sEngineConfig) error {
	if !cfg.Enabled {
		slog.Debug("k8s secrets engine disabled in config, skipping")
		return nil
	}
	if cfg.TokenReviewerNamespace == "" || cfg.TokenReviewerSecret == "" || cfg.HostNamespace == "" {
		return fmt.Errorf("k8s engine config missing required fields (TokenReviewerNamespace, TokenReviewerSecret, HostNamespace)")
	}

	mounts, err := c.raw.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("listing mounts: %w", err)
	}
	if _, ok := mounts["kubernetes/"]; !ok {
		if err := c.raw.Sys().MountWithContext(ctx, "kubernetes", &vaultapi.MountInput{
			Type: "kubernetes",
		}); err != nil {
			return fmt.Errorf("mounting kubernetes secrets engine: %w", err)
		}
		slog.Info("mounted kubernetes secrets engine")
	} else {
		slog.Debug("kubernetes secrets engine already mounted")
	}

	tokenSecret, err := k8s.CoreV1().Secrets(cfg.TokenReviewerNamespace).Get(ctx, cfg.TokenReviewerSecret, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading token-reviewer secret %s/%s: %w",
			cfg.TokenReviewerNamespace, cfg.TokenReviewerSecret, err)
	}
	tokenBytes, ok := tokenSecret.Data["token"]
	if !ok || len(tokenBytes) == 0 {
		// Soft-fail: kube-controller-manager hasn't populated the Secret
		// yet. Next reconcile pass will retry.
		slog.Warn("token-reviewer secret has no token yet, retry next pass",
			"namespace", cfg.TokenReviewerNamespace, "name", cfg.TokenReviewerSecret)
		return nil
	}

	caCert, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return fmt.Errorf("reading service account ca.crt: %w", err)
	}
	kubeHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubePort := os.Getenv("KUBERNETES_SERVICE_PORT")
	if kubeHost == "" || kubePort == "" {
		return fmt.Errorf("KUBERNETES_SERVICE_HOST/PORT not set — are we running in-cluster?")
	}

	if _, err := c.raw.Logical().WriteWithContext(ctx, "kubernetes/config", map[string]interface{}{
		"kubernetes_host":     fmt.Sprintf("https://%s:%s", kubeHost, kubePort),
		"kubernetes_ca_cert":  string(caCert),
		"service_account_jwt": string(tokenBytes),
	}); err != nil {
		return fmt.Errorf("configuring kubernetes/config: %w", err)
	}
	slog.Info("configured kubernetes secrets engine")

	for _, role := range cfg.Roles {
		ttl := role.TTL
		if ttl == "" {
			ttl = "1h"
		}
		maxTTL := role.MaxTTL
		if maxTTL == "" {
			maxTTL = "8h"
		}
		if _, err := c.raw.Logical().WriteWithContext(ctx, "kubernetes/roles/"+role.Name, map[string]interface{}{
			"service_account_name":          role.ServiceAccountName,
			"allowed_kubernetes_namespaces": []string{cfg.HostNamespace},
			"token_default_ttl":             ttl,
			"token_max_ttl":                 maxTTL,
		}); err != nil {
			return fmt.Errorf("writing kubernetes/roles/%s: %w", role.Name, err)
		}
		slog.Info("wrote kubernetes engine role", "role", role.Name, "sa", role.ServiceAccountName, "ttl", ttl)
	}
	return nil
}

// ensurePKI sets up the two-tier Vault PKI used to issue homelab certificates.
//
//   - pki/      — long-lived root CA (Homelab Root CA, 10y)
//   - pki_int/  — intermediate CA signed by the root (3y), used by cert-manager
//   - role homelab-default — issuance role for *.shivlab.com (30d max TTL)
//
// All steps are idempotent so this function can run on every reconcile pass.
func (c *Client) ensurePKI(ctx context.Context, cfg PKIConfig) error {
	if cfg.RootCN == "" || cfg.IntCN == "" || cfg.Organization == "" || len(cfg.AllowedDomains) == 0 {
		return fmt.Errorf("pki config requires rootCN, intCN, organization, and allowedDomains")
	}
	if cfg.RoleMaxTTL == "" {
		cfg.RoleMaxTTL = "720h"
	}
	mounts, err := c.raw.Sys().ListMountsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("listing mounts: %w", err)
	}

	// Mount root CA engine.
	if _, ok := mounts[pkiRootMount+"/"]; !ok {
		err = c.raw.Sys().MountWithContext(ctx, pkiRootMount, &vaultapi.MountInput{
			Type: "pki",
			Config: vaultapi.MountConfigInput{
				MaxLeaseTTL: pkiRootTTL,
			},
		})
		if err != nil {
			return fmt.Errorf("mounting pki at %s/: %w", pkiRootMount, err)
		}
		slog.Info("mounted pki engine", "path", pkiRootMount)
	} else {
		slog.Debug("pki engine already mounted", "path", pkiRootMount)
	}

	// Mount intermediate CA engine.
	if _, ok := mounts[pkiIntMount+"/"]; !ok {
		err = c.raw.Sys().MountWithContext(ctx, pkiIntMount, &vaultapi.MountInput{
			Type: "pki",
			Config: vaultapi.MountConfigInput{
				MaxLeaseTTL: pkiIntTTL,
			},
		})
		if err != nil {
			return fmt.Errorf("mounting pki at %s/: %w", pkiIntMount, err)
		}
		slog.Info("mounted pki engine", "path", pkiIntMount)
	} else {
		slog.Debug("pki engine already mounted", "path", pkiIntMount)
	}

	// Generate the root CA if it doesn't already exist. We detect "exists"
	// by trying to read the CA cert — a freshly mounted PKI has none.
	rootExists, err := c.hasIssuedCA(ctx, pkiRootMount)
	if err != nil {
		return fmt.Errorf("checking root CA existence: %w", err)
	}
	if !rootExists {
		_, err := c.raw.Logical().WriteWithContext(ctx, pkiRootMount+"/root/generate/internal", map[string]interface{}{
			"common_name":  cfg.RootCN,
			"organization": cfg.Organization,
			"ttl":          pkiRootTTL,
		})
		if err != nil {
			return fmt.Errorf("generating root CA: %w", err)
		}
		slog.Info("generated root CA", "cn", cfg.RootCN)
	} else {
		slog.Debug("root CA already exists, skipping generation")
	}

	// Generate the intermediate CSR + sign it with the root + import the
	// signed cert. Skipped if the intermediate already has a CA cert.
	intExists, err := c.hasIssuedCA(ctx, pkiIntMount)
	if err != nil {
		return fmt.Errorf("checking intermediate CA existence: %w", err)
	}
	if !intExists {
		csrResp, err := c.raw.Logical().WriteWithContext(ctx, pkiIntMount+"/intermediate/generate/internal", map[string]interface{}{
			"common_name":  cfg.IntCN,
			"organization": cfg.Organization,
		})
		if err != nil {
			return fmt.Errorf("generating intermediate CSR: %w", err)
		}
		if csrResp == nil || csrResp.Data == nil {
			return fmt.Errorf("intermediate CSR response was empty")
		}
		csr, _ := csrResp.Data["csr"].(string)
		if csr == "" {
			return fmt.Errorf("intermediate CSR response missing csr field")
		}

		signResp, err := c.raw.Logical().WriteWithContext(ctx, pkiRootMount+"/root/sign-intermediate", map[string]interface{}{
			"csr":    csr,
			"format": "pem_bundle",
			"ttl":    pkiIntTTL,
		})
		if err != nil {
			return fmt.Errorf("signing intermediate CSR with root: %w", err)
		}
		if signResp == nil || signResp.Data == nil {
			return fmt.Errorf("sign-intermediate response was empty")
		}
		signed, _ := signResp.Data["certificate"].(string)
		if signed == "" {
			return fmt.Errorf("sign-intermediate response missing certificate field")
		}

		if _, err := c.raw.Logical().WriteWithContext(ctx, pkiIntMount+"/intermediate/set-signed", map[string]interface{}{
			"certificate": signed,
		}); err != nil {
			return fmt.Errorf("importing signed intermediate: %w", err)
		}
		slog.Info("generated and signed intermediate CA", "cn", cfg.IntCN)
	} else {
		slog.Debug("intermediate CA already exists, skipping generation")
	}

	// Create / update the default issuance role. Idempotent so it can run
	// every reconcile to absorb config drift.
	if _, err := c.raw.Logical().WriteWithContext(ctx, pkiIntMount+"/roles/"+pkiDefaultRole, map[string]interface{}{
		"allowed_domains":  cfg.AllowedDomains,
		"allow_subdomains": true,
		"max_ttl":          cfg.RoleMaxTTL,
		"key_type":         "rsa",
		"key_bits":         2048,
	}); err != nil {
		return fmt.Errorf("writing pki role %s: %w", pkiDefaultRole, err)
	}
	slog.Info("wrote pki role", "role", pkiDefaultRole)
	return nil
}

// hasIssuedCA reports whether the given PKI mount has a CA certificate. A
// freshly mounted pki engine has no CA; once root/generate (or
// intermediate/set-signed) succeeds, /cert/ca returns a populated cert.
func (c *Client) hasIssuedCA(ctx context.Context, mount string) (bool, error) {
	resp, err := c.raw.Logical().ReadWithContext(ctx, mount+"/cert/ca")
	if err != nil {
		// Vault returns 204 when no CA exists; the SDK may surface that as
		// a nil response and no error, or as an explicit error string.
		// Treat both as "no CA yet" rather than failing the bootstrap.
		var rerr *vaultapi.ResponseError
		if errors.As(err, &rerr) && (rerr.StatusCode == 404 || rerr.StatusCode == 204) {
			return false, nil
		}
		return false, err
	}
	if resp == nil || resp.Data == nil {
		return false, nil
	}
	cert, _ := resp.Data["certificate"].(string)
	return cert != "", nil
}

// ensureCertManagerAuth wires cert-manager to Vault's kubernetes auth method.
// cert-manager authenticates as its own ServiceAccount; the role policy gives
// it just enough to request certs from the homelab-default PKI role.
func (c *Client) ensureCertManagerAuth(ctx context.Context) error {
	if err := c.raw.Sys().PutPolicyWithContext(ctx, "cert-manager-issuer", certManagerPolicy); err != nil {
		return fmt.Errorf("writing cert-manager-issuer policy: %w", err)
	}
	slog.Info("wrote vault policy", "name", "cert-manager-issuer")

	if _, err := c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/role/"+certManagerSA, map[string]interface{}{
		"bound_service_account_names":      []string{certManagerSA},
		"bound_service_account_namespaces": []string{certManagerNamespace},
		"policies":                         []string{"cert-manager-issuer"},
		"ttl":                              "1h",
	}); err != nil {
		return fmt.Errorf("writing cert-manager kubernetes auth role: %w", err)
	}
	slog.Info("wrote kubernetes auth role", "role", certManagerSA)
	return nil
}

// ensureAuditDevice enables the file audit device at /vault/audit/audit.log.
// The vault chart mounts this path via server.auditStorage. We disable
// log_raw because the default is already 'false' and we want consistent
// behavior across vault versions.
func (c *Client) ensureAuditDevice(ctx context.Context) error {
	devices, err := c.raw.Sys().ListAuditWithContext(ctx)
	if err != nil {
		return fmt.Errorf("listing audit devices: %w", err)
	}
	if _, ok := devices[auditDevicePath+"/"]; ok {
		slog.Debug("audit device already enabled", "path", auditDevicePath)
		return nil
	}
	if err := c.raw.Sys().EnableAuditWithOptionsWithContext(ctx, auditDevicePath, &vaultapi.EnableAuditOptions{
		Type: "file",
		Options: map[string]string{
			"file_path": auditDeviceFile,
			"log_raw":   "false",
		},
	}); err != nil {
		return fmt.Errorf("enabling file audit device: %w", err)
	}
	slog.Info("enabled file audit device", "path", auditDeviceFile)
	return nil
}

// CAChain reads a legacy private-PKI chain for migration diagnostics. New
// clusters use the publicly trusted cert-manager ACME issuer and never call it.
func (c *Client) CAChain(ctx context.Context) (string, error) {
	resp, err := c.raw.Logical().ReadWithContext(ctx, pkiIntMount+"/cert/ca_chain")
	if err != nil {
		return "", fmt.Errorf("reading %s/cert/ca_chain: %w", pkiIntMount, err)
	}
	if resp == nil || resp.Data == nil {
		return "", fmt.Errorf("pki ca_chain not yet available")
	}
	chain, _ := resp.Data["certificate"].(string)
	if chain == "" {
		return "", fmt.Errorf("pki ca_chain response missing certificate field")
	}
	return chain, nil
}
