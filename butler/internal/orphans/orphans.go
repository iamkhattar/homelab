// Package orphans compares the objects declared in the repository with the
// objects that exist in Pocket ID and Vault.
//
// Butler never deletes provider data: a renamed or removed declaration leaves
// its provider object behind. This read-only report is how an operator finds
// that debris, and the inverse condition (a declaration whose provider object
// never appeared) without browsing two UIs.
package orphans

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

// Provider object kinds. They name the provider surface, not a Kubernetes kind,
// because the same concept has a different declaration on each side.
const (
	KindPocketIDClient      = "pocket-id-client"
	KindPocketIDGroup       = "pocket-id-group"
	KindVaultJWTRole        = "vault-jwt-role"
	KindVaultKubernetesRole = "vault-kubernetes-role"
	KindVaultPolicy         = "vault-policy"
)

// Source status values.
const (
	StatusOK          = "ok"
	StatusUnavailable = "unavailable"
)

// Entry is one orphaned provider object or one absent declaration. It carries
// names and kinds only; no provider value, credential or secret ever reaches
// the API.
type Entry struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Why  string `json:"why"`
}

// Source reports whether one provider query succeeded. A failed query marks
// only that source unavailable; the remaining comparisons still run.
type Source struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Report is the complete comparison. Orphaned objects exist in a provider but
// are declared nowhere; missing objects are declared but absent from their
// provider.
type Report struct {
	Orphaned []Entry  `json:"orphaned"`
	Missing  []Entry  `json:"missing"`
	Sources  []Source `json:"sources"`
}

// Clean reports whether the providers match the declarations exactly.
func (r Report) Clean() bool {
	return len(r.Orphaned) == 0 && len(r.Missing) == 0
}

// Declarations is the subset of the platform store the report reads.
type Declarations interface {
	ListPocketIDClients(context.Context) ([]platform.PocketIDClient, error)
	ListPocketIDGroups(context.Context) ([]platform.PocketIDGroup, error)
	ListVaultOIDCRoles(context.Context) ([]platform.VaultOIDCRole, error)
}

// PocketID is the provider surface the report reads.
type PocketID interface {
	ListClients(context.Context) ([]pocketid.OIDCClient, error)
	ListGroups(context.Context) ([]pocketid.UserGroup, error)
}

// Vault is the provider surface the report reads. Every method is a read.
type Vault interface {
	ListJWTRoleNames(context.Context) ([]string, error)
	ListKubernetesRoleNames(context.Context) ([]string, error)
	ListACLPolicyNames(context.Context) ([]string, error)
}

// Service builds orphan reports. It never writes to a provider.
type Service struct {
	declarations Declarations
	pocketID     PocketID
	vault        Vault

	kubernetesRoles []string
	policies        []string
	namespace       string
}

// New creates the report service. cfg supplies the declarations that live in
// Butler's ConfigMap rather than a Kubernetes resource: the Kubernetes
// secrets-engine roles and the policy set Butler writes.
func New(declarations Declarations, pocketID PocketID, vaultClient Vault, cfg *config.Config) *Service {
	consumers := make([]vault.ConsumerRoleSpec, 0, len(cfg.Vault.KubernetesAuth.Consumers))
	for _, consumer := range cfg.Vault.KubernetesAuth.Consumers {
		consumers = append(consumers, vault.ConsumerRoleSpec{Name: consumer.Name, Namespace: consumer.Namespace})
	}
	kubernetesRoles := make([]string, 0, len(cfg.K8sIssuance.Roles))
	for _, role := range cfg.K8sIssuance.Roles {
		kubernetesRoles = append(kubernetesRoles, role.Name)
	}
	return &Service{
		declarations:    declarations,
		pocketID:        pocketID,
		vault:           vaultClient,
		kubernetesRoles: kubernetesRoles,
		policies:        vault.DeclaredPolicyNames(cfg.OIDC.Issuer, consumers),
		namespace:       cfg.Namespace,
	}
}

