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

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/vault"
)

const (
	// databaseConnectionName is the Vault database connection Butler writes at
	// database/config/<name> for the shared PostgreSQL instance.
	databaseConnectionName = "postgresql"

	// defaultDatabaseRotationPeriod is deliberately ten years. Static roles are
	// used because consumers own their schema migrations: a dynamic role's
	// revocation would fail (or drop the schema) when the generated user owns
	// tables. With this period the password is learned once when the role is
	// imported and then stays stable. Rotation is a deliberate operator action:
	// `vault write -f database/rotate-role/<name>`.
	defaultDatabaseRotationPeriod = "87600h"
)

// databaseRoleName restricts both the Vault record name and the PostgreSQL
// role it manages. The value appears in SQL, so it must stay a conservative
// identifier rather than arbitrary text.
var databaseRoleName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// VaultDatabaseRoles converges the static database roles declared by
// VaultDatabaseRole resources into database/static-roles/<name>.
//
// A declaration only names the database (and therefore the PostgreSQL role the
// provisioning job created). Butler owns the rotation statement, so charts
// cannot inject SQL. skip_import_rotation is deliberately left at its default:
// Vault rotates once when the role is imported to learn the generated
// password, then not again until the rotation period elapses.
type VaultDatabaseRoles struct {
	vault     *vault.Client
	resources platform.Resources
}

func NewVaultDatabaseRoles(vc *vault.Client, resources platform.Resources) *VaultDatabaseRoles {
	return &VaultDatabaseRoles{vault: vc, resources: resources}
}

func (r *VaultDatabaseRoles) Name() string { return "vault-database-roles" }

func (r *VaultDatabaseRoles) Reconcile(ctx context.Context) error {
	items, err := r.resources.ListVaultDatabaseRoles(ctx)
	if err != nil {
		return fmt.Errorf("listing VaultDatabaseRoles: %w", err)
	}

	// Role names are global in Vault but resources are namespaced. Refuse an
	// ambiguous declaration before writing anything instead of letting one
	// namespace silently win over another.
	owners := make(map[string][]string, len(items))
	for i := range items {
		owners[items[i].Name] = append(owners[items[i].Name], items[i].Namespace)
	}

	var failures []error
	for i := range items {
		role := &items[i]
		if namespaces := owners[role.Name]; len(namespaces) > 1 {
			failures = append(failures, fmt.Errorf("Vault database role %q is declared in more than one namespace: %s", role.Name, strings.Join(namespaces, ", ")))
			continue
		}
		if err := r.ensureRole(ctx, role); err != nil {
			failures = append(failures, fmt.Errorf("reconciling Vault database role %q: %w", role.Name, err))
		}
	}
	return errors.Join(failures...)
}

func (r *VaultDatabaseRoles) ensureRole(ctx context.Context, role *platform.VaultDatabaseRole) error {
	if !databaseRoleName.MatchString(role.Name) {
		return fmt.Errorf("role name %q is not a valid Vault role name", role.Name)
	}
	database := strings.TrimSpace(role.Spec.Database)
	if !databaseRoleName.MatchString(database) {
		return fmt.Errorf("database %q is not a valid PostgreSQL role name", role.Spec.Database)
	}
	rotationPeriod, err := roleDuration(role.Spec.RotationPeriod, defaultDatabaseRotationPeriod)
	if err != nil {
		return fmt.Errorf("rotationPeriod: %w", err)
	}

	// Vault substitutes {{name}} and {{password}} when it learns or rotates the
	// password. The PostgreSQL role and database are created without a password
	// by the consumer's provisioning job, so this statement is the only place a
	// password is ever set.
	rotationStatements := []string{
		`ALTER USER "{{name}}" WITH PASSWORD='{{password}}'`,
	}

	desired := map[string]interface{}{
		"db_name":             databaseConnectionName,
		"username":            role.Name,
		"rotation_period":     rotationPeriod,
		"rotation_statements": rotationStatements,
	}

	path := "database/static-roles/" + role.Name
	current, err := r.vault.Raw().Logical().ReadWithContext(ctx, path)
	if err != nil && !isMissingVaultRole(err) {
		return fmt.Errorf("reading existing static role: %w", err)
	}
	if current != nil && databaseRoleMatches(desired, current.Data) {
		slog.Debug("vault database static role already matches its declaration", "role", role.Name)
		return nil
	}

	if _, err := r.vault.Raw().Logical().WriteWithContext(ctx, path, desired); err != nil {
		return fmt.Errorf("writing static role: %w", err)
	}
	slog.Info("wrote vault database static role", "role", role.Name, "database", database, "rotation_period", rotationPeriod)
	return nil
}

// databaseRoleMatches compares the fields Butler owns. Vault reports the
// rotation period in seconds, so the declared duration is converted to the
// same unit.
func databaseRoleMatches(desired map[string]interface{}, actual map[string]interface{}) bool {
	if actual == nil {
		return false
	}
	if value, _ := actual["db_name"].(string); value != desired["db_name"] {
		return false
	}
	if value, _ := actual["username"].(string); value != desired["username"] {
		return false
	}
	if !reflect.DeepEqual(vaultStringSlice(actual["rotation_statements"]), desired["rotation_statements"]) {
		return false
	}
	period, _ := time.ParseDuration(desired["rotation_period"].(string))
	return vaultSeconds(actual["rotation_period"]) == int64(period.Seconds())
}
