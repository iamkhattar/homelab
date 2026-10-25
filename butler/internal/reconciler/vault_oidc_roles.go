package reconciler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

const (
	defaultOIDCRoleTTL    = "1h"
	defaultOIDCRoleMaxTTL = "8h"
)

// oidcRoleName restricts group names, which double as Vault role names, to a
// shape Vault accepts in a path segment.
var oidcRoleName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// VaultOIDCRoles converges the Vault auth/jwt roles declared by VaultOIDCRole
// resources.
//
// The Vault chart owns these resources: Vault owns its policy set, so it also
// declares which group carries which policies. Butler only writes provider
// state, exactly as it does for the other platform kinds. The Kubernetes
// secrets-engine roles in k8sIssuance are a different thing and stay in
// VaultBootstrapK8sEngine: one describes human authorization, the other
// describes which ServiceAccount a minted token belongs to.
type VaultOIDCRoles struct {
	vault     *vault.Client
	resources platform.Resources
	domain    string
	namespace string
}

// NewVaultOIDCRoles creates the reconciler. namespace bounds where an authority
// declaration may live; a role declared in any other namespace is ignored.
func NewVaultOIDCRoles(vc *vault.Client, resources platform.Resources, domain, namespace string) *VaultOIDCRoles {
	return &VaultOIDCRoles{vault: vc, resources: resources, domain: domain, namespace: namespace}
}

func (r *VaultOIDCRoles) Name() string { return "vault-oidc-roles" }

func (r *VaultOIDCRoles) Reconcile(ctx context.Context) error {
	items, err := r.resources.ListVaultOIDCRoles(ctx)
	if err != nil {
		return fmt.Errorf("listing VaultOIDCRoles: %w", err)
	}
	if len(items) == 0 || r.namespace == "" {
		return nil
	}

	// bound_audiences must be the Vault OIDC client ID, which Butler generates
	// once Pocket ID's client exists. Until then the roles cannot be written and
	// the next pass retries.
	credentials, err := r.vault.ReadSecretIfExists(ctx, vaultOAuthClientPath)
	if err != nil {
		return fmt.Errorf("reading Vault OIDC client credential: %w", err)
	}
	clientID, _ := credentials["client_id"].(string)
	if clientID == "" {
		slog.Debug("vault oidc client is not ready, deferring VaultOIDCRole reconciliation")
		return nil
	}

	// Authority declarations are only trusted beside the Vault release.
	// Treating any namespace as authoritative would let a namespace owner grant
	// themselves a Vault policy.
	scoped := make([]*platform.VaultOIDCRole, 0, len(items))
	owners := make(map[string][]string, len(items))
	for i := range items {
		role := &items[i]
		if role.Namespace != r.namespace {
			slog.Warn("ignoring VaultOIDCRole outside the Vault namespace",
				"namespace", role.Namespace, "name", role.Name, "expected", r.namespace)
			continue
		}
		scoped = append(scoped, role)
		owners[role.Spec.Group] = append(owners[role.Spec.Group], role.Name)
	}

	var failures []error
	for i := range scoped {
		role := scoped[i]
		// Ambiguity is refused before anything is written: last-writer-wins
		// would let one declaration silently override another's policies.
		if names := owners[role.Spec.Group]; len(names) > 1 {
			failures = append(failures, fmt.Errorf("Vault group %q is declared by more than one resource: %s", role.Spec.Group, strings.Join(names, ", ")))
			continue
		}
		if err := r.ensureRole(ctx, role, clientID); err != nil {
			failures = append(failures, fmt.Errorf("reconciling Vault role %q: %w", role.Spec.Group, err))
		}
	}
	return errors.Join(failures...)
}

