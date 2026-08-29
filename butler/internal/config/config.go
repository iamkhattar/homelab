package config

import (
	_ "embed"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// configData is the compile-time fallback. In production butler mounts a
// chart-rendered ConfigMap at /etc/butler/config.yaml (path configurable via
// BUTLER_CONFIG_PATH) and that takes precedence. The embedded copy is just
// a safety net for local dev / unit tests / and crash-loop recovery if the
// ConfigMap somehow goes missing.
//
//go:embed butler.yaml
var configData []byte

// DefaultConfigPath is the location the butler chart mounts the rendered
// ConfigMap to. Override with BUTLER_CONFIG_PATH for local testing.
const DefaultConfigPath = "/etc/butler/config.yaml"

// Config is the top-level Butler configuration. Externalized into a
// chart-rendered ConfigMap as of Phase 1B so secrets/oidc/oauthClients/pki/
// k8sIssuance can be configured at deploy time without rebuilding the
// container image.
type Config struct {
	Vault              VaultConfig             `yaml:"vault"`
	Server             ServerConfig            `yaml:"server"`
	Reconcile          ReconcileConfig         `yaml:"reconcile"`
	OIDC               OIDCConfig              `yaml:"oidc"`
	Secrets            []SecretSpec            `yaml:"secrets"`
	ManagedCredentials []ManagedCredentialSpec `yaml:"managedCredentials"`
	PocketIDGroups     []PocketIDGroupSpec     `yaml:"pocketIdGroups"`
	OAuthClients       []OAuthClientSpec       `yaml:"oauthClients"`
	PKI                PKIConfig               `yaml:"pki"`
	K8sIssuance        K8sIssuanceConfig       `yaml:"k8sIssuance"`
	Garage             GarageConfig            `yaml:"garage"`

	// Derived at load time, not from YAML.
	Namespace         string        `yaml:"-"`
	LogLevel          string        `yaml:"-"`
	ReconcileInterval time.Duration `yaml:"-"`
}

// VaultConfig holds Vault connection settings.
type VaultConfig struct {
	Address        string                    `yaml:"address"`
	KubernetesAuth VaultKubernetesAuthConfig `yaml:"kubernetesAuth"`
}

// VaultKubernetesAuthConfig controls Butler's post-bootstrap identity and the
// narrow Kubernetes-auth roles used by Vault Secrets Operator consumers.
type VaultKubernetesAuthConfig struct {
	Role      string                  `yaml:"role"`
	TokenPath string                  `yaml:"tokenPath"`
	Consumers []VaultConsumerRoleSpec `yaml:"consumers"`
}

// VaultConsumerRoleSpec binds one Kubernetes identity to read-only KV paths.
type VaultConsumerRoleSpec struct {
	Name            string   `yaml:"name"`
	Namespace       string   `yaml:"namespace"`
	ServiceAccounts []string `yaml:"serviceAccounts"`
	Paths           []string `yaml:"paths"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port string `yaml:"port"`
}

// ReconcileConfig holds reconciliation settings.
type ReconcileConfig struct {
	Interval string `yaml:"interval"`
}

// OIDCConfig holds OIDC/JWT auth settings. The Issuer is used both for
// butler's own API authz (verifies tokens from this issuer) and as the
// upstream issuer for Vault's auth/jwt mount.
type OIDCConfig struct {
	Issuer           string   `yaml:"issuer"`
	Audience         string   `yaml:"audience"`
	AllowedAudiences []string `yaml:"allowedAudiences"`
	AdminURL         string   `yaml:"adminURL"`
}

// SecretSpec defines a single Vault KV path and its expected keys.
type SecretSpec struct {
	Path string               `yaml:"path"`
	Keys map[string]KeyConfig `yaml:"keys"`
}

// KeyConfig describes how a secret key's value is determined.
// Exactly one of Length, HexLength, Static, or Template must be set.
type KeyConfig struct {
	Length    int    `yaml:"length,omitempty"`
	HexLength int    `yaml:"hexLength,omitempty"`
	Static    string `yaml:"static,omitempty"`
	Template  string `yaml:"template,omitempty"`
}

// ManagedCredentialSpec publishes a least-privilege view of a source Vault
// secret to a consumer-specific path. Values are copied server-side by Butler;
// application Vault roles never receive access to the shared source path.
type ManagedCredentialSpec struct {
	SourcePath      string            `yaml:"sourcePath"`
	DestinationPath string            `yaml:"destinationPath"`
	Keys            map[string]string `yaml:"keys"`
	Static          map[string]string `yaml:"static,omitempty"`
}

// PocketIDGroupSpec declares an authorization group Butler owns in Pocket ID.
// Human users and passkeys are deliberately not created by Butler; an operator
// enrolls people in Pocket ID and assigns them to these reconciled groups.
type PocketIDGroupSpec struct {
	Name         string `yaml:"name"`
	FriendlyName string `yaml:"friendlyName"`
}

type GarageConfig struct {
	Enabled        bool               `yaml:"enabled"`
	Endpoint       string             `yaml:"endpoint"`
	AdminTokenPath string             `yaml:"adminTokenPath"`
	AdminTokenKey  string             `yaml:"adminTokenKey"`
	Layout         GarageLayoutConfig `yaml:"layout"`
	Buckets        []GarageBucketSpec `yaml:"buckets"`
}

type GarageLayoutConfig struct {
	Zone          string `yaml:"zone"`
	CapacityBytes int64  `yaml:"capacityBytes"`
}

type GarageBucketSpec struct {
	Name           string `yaml:"name"`
	CredentialPath string `yaml:"credentialPath"`
	Read           bool   `yaml:"read"`
	Write          bool   `yaml:"write"`
	Owner          bool   `yaml:"owner"`
}

// OAuthClientSpec declares an OIDC client butler should provision in
// Pocket-ID. Butler stores the resulting {client_id, client_secret} at
// secret/oauth/<name>.
type OAuthClientSpec struct {
	Name         string   `yaml:"name"`
	Kind         string   `yaml:"kind"` // "confidential" or "public"
	RedirectURIs []string `yaml:"redirectURIs"`
}

// PKIConfig is the configuration knob for the Vault PKI engines butler
// manages. The defaults (in butler.yaml) match the Phase 1A constants;
// surfacing them here lets operators change the org/CN/domain at deploy
// time without rebuilding the image.
type PKIConfig struct {
	RootCN         string   `yaml:"rootCN"`
	IntCN          string   `yaml:"intCN"`
	Organization   string   `yaml:"organization"`
	AllowedDomains []string `yaml:"allowedDomains"`
	RoleMaxTTL     string   `yaml:"roleMaxTTL"`
}

// K8sIssuanceConfig configures Vault's kubernetes/ secrets engine. Vault
// becomes the broker that mints short-lived K8s ServiceAccount tokens for
// the operator workflows in Phase 1B.
type K8sIssuanceConfig struct {
	Enabled                bool                  `yaml:"enabled"`
	HostNamespace          string                `yaml:"hostNamespace"`          // where vault-managed-{admin,operator,viewer} live (kube-system)
	TokenReviewerNamespace string                `yaml:"tokenReviewerNamespace"` // where vault-k8s-token-reviewer lives (security)
	TokenReviewerSA        string                `yaml:"tokenReviewerSA"`        // name of the token-reviewer SA
	TokenReviewerRef       string                `yaml:"tokenReviewerRef"`       // reference to the long-lived token Secret object
	Roles                  []K8sIssuanceRoleSpec `yaml:"roles"`
}

// K8sIssuanceRoleSpec maps one Vault kubernetes/roles/<name> entry plus
// the matching JWT role and policy set.
type K8sIssuanceRoleSpec struct {
	Name               string   `yaml:"name"`               // homelab-admin / operator / viewer
	ServiceAccountName string   `yaml:"serviceAccountName"` // vault-managed-admin etc.
	TTL                string   `yaml:"ttl"`
	MaxTTL             string   `yaml:"maxTTL"`
	PocketIDGroup      string   `yaml:"pocketIdGroup"` // group claim that selects this role
	VaultPolicies      []string `yaml:"vaultPolicies"` // attached to the Vault token issued from auth/jwt
}

// Load resolves butler's config. Resolution order:
//  1. If BUTLER_CONFIG_PATH is set and the file exists, use it.
//  2. If /etc/butler/config.yaml exists (chart-mounted ConfigMap), use it.
//  3. Fall back to the embedded butler.yaml.
//
// Environment variables override select scalar fields after the file load.
func Load() (*Config, error) {
	raw, source, err := readConfigBytes()
	if err != nil {
		return nil, err
	}
	slog.Info("loaded butler config", "source", source)

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config from %s: %w", source, err)
	}

	cfg.Vault.Address = envOr("VAULT_ADDR", cfg.Vault.Address)
	cfg.Server.Port = envOr("SERVER_PORT", cfg.Server.Port)
	cfg.OIDC.Issuer = envOr("OIDC_ISSUER", cfg.OIDC.Issuer)
	cfg.OIDC.Audience = envOr("OIDC_AUDIENCE", cfg.OIDC.Audience)
	cfg.Namespace = envOr("NAMESPACE", "security")
	cfg.LogLevel = envOr("LOG_LEVEL", "info")

	intervalStr := envOr("RECONCILE_INTERVAL", cfg.Reconcile.Interval)
	d, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("parsing reconcile interval %q: %w", intervalStr, err)
	}
	cfg.ReconcileInterval = d
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config from %s: %w", source, err)
	}

	return &cfg, nil
}

var pocketIDNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,127}$`)

// Validate rejects incomplete API-managed declarations before Butler starts a
// reconciliation loop that can never converge.
func (c *Config) Validate() error {
	if c.OIDC.Issuer != "" {
		if c.OIDC.Audience == "" {
			return fmt.Errorf("oidc.audience is required when oidc.issuer is configured")
		}
		if _, err := absoluteHTTPURL(c.OIDC.Issuer); err != nil {
			return fmt.Errorf("oidc.issuer: %w", err)
		}
	}
	if len(c.PocketIDGroups)+len(c.OAuthClients) > 0 {
		if _, err := absoluteHTTPURL(c.OIDC.AdminURL); err != nil {
			return fmt.Errorf("oidc.adminURL: %w", err)
		}
	}
	seenGroups := map[string]struct{}{}
	for _, group := range c.PocketIDGroups {
		if !pocketIDNamePattern.MatchString(group.Name) || strings.TrimSpace(group.FriendlyName) == "" {
			return fmt.Errorf("invalid pocketIdGroups entry %q", group.Name)
		}
		if _, exists := seenGroups[group.Name]; exists {
			return fmt.Errorf("duplicate pocketIdGroups name %q", group.Name)
		}
		seenGroups[group.Name] = struct{}{}
	}
	seenClients := map[string]struct{}{}
	for _, client := range c.OAuthClients {
		if !pocketIDNamePattern.MatchString(client.Name) {
			return fmt.Errorf("invalid oauth client name %q", client.Name)
		}
		if client.Kind != "public" && client.Kind != "confidential" {
			return fmt.Errorf("oauth client %q kind must be public or confidential", client.Name)
		}
		if len(client.RedirectURIs) == 0 {
			return fmt.Errorf("oauth client %q requires at least one redirect URI", client.Name)
		}
		for _, redirect := range client.RedirectURIs {
			if _, err := absoluteHTTPURL(redirect); err != nil {
				return fmt.Errorf("oauth client %q redirect URI: %w", client.Name, err)
			}
		}
		if _, exists := seenClients[client.Name]; exists {
			return fmt.Errorf("duplicate oauth client name %q", client.Name)
		}
		seenClients[client.Name] = struct{}{}
	}
	return nil
}

func absoluteHTTPURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("must be an absolute http(s) URL")
	}
	return parsed, nil
}

func readConfigBytes() ([]byte, string, error) {
	if envPath := os.Getenv("BUTLER_CONFIG_PATH"); envPath != "" {
		// #nosec G304,G703 -- the deployment operator explicitly controls this
		// configuration mount path; Butler never derives it from a network request.
		data, err := os.ReadFile(envPath)
		if err == nil {
			return data, envPath, nil
		}
		return nil, "", fmt.Errorf("reading BUTLER_CONFIG_PATH=%s: %w", envPath, err)
	}
	if data, err := os.ReadFile(DefaultConfigPath); err == nil {
		return data, DefaultConfigPath, nil
	}
	return configData, "embedded butler.yaml", nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
