package vault

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	vaultapi "github.com/hashicorp/vault/api"
)

// ListJWTRoleNames returns the role names on the auth/jwt mount. A missing
// mount means no roles exist; that is reported as an empty list so the orphans
// report can distinguish "nothing exists" from "the provider is down".
func (c *Client) ListJWTRoleNames(ctx context.Context) ([]string, error) {
	return c.listNames(ctx, "auth/jwt/role")
}

// ListKubernetesRoleNames returns the role names on the kubernetes/ secrets
// engine. A missing engine reports an empty list, as with ListJWTRoleNames.
func (c *Client) ListKubernetesRoleNames(ctx context.Context) ([]string, error) {
	return c.listNames(ctx, "kubernetes/roles")
}

// ListACLPolicyNames returns every ACL policy name Vault knows about,
// including the built-in policies the caller must filter out.
func (c *Client) ListACLPolicyNames(ctx context.Context) ([]string, error) {
	names, err := c.raw.Sys().ListPoliciesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ACL policies: %w", err)
	}
	return names, nil
}

// DeclaredPolicyNames returns the non-built-in ACL policy names Butler's
// bootstrap writes for the given configuration: the policies that gate the
// Vault auth roles, the VSO policy, the Kubernetes issuance policies, and one
// policy per Vault Secrets Operator consumer. Keep this list in sync with
// Bootstrap and the ensure* helpers it calls.
//
// The vault-admin, vault-read and vault-metadata policies are only written once
// Pocket ID is configured, because they back the jwt auth roles.
func DeclaredPolicyNames(oidcIssuer string, consumerRoles []ConsumerRoleSpec) []string {
	names := []string{
		"butler",
		"butler-recovery",
		"vso",
		"k8s-admin",
		"k8s-operator",
		"k8s-viewer",
	}
	if oidcIssuer != "" {
		names = append(names, "vault-admin", "vault-read", "vault-metadata")
	}
	for _, role := range consumerRoles {
		// ensureConsumerRoles writes an ACL policy named after each consumer
		// role, which is why vso-<namespace> policies survive a rename.
		names = append(names, role.Name)
	}
	return names
}

func (c *Client) listNames(ctx context.Context, path string) ([]string, error) {
	secret, err := c.raw.Logical().ListWithContext(ctx, path)
	if err != nil {
		var responseError *vaultapi.ResponseError
		if errors.As(err, &responseError) && responseError.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("listing %s: %w", path, err)
	}
	if secret == nil {
		return nil, nil
	}
	raw, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return nil, nil
	}
	names := make([]string, 0, len(raw))
	for _, item := range raw {
		if name, ok := item.(string); ok && name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
