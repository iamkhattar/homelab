package access

import (
	"context"
	"fmt"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

type CredentialIssuer interface {
	IssueKubernetesCredential(context.Context, string, string, time.Duration) (vault.KubernetesCredential, error)
}

type CredentialService struct {
	issuer    CredentialIssuer
	namespace string
	roles     map[string]time.Duration
}

func NewCredentialService(issuer CredentialIssuer, cfg config.K8sIssuanceConfig) (*CredentialService, error) {
	roles := make(map[string]time.Duration, len(cfg.Roles))
	for _, role := range cfg.Roles {
		maximum := 8 * time.Hour
		if role.MaxTTL != "" {
			parsed, err := time.ParseDuration(role.MaxTTL)
			if err != nil {
				return nil, fmt.Errorf("parsing maximum TTL for %s: %w", role.Name, err)
			}
			maximum = parsed
		}
		roles[role.Name] = maximum
	}
	return &CredentialService{issuer: issuer, namespace: cfg.HostNamespace, roles: roles}, nil
}

func (s *CredentialService) Issue(ctx context.Context, principal Principal, role string, ttl time.Duration) (vault.KubernetesCredential, error) {
	if principal.Role != Admin {
		return vault.KubernetesCredential{}, fmt.Errorf("only homelab administrators may issue Kubernetes credentials")
	}
	maximum, ok := s.roles[role]
	if !ok {
		return vault.KubernetesCredential{}, fmt.Errorf("Kubernetes credential role %q is not approved", role)
	}
	if ttl <= 0 {
		return vault.KubernetesCredential{}, fmt.Errorf("credential TTL must be positive")
	}
	if ttl > maximum {
		return vault.KubernetesCredential{}, fmt.Errorf("credential TTL %s exceeds role maximum %s", ttl, maximum)
	}
	return s.issuer.IssueKubernetesCredential(ctx, role, s.namespace, ttl)
}
