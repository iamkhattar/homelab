package orphans

import (
	"context"
	"errors"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
)

type fakeDeclarations struct {
	clients []platform.PocketIDClient
	groups  []platform.PocketIDGroup
	roles   []platform.VaultOIDCRole

	clientsErr error
	groupsErr  error
	rolesErr   error
}

func (f fakeDeclarations) ListPocketIDClients(context.Context) ([]platform.PocketIDClient, error) {
	return f.clients, f.clientsErr
}

func (f fakeDeclarations) ListPocketIDGroups(context.Context) ([]platform.PocketIDGroup, error) {
	return f.groups, f.groupsErr
}

func (f fakeDeclarations) ListVaultOIDCRoles(context.Context) ([]platform.VaultOIDCRole, error) {
	return f.roles, f.rolesErr
}

type fakePocketID struct {
	clients []pocketid.OIDCClient
	groups  []pocketid.UserGroup

	clientsErr error
	groupsErr  error
}

func (f fakePocketID) ListClients(context.Context) ([]pocketid.OIDCClient, error) {
	return f.clients, f.clientsErr
}

func (f fakePocketID) ListGroups(context.Context) ([]pocketid.UserGroup, error) {
	return f.groups, f.groupsErr
}

type fakeVault struct {
	jwtRoles        []string
	kubernetesRoles []string
	policies        []string

	jwtErr        error
	kubernetesErr error
	policiesErr   error
}

func (f fakeVault) ListJWTRoleNames(context.Context) ([]string, error) {
	return f.jwtRoles, f.jwtErr
}

func (f fakeVault) ListKubernetesRoleNames(context.Context) ([]string, error) {
	return f.kubernetesRoles, f.kubernetesErr
}

func (f fakeVault) ListACLPolicyNames(context.Context) ([]string, error) {
	return f.policies, f.policiesErr
}

func testConfig() *config.Config {
	return &config.Config{
		Namespace: "security",
		OIDC:      config.OIDCConfig{Issuer: "https://auth.example.com"},
		K8sIssuance: config.K8sIssuanceConfig{
			Enabled: true,
			Roles:   []config.K8sIssuanceRoleSpec{{Name: "k8sadmin"}},
		},
		Vault: config.VaultConfig{KubernetesAuth: config.VaultKubernetesAuthConfig{
			Consumers: []config.VaultConsumerRoleSpec{{Name: "vso-homepage", Namespace: "homepage"}},
		}},
	}
}

// declaredPolicies mirrors vault.DeclaredPolicyNames for the test config.
func declaredPolicies() []string {
	return []string{
		"butler", "butler-recovery", "vso", "k8s-admin", "k8s-operator", "k8s-viewer",
		"vault-admin", "vault-read", "vault-metadata", "vso-homepage",
	}
}

func declaredInput(config *config.Config) (fakeDeclarations, fakePocketID, fakeVault) {
	declarations := fakeDeclarations{
		clients: []platform.PocketIDClient{{Spec: platform.PocketIDClientSpec{}}},
		groups:  []platform.PocketIDGroup{{Spec: platform.PocketIDGroupSpec{}}},
		roles:   []platform.VaultOIDCRole{{Spec: platform.VaultOIDCRoleSpec{Group: "homelab-admin"}}},
	}
	declarations.clients[0].Name = "grafana"
	declarations.groups[0].Name = "homelab-admin"
	declarations.roles[0].Namespace = "security"
	return declarations, fakePocketID{
		clients: []pocketid.OIDCClient{{Name: "grafana"}},
		groups:  []pocketid.UserGroup{{Name: "homelab-admin"}},
	}, fakeVault{
		jwtRoles:        []string{"homelab-admin"},
		kubernetesRoles: []string{"k8sadmin"},
		policies:        append(declaredPolicies(), "default", "root", "vso-ceiling"),
	}
}

func newService(declarations fakeDeclarations, pid fakePocketID, vc fakeVault, cfg *config.Config) *Service {
	return New(declarations, pid, vc, cfg)
}

func TestReportCleanIgnoresVaultBuiltins(t *testing.T) {
	declarations, pid, vc := declaredInput(testConfig())
	report := newService(declarations, pid, vc, testConfig()).Report(t.Context())

	if !report.Clean() {
		t.Fatalf("report not clean: orphaned=%+v missing=%+v", report.Orphaned, report.Missing)
	}
	for _, source := range report.Sources {
		if source.Status != StatusOK {
			t.Fatalf("source %s = %+v, want ok", source.Name, source)
		}
	}
}

func TestReportListsOrphanedProviderObjects(t *testing.T) {
	declarations, pid, vc := declaredInput(testConfig())
	pid.clients = append(pid.clients, pocketid.OIDCClient{Name: "retired-client"})
	pid.groups = append(pid.groups, pocketid.UserGroup{Name: "retired-group"})
	vc.jwtRoles = append(vc.jwtRoles, "retired-role")
	vc.kubernetesRoles = append(vc.kubernetesRoles, "retired-k8s-role")
	vc.policies = append(vc.policies, "retired-policy")

	report := newService(declarations, pid, vc, testConfig()).Report(t.Context())

	if report.Clean() {
		t.Fatal("report should not be clean")
	}
	want := []Entry{
		{Kind: KindPocketIDClient, Name: "retired-client", Why: "declared in no PocketIDClient resource"},
		{Kind: KindPocketIDGroup, Name: "retired-group", Why: "declared in no PocketIDGroup resource"},
		{Kind: KindVaultJWTRole, Name: "retired-role", Why: "declared in no VaultOIDCRole resource"},
		{Kind: KindVaultKubernetesRole, Name: "retired-k8s-role", Why: "declared in no k8sIssuance.roles entry"},
		{Kind: KindVaultPolicy, Name: "retired-policy", Why: "declared in no Butler policy set"},
	}
	assertEntries(t, "orphaned", report.Orphaned, want)
	if len(report.Missing) != 0 {
		t.Fatalf("missing = %+v, want none", report.Missing)
	}
}

