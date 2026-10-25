package reconciler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func vaultDatabaseRoleCR(namespace, name, database, rotationPeriod string) *unstructured.Unstructured {
	spec := map[string]interface{}{"database": database}
	if rotationPeriod != "" {
		spec["rotationPeriod"] = rotationPeriod
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": platform.Group + "/" + platform.Version,
		"kind":       "VaultDatabaseRole",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace, "generation": int64(1)},
		"spec":       spec,
	}}
}

func TestVaultDatabaseRolesWritesDeclaredStaticRole(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		switch {
		case method == http.MethodGet && path == "database/static-roles/example-app":
			return http.StatusNotFound, nil
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultDatabaseRoleCR("example", "example-app", "example-app", ""))
	r := NewVaultDatabaseRoles(vc, store)
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}

	mutations := log.mutations()
	if len(mutations) != 1 {
		t.Fatalf("mutations = %d, want exactly one static role write:%v", len(mutations), log)
	}
	if mutations[0].Path != "database/static-roles/example-app" {
		t.Fatalf("path = %q, want database/static-roles/example-app", mutations[0].Path)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(mutations[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	if got, _ := body["db_name"].(string); got != "postgresql" {
		t.Errorf("db_name = %q, want postgresql", got)
	}
	if got, _ := body["username"].(string); got != "example-app" {
		t.Errorf("username = %q, want the resource name", got)
	}
	// Vault's database engine takes rotation_statements as a comma-separated
	// string. A JSON array serialises into malformed SQL that PostgreSQL
	// rejects with `syntax error at or near "="`.
	wantStatements := `ALTER USER "{{name}}" WITH PASSWORD '{{password}}'`
	if got, _ := body["rotation_statements"].(string); got != wantStatements {
		t.Errorf("rotation_statements = %q, want %q", got, wantStatements)
	}
	if got, _ := body["rotation_period"].(string); got != "87600h" {
		t.Errorf("rotation_period = %q, want 87600h", got)
	}
	if _, found := body["skip_import_rotation"]; found {
		t.Error("skip_import_rotation must not be set: Vault must learn the password on import")
	}
}

func TestVaultDatabaseRolesRepairsDrift(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		switch {
		case method == http.MethodGet && path == "database/static-roles/example-app":
			// A stale period and a foreign statement must both be repaired.
			return http.StatusOK, []byte(`{"data":{"db_name":"postgresql","username":"example-app","rotation_statements":["ALTER USER \"{{name}}\" WITH PASSWORD='stale'"],"rotation_period":3600}}`)
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultDatabaseRoleCR("example", "example-app", "example-app", "24h"))
	r := NewVaultDatabaseRoles(vc, store)
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}

	mutations := log.mutations()
	if len(mutations) != 1 {
		t.Fatalf("drift was not repaired, mutations = %d:%v", len(mutations), log)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(mutations[0].Body), &body); err != nil {
		t.Fatal(err)
	}
	if got, _ := body["rotation_period"].(string); got != "24h" {
		t.Fatalf("rotation_period = %q, want 24h", got)
	}
}

func TestVaultDatabaseRolesLeavesMatchingRoleAlone(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		switch {
		case method == http.MethodGet && path == "database/static-roles/example-app":
			// Vault reports the period in seconds. A converged role must not be
			// rewritten on every reconcile pass.
			return http.StatusOK, []byte(`{"data":{"db_name":"postgresql","username":"example-app","rotation_statements":["ALTER USER \"{{name}}\" WITH PASSWORD '{{password}}'"],"rotation_period":315360000}}`)
		}
		return http.StatusOK, []byte(`{}`)
	}, log)

	store := newPlatformStore(vaultDatabaseRoleCR("example", "example-app", "example-app", "87600h"))
	r := NewVaultDatabaseRoles(vc, store)
	if err := r.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("a converged role must not be rewritten:%v", log)
	}
}

func TestVaultDatabaseRolesRejectsDuplicateNames(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(
		vaultDatabaseRoleCR("example", "shared", "example-app", ""),
		vaultDatabaseRoleCR("other", "shared", "other-app", ""),
	)
	r := NewVaultDatabaseRoles(vc, store)
	if err := r.Reconcile(t.Context()); err == nil {
		t.Fatal("one Vault role declared in two namespaces must fail instead of last-writer-wins")
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("an ambiguous role was written:%v", log)
	}
}

func TestVaultDatabaseRolesRejectsUnsafeDatabaseName(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vc := newVaultServer(t, func(method, path string, _ []byte) (int, []byte) {
		return http.StatusNotFound, nil
	}, log)

	store := newPlatformStore(vaultDatabaseRoleCR("example", "example-app", `example-app"; DROP DATABASE postgres; --`, ""))
	r := NewVaultDatabaseRoles(vc, store)
	if err := r.Reconcile(t.Context()); err == nil {
		t.Fatal("an unsafe database name must be rejected before it reaches SQL")
	}
	if mutations := log.mutations(); len(mutations) != 0 {
		t.Fatalf("an unsafe role was written:%v", log)
	}
}
