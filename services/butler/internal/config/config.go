package config

import (
	_ "embed"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed butler.yaml
var configData []byte

// Config is the top-level Butler configuration.
type Config struct {
	Vault     VaultConfig     `yaml:"vault"`
	Server    ServerConfig    `yaml:"server"`
	Reconcile ReconcileConfig `yaml:"reconcile"`
	OIDC      OIDCConfig      `yaml:"oidc"`
	Secrets   []SecretSpec    `yaml:"secrets"`

	// Derived at load time, not from YAML.
	Namespace         string        `yaml:"-"`
	LogLevel          string        `yaml:"-"`
	ReconcileInterval time.Duration `yaml:"-"`
}

// VaultConfig holds Vault connection settings.
type VaultConfig struct {
	Address string `yaml:"address"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port string `yaml:"port"`
}

// ReconcileConfig holds reconciliation settings.
type ReconcileConfig struct {
	Interval string `yaml:"interval"`
}

// OIDCConfig holds OIDC/JWT auth settings.
type OIDCConfig struct {
	Issuer string `yaml:"issuer"`
}

// SecretSpec defines a single Vault KV path and its expected keys.
type SecretSpec struct {
	Path string               `yaml:"path"`
	Keys map[string]KeyConfig `yaml:"keys"`
}

// KeyConfig describes how a secret key's value is determined.
// Exactly one of Length, Static, or Template should be set.
type KeyConfig struct {
	Length   int    `yaml:"length,omitempty"`
	Static   string `yaml:"static,omitempty"`
	Template string `yaml:"template,omitempty"`
}

// Load parses the embedded butler.yaml config. Environment variables override
// select fields.
func Load() (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		return nil, fmt.Errorf("parsing embedded config: %w", err)
	}

	// Env overrides.
	cfg.Vault.Address = envOr("VAULT_ADDR", cfg.Vault.Address)
	cfg.Server.Port = envOr("SERVER_PORT", cfg.Server.Port)
	cfg.OIDC.Issuer = envOr("OIDC_ISSUER", cfg.OIDC.Issuer)
	cfg.Namespace = envOr("NAMESPACE", "security")
	cfg.LogLevel = envOr("LOG_LEVEL", "info")

	intervalStr := envOr("RECONCILE_INTERVAL", cfg.Reconcile.Interval)
	d, err := time.ParseDuration(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("parsing reconcile interval %q: %w", intervalStr, err)
	}
	cfg.ReconcileInterval = d

	return &cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
