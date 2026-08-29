package vault

import (
	"context"
	"fmt"
	"os"
)

// LoginKubernetes replaces the current client token with a short-lived token
// issued for Butler's projected ServiceAccount identity.
func (c *Client) LoginKubernetes(ctx context.Context, role, tokenPath string) error {
	if role == "" || tokenPath == "" {
		return fmt.Errorf("vault kubernetes login requires role and token path")
	}
	// #nosec G304 -- tokenPath is trusted deployment configuration, never a
	// request-derived filesystem path.
	jwt, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("reading projected vault service account token: %w", err)
	}
	secret, err := c.raw.Logical().WriteWithContext(ctx, "auth/kubernetes/login", map[string]interface{}{
		"role": role,
		"jwt":  string(jwt),
	})
	if err != nil {
		return fmt.Errorf("logging into vault with kubernetes auth: %w", err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return fmt.Errorf("vault kubernetes login returned no client token")
	}
	c.SetToken(secret.Auth.ClientToken)
	return nil
}
