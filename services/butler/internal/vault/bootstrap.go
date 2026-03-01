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

// Bootstrap ensures all Vault server-side configuration is in place.
// Every operation is idempotent.
func (c *Client) Bootstrap(ctx context.Context) error {
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