func TestReportListsMissingDeclarations(t *testing.T) {
	declarations, _, vc := declaredInput(testConfig())
	declarations.clients = append(declarations.clients, platform.PocketIDClient{})
	declarations.clients[len(declarations.clients)-1].Name = "new-client"
	declarations.groups = append(declarations.groups, platform.PocketIDGroup{})
	declarations.groups[len(declarations.groups)-1].Name = "new-group"
	declarations.roles = append(declarations.roles, platform.VaultOIDCRole{Spec: platform.VaultOIDCRoleSpec{Group: "new-role"}})
	declarations.roles[len(declarations.roles)-1].Namespace = "security"
	vc.jwtRoles = nil
	vc.kubernetesRoles = nil
	vc.policies = nil

	report := newService(declarations, fakePocketID{}, vc, testConfig()).Report(t.Context())

	want := []Entry{
		{Kind: KindPocketIDClient, Name: "grafana", Why: "declared by a PocketIDClient resource but absent from Pocket ID"},
		{Kind: KindPocketIDClient, Name: "new-client", Why: "declared by a PocketIDClient resource but absent from Pocket ID"},
		{Kind: KindPocketIDGroup, Name: "homelab-admin", Why: "declared by a PocketIDGroup resource but absent from Pocket ID"},
		{Kind: KindPocketIDGroup, Name: "new-group", Why: "declared by a PocketIDGroup resource but absent from Pocket ID"},
		{Kind: KindVaultJWTRole, Name: "homelab-admin", Why: "declared by a VaultOIDCRole resource but absent from Vault"},
		{Kind: KindVaultJWTRole, Name: "new-role", Why: "declared by a VaultOIDCRole resource but absent from Vault"},
		{Kind: KindVaultKubernetesRole, Name: "k8sadmin", Why: "declared in k8sIssuance.roles but absent from Vault"},
	}
	for _, policy := range declaredPolicies() {
		want = append(want, Entry{Kind: KindVaultPolicy, Name: policy, Why: "written by Butler configuration but absent from Vault"})
	}
	assertEntries(t, "missing", report.Missing, want)
	if len(report.Orphaned) != 0 {
		t.Fatalf("orphaned = %+v, want none", report.Orphaned)
	}
}

func TestReportKeepsOtherSourcesWhenProvidersFail(t *testing.T) {
	declarations, pid, vc := declaredInput(testConfig())
	pid.clientsErr = errors.New("pocket id is down")
	vc.jwtErr = errors.New("vault is sealed")
	pid.groups = append(pid.groups, pocketid.UserGroup{Name: "legacy-group"})

	report := newService(declarations, pid, vc, testConfig()).Report(t.Context())

	assertSource(t, report, "pocket-id-clients", StatusUnavailable)
	assertSource(t, report, "vault-jwt-roles", StatusUnavailable)
	assertSource(t, report, "pocket-id-groups", StatusOK)
	assertSource(t, report, "vault-kubernetes-roles", StatusOK)
	assertSource(t, report, "vault-policies", StatusOK)
	if len(report.Orphaned) != 1 || report.Orphaned[0].Name != "legacy-group" {
		t.Fatalf("orphaned = %+v, want the legacy group only", report.Orphaned)
	}
}

func TestReportMarksSourcesUnavailableWhenDeclarationsFail(t *testing.T) {
	declarations, pid, vc := declaredInput(testConfig())
	declarations.clientsErr = errors.New("kubernetes api is down")

	report := newService(declarations, pid, vc, testConfig()).Report(t.Context())

	assertSource(t, report, "pocket-id-clients", StatusUnavailable)
	assertSource(t, report, "pocket-id-groups", StatusOK)
}

func TestReportIgnoresDeclarationsOutsideTheVaultNamespace(t *testing.T) {
	declarations, pid, vc := declaredInput(testConfig())
	declarations.roles = append(declarations.roles, platform.VaultOIDCRole{Spec: platform.VaultOIDCRoleSpec{Group: "other-role"}})
	declarations.roles[len(declarations.roles)-1].Namespace = "other"

	report := newService(declarations, pid, vc, testConfig()).Report(t.Context())

	if !report.Clean() {
		t.Fatalf("report not clean: %+v", report)
	}
}

func assertEntries(t *testing.T, label string, got, want []Entry) {
	t.Helper()
	sortEntries(want)
	if len(got) != len(want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %+v, want %+v", label, i, got[i], want[i])
		}
	}
}

func assertSource(t *testing.T, report Report, name, status string) {
	t.Helper()
	for _, source := range report.Sources {
		if source.Name == name {
			if source.Status != status {
				t.Fatalf("source %s = %+v, want %s", name, source, status)
			}
			if status == StatusUnavailable && source.Error == "" {
				t.Fatalf("source %s is unavailable without an error", name)
			}
			return
		}
	}
	t.Fatalf("source %s is missing from %+v", name, report.Sources)
}