func (r *VaultOIDCRoles) ensureRole(ctx context.Context, role *platform.VaultOIDCRole, clientID string) error {
	if !oidcRoleName.MatchString(role.Spec.Group) {
		return fmt.Errorf("group %q is not a valid Vault role name", role.Spec.Group)
	}
	if len(role.Spec.Policies) == 0 {
		return errors.New("at least one Vault policy is required")
	}
	for _, policy := range role.Spec.Policies {
		if !oidcRoleName.MatchString(policy) {
			return fmt.Errorf("policy %q is not a valid Vault policy name", policy)
		}
	}
	ttl, err := oidcRoleDuration(role.Spec.TTL, defaultOIDCRoleTTL)
	if err != nil {
		return fmt.Errorf("ttl: %w", err)
	}
	maxTTL, err := oidcRoleDuration(role.Spec.MaxTTL, defaultOIDCRoleMaxTTL)
	if err != nil {
		return fmt.Errorf("maxTTL: %w", err)
	}
	if maxTTL < ttl {
		return fmt.Errorf("maxTTL %s must not be shorter than ttl %s", role.Spec.MaxTTL, role.Spec.TTL)
	}

	desired := map[string]interface{}{
		"role_type":       "oidc",
		"user_claim":      "sub",
		"groups_claim":    "groups",
		"oidc_scopes":     []string{"openid", "profile", "email", "groups"},
		"bound_audiences": []string{clientID},
		"bound_claims": map[string]interface{}{
			"groups": []string{role.Spec.Group},
		},
		"policies": role.Spec.Policies,
		"ttl":      ttl,
		"max_ttl":  maxTTL,
		"allowed_redirect_uris": []string{
			// Loopback for the Vault CLI; UI callback path for the Vault UI.
			"http://localhost:8250/oidc/callback",
			"https://vault." + r.domain + "/ui/vault/auth/jwt/oidc/callback",
		},
	}

	path := "auth/jwt/role/" + role.Spec.Group
	current, err := r.vault.Raw().Logical().ReadWithContext(ctx, path)
	if err != nil && !isMissingVaultRole(err) {
		return fmt.Errorf("reading existing role: %w", err)
	}
	if current != nil && roleMatches(desired, current.Data) {
		slog.Debug("vault oidc role already matches its declaration", "role", role.Spec.Group)
		return nil
	}

	if _, err := r.vault.Raw().Logical().WriteWithContext(ctx, path, desired); err != nil {
		return fmt.Errorf("writing role: %w", err)
	}
	slog.Info("wrote vault oidc role", "role", role.Spec.Group, "policies", role.Spec.Policies)
	return nil
}

// roleMatches compares the fields Butler owns. Vault normalizes durations to
// seconds, so the comparison converts the declared ttl to the same unit.
func roleMatches(desired map[string]interface{}, actual map[string]interface{}) bool {
	if actual == nil {
		return false
	}
	for _, field := range []string{"role_type", "user_claim", "groups_claim"} {
		if value, _ := actual[field].(string); value != desired[field] {
			return false
		}
	}
	for _, field := range []string{"oidc_scopes", "bound_audiences", "allowed_redirect_uris"} {
		if !reflect.DeepEqual(vaultStringSlice(actual[field]), desired[field]) {
			return false
		}
	}
	// Vault reports role policies as token_policies and may omit the legacy
	// alias on newer versions. Read whichever is present so a converged role is
	// not rewritten on every pass.
	actualPolicies := vaultStringSlice(actual["policies"])
	if len(actualPolicies) == 0 {
		actualPolicies = vaultStringSlice(actual["token_policies"])
	}
	if !reflect.DeepEqual(actualPolicies, desired["policies"]) {
		return false
	}
	if !reflect.DeepEqual(vaultBoundGroups(actual["bound_claims"]), vaultBoundGroups(desired["bound_claims"])) {
		return false
	}
	ttl, _ := time.ParseDuration(desired["ttl"].(string))
	maxTTL, _ := time.ParseDuration(desired["max_ttl"].(string))
	return vaultSeconds(actual["ttl"]) == int64(ttl.Seconds()) &&
		vaultSeconds(actual["max_ttl"]) == int64(maxTTL.Seconds())
}

func vaultStringSlice(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		if direct, ok := raw.([]string); ok {
			return direct
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.(string); ok {
			out = append(out, value)
		}
	}
	return out
}

func vaultBoundGroups(raw interface{}) []string {
	claims, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	return vaultStringSlice(claims["groups"])
}

func vaultSeconds(raw interface{}) int64 {
	switch value := raw.(type) {
	case int64:
		return value
	case int:
		return int64(value)
	case float64:
		return int64(value)
	case interface{ Int64() (int64, error) }:
		parsed, err := value.Int64()
		if err != nil {
			return -1
		}
		return parsed
	case string:
		parsed, err := time.ParseDuration(value + "s")
		if err != nil {
			return -1
		}
		return int64(parsed.Seconds())
	}
	return -1
}

func oidcRoleDuration(value, fallback string) (string, error) {
	if strings.TrimSpace(value) == "" {
		value = fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return "", err
	}
	if parsed <= 0 {
		return "", errors.New("must be positive")
	}
	return value, nil
}

func isMissingVaultRole(err error) bool {
	var responseError *vaultapi.ResponseError
	return errors.As(err, &responseError) && responseError.StatusCode == 404
}
