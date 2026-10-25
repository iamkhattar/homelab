package access

import "testing"

func TestRoleMappingAndHierarchy(t *testing.T) {
	admin := FromClaims("one", "", []string{"homelab-viewer", "homelab-admin"})
	if admin.Role != Admin || !Allows(admin.Role, Operator) {
		t.Fatalf("admin mapping = %#v", admin)
	}
	viewer := FromClaims("two", "", []string{"homelab-viewer"})
	if viewer.Role != Viewer || Allows(viewer.Role, Operator) {
		t.Fatalf("viewer mapping = %#v", viewer)
	}
	if !Allows(viewer.Role, Viewer) {
		t.Fatal("a viewer must pass a viewer check")
	}
}

func TestFromClaimsDefaultsToNone(t *testing.T) {
	// An authenticated household identity is not a control-plane user. Only an
	// explicit homelab group grants authority.
	principal := FromClaims("sub", "person@example.test", nil)
	if principal.Role != None {
		t.Fatalf("role = %q, want none", principal.Role)
	}
	if Allows(principal.Role, Viewer) {
		t.Fatal("an identity without a homelab group must not pass a viewer check")
	}
}

func TestFromClaimsIgnoresUnknownGroups(t *testing.T) {
	principal := FromClaims("sub", "person@example.test", []string{"household", "grafana-admin"})
	if principal.Role != None {
		t.Fatalf("role = %q, want none for non-platform groups", principal.Role)
	}
}

func TestFromClaimsTakesTheHighestTier(t *testing.T) {
	principal := FromClaims("sub", "", []string{"homelab-viewer", "homelab-operator", "homelab-admin"})
	if principal.Role != Admin {
		t.Fatalf("role = %q, want admin", principal.Role)
	}
}
