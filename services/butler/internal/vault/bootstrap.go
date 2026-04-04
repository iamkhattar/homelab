package vault

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"

	vaultapi "github.com/hashicorp/vault/api"
)

//go:embed policies/vso.hcl
var vsoPolicy string
//go:embed policies/vault_admin.hcl
var vaultAdminPolicy string

//go:embed policies/vault_read.hcl
var vaultReadPolicy string

// Bootstrap ensures all Vault server-side configuration is in place.
// Every operation is idempotent.
func (c *Client) Bootstrap(ctx context.Context, oidcIssuer string) error {
	if err := c.ensureKVv2(ctx); err != nil {
		return err
	}
	if err := c.ensureKubernetesAuth(ctx); err != nil {
		return err
	}
	if err := c.ensurePolicy(ctx); err != nil {
		return err
	}
	if err := c.ensureRole(ctx); err != nil {
		return err
	}
	if err := c.ensureJWTAuth(ctx, oidcIssuer); err != nil {
		return err
	}
	if err := c.ensureJWTPolicies(ctx, oidcIssuer); err != nil {
		return err
	}
	if err := c.ensureJWTRoles(ctx, oidcIssuer); err != nil {
		return err
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
		"bound_service_account_names":      []string{"vault-secrets-operator"},
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

// ensureJWTAuth enables and configures the jwt auth method for OIDC-issued JWTs.
// If oidcIssuer is empty, JWT auth bootstrapping is skipped.
func (c *Client) ensureJWTAuth(ctx context.Context, oidcIssuer string) error {
	if oidcIssuer == "" {
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

	_, err = c.raw.Logical().WriteWithContext(ctx, "auth/jwt/config", map[string]interface{}{
		"oidc_discovery_url": oidcIssuer,
		"bound_issuer":       oidcIssuer,
	})
	if err != nil {
		return fmt.Errorf("configuring jwt auth: %w", err)
	}

	slog.Info("configured jwt auth method", "issuer", oidcIssuer)
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

// ensureJWTRoles creates or updates jwt roles mapping OIDC groups to policies.
// If oidcIssuer is empty, JWT role bootstrapping is skipped.
func (c *Client) ensureJWTRoles(ctx context.Context, oidcIssuer string) error {
	if oidcIssuer == "" {
		return nil
	}

	_, err := c.raw.Logical().WriteWithContext(ctx, "auth/jwt/role/vault-admin", map[string]interface{}{
		"role_type":      "jwt",
		"user_claim":     "sub",
		"groups_claim":   "groups",
		"bound_audiences": []string{"vault"},
		"bound_claims": map[string]interface{}{
			"groups": []string{"vault_admin"},
		},
		"policies": []string{"vault-admin"},
		"ttl":      "1h",
	})
	if err != nil {
		return fmt.Errorf("writing jwt role vault-admin: %w", err)
	}
	slog.Info("wrote jwt role", "role", "vault-admin")

	_, err = c.raw.Logical().WriteWithContext(ctx, "auth/jwt/role/vault-read", map[string]interface{}{
		"role_type":      "jwt",
		"user_claim":     "sub",
		"groups_claim":   "groups",
		"bound_audiences": []string{"vault"},
		"bound_claims": map[string]interface{}{
			"groups": []string{"vault_read"},
		},
		"policies": []string{"vault-read"},
		"ttl":      "1h",
	})
	if err != nil {
		return fmt.Errorf("writing jwt role vault-read: %w", err)
	}
	slog.Info("wrote jwt role", "role", "vault-read")

	return nil
}
