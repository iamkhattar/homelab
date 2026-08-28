package vault

import (
	"context"
	"fmt"
	"time"
)

type KubernetesCredential struct {
	Role           string    `json:"role"`
	ServiceAccount string    `json:"serviceAccount"`
	Namespace      string    `json:"namespace"`
	Token          string    `json:"token"`
	LeaseID        string    `json:"leaseId"`
	TTLSeconds     int64     `json:"ttlSeconds"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func (c *Client) IssueKubernetesCredential(ctx context.Context, role, namespace string, ttl time.Duration) (KubernetesCredential, error) {
	if !safeVaultComponent.MatchString(role) || !safeVaultComponent.MatchString(namespace) {
		return KubernetesCredential{}, fmt.Errorf("invalid Kubernetes credential role or namespace")
	}
	secret, err := c.raw.Logical().WriteWithContext(ctx, "kubernetes/creds/"+role, map[string]interface{}{
		"kubernetes_namespace": namespace,
		"ttl":                  ttl.String(),
	})
	if err != nil {
		return KubernetesCredential{}, fmt.Errorf("issuing Kubernetes credential: %w", err)
	}
	if secret == nil || secret.Data == nil {
		return KubernetesCredential{}, fmt.Errorf("Vault returned no Kubernetes credential")
	}
	token, _ := secret.Data["service_account_token"].(string)
	serviceAccount, _ := secret.Data["service_account_name"].(string)
	issuedNamespace, _ := secret.Data["service_account_namespace"].(string)
	if token == "" || serviceAccount == "" || issuedNamespace == "" {
		return KubernetesCredential{}, fmt.Errorf("Vault returned an incomplete Kubernetes credential")
	}
	ttlSeconds := int64(secret.LeaseDuration)
	return KubernetesCredential{
		Role: role, ServiceAccount: serviceAccount, Namespace: issuedNamespace,
		Token: token, LeaseID: secret.LeaseID, TTLSeconds: ttlSeconds,
		ExpiresAt: time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second),
	}, nil
}