// Report compares every supported provider surface. A provider or declaration
// failure is confined to the affected source; the rest of the report is still
// returned.
func (s *Service) Report(ctx context.Context) Report {
	report := Report{Orphaned: []Entry{}, Missing: []Entry{}, Sources: []Source{}}

	clients, clientsErr := s.declarations.ListPocketIDClients(ctx)
	report.compare(ctx, compareInput{
		kind:        KindPocketIDClient,
		source:      "pocket-id-clients",
		declared:    namesOfClients(clients),
		declaredErr: wrapDeclarationError("PocketIDClients", clientsErr),
		actual: func(ctx context.Context) ([]string, error) {
			items, err := s.pocketID.ListClients(ctx)
			if err != nil {
				return nil, fmt.Errorf("listing Pocket ID clients: %w", err)
			}
			return namesOfOIDCClients(items), nil
		},
		orphanWhy:  "declared in no PocketIDClient resource",
		missingWhy: "declared by a PocketIDClient resource but absent from Pocket ID",
	})

	groups, groupsErr := s.declarations.ListPocketIDGroups(ctx)
	report.compare(ctx, compareInput{
		kind:        KindPocketIDGroup,
		source:      "pocket-id-groups",
		declared:    namesOfGroups(groups),
		declaredErr: wrapDeclarationError("PocketIDGroups", groupsErr),
		actual: func(ctx context.Context) ([]string, error) {
			items, err := s.pocketID.ListGroups(ctx)
			if err != nil {
				return nil, fmt.Errorf("listing Pocket ID user groups: %w", err)
			}
			return namesOfUserGroups(items), nil
		},
		orphanWhy:  "declared in no PocketIDGroup resource",
		missingWhy: "declared by a PocketIDGroup resource but absent from Pocket ID",
	})

	roles, rolesErr := s.declarations.ListVaultOIDCRoles(ctx)
	report.compare(ctx, compareInput{
		kind:        KindVaultJWTRole,
		source:      "vault-jwt-roles",
		declared:    s.declaredJWTRoles(roles),
		declaredErr: wrapDeclarationError("VaultOIDCRoles", rolesErr),
		actual: func(ctx context.Context) ([]string, error) {
			return s.vault.ListJWTRoleNames(ctx)
		},
		orphanWhy:  "declared in no VaultOIDCRole resource",
		missingWhy: "declared by a VaultOIDCRole resource but absent from Vault",
	})

	report.compare(ctx, compareInput{
		kind:     KindVaultKubernetesRole,
		source:   "vault-kubernetes-roles",
		declared: s.kubernetesRoles,
		actual: func(ctx context.Context) ([]string, error) {
			return s.vault.ListKubernetesRoleNames(ctx)
		},
		orphanWhy:  "declared in no k8sIssuance.roles entry",
		missingWhy: "declared in k8sIssuance.roles but absent from Vault",
	})

	report.compare(ctx, compareInput{
		kind:     KindVaultPolicy,
		source:   "vault-policies",
		declared: s.policies,
		actual: func(ctx context.Context) ([]string, error) {
			names, err := s.vault.ListACLPolicyNames(ctx)
			if err != nil {
				return nil, err
			}
			return withoutVaultBuiltins(names), nil
		},
		orphanWhy:  "declared in no Butler policy set",
		missingWhy: "written by Butler configuration but absent from Vault",
	})

	sortEntries(report.Orphaned)
	sortEntries(report.Missing)
	return report
}

type compareInput struct {
	kind        string
	source      string
	declared    []string
	declaredErr error
	actual      func(context.Context) ([]string, error)
	orphanWhy   string
	missingWhy  string
}

func (r *Report) compare(ctx context.Context, in compareInput) {
	if in.declaredErr != nil {
		r.unavailable(in.source, in.declaredErr)
		return
	}
	actual, err := in.actual(ctx)
	if err != nil {
		r.unavailable(in.source, err)
		return
	}
	declared := set(in.declared)
	existing := set(actual)
	for _, name := range sortedNames(declared) {
		if _, ok := existing[name]; !ok {
			r.Missing = append(r.Missing, Entry{Kind: in.kind, Name: name, Why: in.missingWhy})
		}
	}
	for _, name := range sortedNames(existing) {
		if _, ok := declared[name]; !ok {
			r.Orphaned = append(r.Orphaned, Entry{Kind: in.kind, Name: name, Why: in.orphanWhy})
		}
	}
	r.Sources = append(r.Sources, Source{Name: in.source, Status: StatusOK})
}

func (r *Report) unavailable(source string, err error) {
	r.Sources = append(r.Sources, Source{Name: source, Status: StatusUnavailable, Error: err.Error()})
}

func (s *Service) declaredJWTRoles(roles []platform.VaultOIDCRole) []string {
	names := make([]string, 0, len(roles))
	for i := range roles {
		// vault-oidc-roles only trusts declarations beside the Vault release.
		// Report the same scope, otherwise ignored declarations would look
		// like missing provider objects forever.
		if s.namespace != "" && roles[i].Namespace != s.namespace {
			continue
		}
		names = append(names, roles[i].Spec.Group)
	}
	return names
}

func wrapDeclarationError(kind string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("listing %s: %w", kind, err)
}

func namesOfClients(items []platform.PocketIDClient) []string {
	names := make([]string, 0, len(items))
	for i := range items {
		names = append(names, items[i].Name)
	}
	return names
}

func namesOfGroups(items []platform.PocketIDGroup) []string {
	names := make([]string, 0, len(items))
	for i := range items {
		names = append(names, items[i].Name)
	}
	return names
}

func namesOfOIDCClients(items []pocketid.OIDCClient) []string {
	names := make([]string, 0, len(items))
	for i := range items {
		names = append(names, items[i].Name)
	}
	return names
}

func namesOfUserGroups(items []pocketid.UserGroup) []string {
	names := make([]string, 0, len(items))
	for i := range items {
		names = append(names, items[i].Name)
	}
	return names
}

// withoutVaultBuiltins removes policies Vault owns. Built-ins are never
// declared in the repository, so leaving them in would report the same
// always-present entries on every run.
func withoutVaultBuiltins(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name == "default" || name == "root" || strings.HasSuffix(name, "-ceiling") {
			continue
		}
		out = append(out, name)
	}
	return out
}

func set(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func sortedNames(names map[string]struct{}) []string {
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})
}
