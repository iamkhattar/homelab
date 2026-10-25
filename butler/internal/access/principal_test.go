package access

import "testing"

// Production maps two tiers: butleradmin and butlerviewer. The operator tier is
// deliberately disabled, which an empty group name expresses.
var roles = Roles{Admin: "butleradmin", Viewer: "butlerviewer"}

func TestRoleMappingAndHierarchy(t *testing.T) {
	admin := roles.FromClaims("one", "", []string{"butlerviewer", "butleradmin"})
	if admin.Role != Admin || !Allows(admin.Role, Operator) {
		t.Fatalf("admin mapping = %#v", admin)
	}
	viewer := roles.FromClaims("two", "", []string{"butlerviewer"})
	if viewer.Role != Viewer || Allows(viewer.Role, Operator) {
		t.Fatalf("viewer mapping = %#v", viewer)
	}
	if !Allows(admin.Role, Viewer) || !Allows(admin.Role, Admin) {
		t.Fatalf("admin must allow every tier: %#v", admin)
	}
}

func TestFromClaimsDefaultsToNone(t *testing.T) {
	// An authenticated household identity is not a control-plane user. Only an
	// explicit Butler group grants authority.
	principal := roles.FromClaims("sub", "person@example.test", nil)
	if principal.Role != None {
		t.Fatalf("role = %q, want none", principal.Role)
	}
	if principal.Subject != "sub" || principal.Email != "person@example.test" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestFromClaimsIgnoresUnknownGroups(t *testing.T) {
	principal := roles.FromClaims("sub", "person@example.test", []string{"household", "grafana-admin"})
	if principal.Role != None {
		t.Fatalf("role = %q, want none for non-platform groups", principal.Role)
	}
}

func TestFromClaimsTakesTheHighestTier(t *testing.T) {
	principal := roles.FromClaims("sub", "", []string{"butlerviewer", "butleradmin"})
	if principal.Role != Admin {
		t.Fatalf("role = %q, want admin", principal.Role)
	}
}

func TestDisabledTierGrantsNothing(t *testing.T) {
	// An empty group name disables the tier, so the historical operator name
	// confers no authority even though the Role constant remains in the model.
	principal := roles.FromClaims("sub", "", []string{"legacy-operator", "butleroperator"})
	if principal.Role != None {
		t.Fatalf("role = %q, want none with the operator tier disabled", principal.Role)
	}
}

func TestConfiguredOperatorTierStillWorks(t *testing.T) {
	// The operator tier is configuration, not deleted code: naming a group
	// re-enables it for a deployment that wants a middle tier.
	configured := Roles{Admin: "butleradmin", Operator: "butleroperator", Viewer: "butlerviewer"}
	if got := configured.FromClaims("sub", "", []string{"butleroperator"}).Role; got != Operator {
		t.Fatalf("role = %q, want operator", got)
	}
	if got := configured.FromClaims("sub", "", []string{"butleroperator", "butlerviewer"}).Role; got != Operator {
		t.Fatalf("role = %q, want operator over viewer", got)
	}
}

func TestVaultAndKubernetesGroupsAreNotButlerRoles(t *testing.T) {
	// Vault and Kubernetes authority comes from group-bound Vault policies, not
	// from Butler. Those groups must not widen the control-plane role.
	groups := []string{"vaultadmin", "vaultread", "vaultmeta", "k8sadmin", "k8soperator", "k8sviewer"}
	if principal := roles.FromClaims("sub", "", groups); principal.Role != None {
		t.Fatalf("role = %q, want none for Vault-only groups", principal.Role)
	}
}
