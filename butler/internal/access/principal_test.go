package access

import "testing"

func TestRoleMappingAndHierarchy(t *testing.T) {
	admin := FromClaims("one", "", []string{"homelab-viewer", "homelab-admin"})
	if admin.Role != Admin || !Allows(admin.Role, Operator) {
		t.Fatalf("admin mapping = %#v", admin)
	}
	viewer := FromClaims("two", "", nil)
	if viewer.Role != Viewer || Allows(viewer.Role, Operator) {
		t.Fatalf("viewer mapping = %#v", viewer)
	}
}
