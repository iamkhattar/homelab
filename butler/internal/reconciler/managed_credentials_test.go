package reconciler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestManagedCredentialsWritesGeneratedSecretToVaultOnly(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vault := newVaultFixture()
	store, client := newPlatformStoreClient(credentialCR("homepage", "homepage", "applications/homepage", map[string]any{
		"auth-secret": generatedField(48),
	}))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	written, ok := vault.secrets["applications/homepage"]
	if !ok {
		t.Fatalf("expected a Vault write, got:%v", log)
	}
	value, _ := written["auth-secret"].(string)
	if value == "" {
		t.Fatal("expected a generated secret in Vault")
	}

	// The generated value must never reach the custom resource.
	object, err := client.Resource(platform.CredentialGVR).Namespace("homepage").Get(context.Background(), "homepage", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("reading back the credential: %v", err)
	}
	raw, _ := json.Marshal(object.Object)
	if string(raw) == "" || contains(string(raw), value) {
		t.Fatalf("generated credential leaked into the custom resource: %s", raw)
	}

	if reason := conditionReason(statusOf(t, store, "ManagedCredential")["homepage/homepage"]); reason != "Reconciled" {
		t.Fatalf("expected Reconciled, got %q", reason)
	}
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestManagedCredentialsDoesNotRotateAnExistingValue(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vault := newVaultFixture()
	// The credential already exists and a consumer is using it. The reconciler
	// must extend the path, never rotate a value that is already published.
	vault.secrets["applications/homepage"] = map[string]any{"auth-secret": "already-in-use"}

	store := newPlatformStore(credentialCR("homepage", "homepage", "applications/homepage", map[string]any{
		"auth-secret": generatedField(48),
		"extra-field": generatedField(16),
	}))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	written := vault.secrets["applications/homepage"]
	if got, _ := written["auth-secret"].(string); got != "already-in-use" {
		t.Fatalf("an existing credential must not be rotated, got %q", got)
	}
	extra, _ := written["extra-field"].(string)
	if extra == "" {
		t.Fatal("the new field must be generated")
	}
	if extra == "already-in-use" {
		t.Fatal("generated fields must not collide with existing values")
	}
}

func TestManagedCredentialsIsIdempotentForAConvergedPath(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vault := newVaultFixture()
	vault.secrets["applications/homepage"] = map[string]any{"auth-secret": "static-value"}

	store := newPlatformStore(credentialCR("homepage", "homepage", "applications/homepage", map[string]any{
		"auth-secret": map[string]any{"value": "static-value"},
	}))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("a converged path must not be rewritten, got:%v", log)
	}
}

func TestManagedCredentialsProjectsSourceRefFromAnotherPath(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vault := newVaultFixture()
	vault.secrets["databases/postgresql"] = map[string]any{"password": "db-password"}

	store := newPlatformStore(credentialCR("kitchenowl", "kitchenowl", "applications/kitchenowl", map[string]any{
		"database-url": map[string]any{"template": "postgres://app:{{password}}@postgresql:5432/app"},
		"password": map[string]any{"sourceRef": map[string]any{
			"path": "databases/postgresql", "key": "password",
		}},
	}))
	vc := newVaultServer(t, vault.handler, log)

	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	written := vault.secrets["applications/kitchenowl"]
	if got, _ := written["password"].(string); got != "db-password" {
		t.Fatalf("expected the source value to be projected, got %q", got)
	}
	url, _ := written["database-url"].(string)
	if url != "postgres://app:db-password@postgresql:5432/app" {
		t.Fatalf("expected the template to resolve against the projected value, got %q", url)
	}
}

func TestManagedCredentialsFailsWhenSourceKeyIsMissing(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vault := newVaultFixture()
	vault.secrets["databases/postgresql"] = map[string]any{"other": "value"}

	store := newPlatformStore(credentialCR("kitchenowl", "kitchenowl", "applications/kitchenowl", map[string]any{
		"password": map[string]any{"sourceRef": map[string]any{"path": "databases/postgresql", "key": "password"}},
	}))
	vc := newVaultServer(t, vault.handler, log)

	err := NewManagedCredentials(vc, store).Reconcile(context.Background())
	if err == nil {
		t.Fatal("expected a missing source key to fail the run")
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("an unresolved reference must not write to Vault, got:%v", log)
	}
	if reason := conditionReason(statusOf(t, store, "ManagedCredential")["kitchenowl/kitchenowl"]); reason != "ReconcileFailed" {
		t.Fatalf("expected ReconcileFailed, got %q", reason)
	}
}

func TestManagedCredentialsRejectsInvalidFieldDefinitions(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]any{
		"no value source":      {"auth-secret": map[string]any{}},
		"two value sources":    {"auth-secret": map[string]any{"value": "x", "generate": map[string]any{"type": "password", "length": int64(8)}}},
		"unknown generator":    {"auth-secret": map[string]any{"generate": map[string]any{"type": "rot13", "length": int64(8)}}},
		"zero length":          {"auth-secret": map[string]any{"generate": map[string]any{"type": "password", "length": int64(0)}}},
		"incomplete sourceRef": {"auth-secret": map[string]any{"sourceRef": map[string]any{"path": "a/b"}}},
	}
	for name, fields := range cases {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			vault := newVaultFixture()
			store := newPlatformStore(credentialCR("homepage", "homepage", "applications/homepage", fields))
			vc := newVaultServer(t, vault.handler, log)

			if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err == nil {
				t.Fatal("expected an invalid field definition to fail the run")
			}
			if got := log.mutations(); len(got) != 0 {
				t.Fatalf("an invalid field definition must not write to Vault, got:%v", log)
			}
		})
	}
}

func TestManagedCredentialsRejectsDuplicateVaultPath(t *testing.T) {
	t.Parallel()
	log := &callLog{}
	vault := newVaultFixture()
	store := newPlatformStore(
		credentialCR("homepage", "homepage", "applications/shared", map[string]any{"a": generatedField(8)}),
		credentialCR("vaultwarden", "vaultwarden", "applications/shared", map[string]any{"b": generatedField(8)}),
	)
	vc := newVaultServer(t, vault.handler, log)

	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err == nil {
		t.Fatal("expected duplicate Vault paths to fail the run")
	}
	for key, status := range statusOf(t, store, "ManagedCredential") {
		if conditionReason(status) != "DuplicateVaultPath" {
			t.Fatalf("%s: expected DuplicateVaultPath, got %q", key, conditionReason(status))
		}
	}
	if got := log.mutations(); len(got) != 0 {
		t.Fatalf("a rejected resource must not write to Vault, got:%v", log)
	}
}

func TestManagedCredentialsRequiresPathAndFields(t *testing.T) {
	t.Parallel()
	for name, object := range map[string]struct {
		vaultPath string
		fields    map[string]any
	}{
		"no path":   {"", map[string]any{"a": generatedField(8)}},
		"no fields": {"applications/homepage", map[string]any{}},
	} {
		t.Run(name, func(t *testing.T) {
			log := &callLog{}
			vault := newVaultFixture()
			store := newPlatformStore(credentialCR("homepage", "homepage", object.vaultPath, object.fields))
			vc := newVaultServer(t, vault.handler, log)

			if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err == nil {
				t.Fatal("expected an incomplete spec to fail the run")
			}
			if got := log.mutations(); len(got) != 0 {
				t.Fatalf("an incomplete spec must not write to Vault, got:%v", log)
			}
		})
	}
}

func TestManagedCredentialsPrunesUndeclaredFieldsOnlyWhenDeclared(t *testing.T) {
	t.Parallel()
	vault := newVaultFixture()
	// The legacy shared document contained every application password and an
	// init script. Prune must delete those while preserving the admin value.
	vault.secrets["databases/postgresql"] = map[string]any{
		"postgres-password": "keep-this",
		"legacy-password":   "remove-this",
		"init.sql":          "remove-this-too",
	}
	credential := credentialCR("databases", "postgresql", "databases/postgresql", map[string]any{
		"postgres-password": generatedField(32),
	})
	credential.Object["spec"].(map[string]interface{})["prune"] = true

	store := newPlatformStore(credential)
	vc := newVaultServer(t, vault.handler, &callLog{})
	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	written := vault.secrets["databases/postgresql"]
	if got, _ := written["postgres-password"].(string); got != "keep-this" {
		t.Fatalf("declared value was rotated: %q", got)
	}
	for _, key := range []string{"legacy-password", "init.sql"} {
		if _, ok := written[key]; ok {
			t.Errorf("undeclared key %q survived a pruning reconcile", key)
		}
	}

	// Without prune, the same declaration preserves the old keys.
	vault.secrets["databases/postgresql"] = map[string]any{"postgres-password": "keep-this", "legacy-password": "still-here"}
	store = newPlatformStore(credentialCR("databases", "postgresql", "databases/postgresql", map[string]any{
		"postgres-password": generatedField(32),
	}))
	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if _, ok := vault.secrets["databases/postgresql"]["legacy-password"]; !ok {
		t.Fatal("an undeclared key was pruned without prune: true")
	}
}

func TestManagedCredentialsNeverPersistsValuesIntoStatus(t *testing.T) {
	t.Parallel()
	vault := newVaultFixture()
	vault.secrets["applications/homepage"] = map[string]any{"auth-secret": "super-secret-value"}
	store, client := newPlatformStoreClient(credentialCR("homepage", "homepage", "applications/homepage", map[string]any{
		"added-field": generatedField(12),
	}))
	vc := newVaultServer(t, vault.handler, &callLog{})
	if err := NewManagedCredentials(vc, store).Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	object, err := client.Resource(platform.CredentialGVR).Namespace("homepage").Get(context.Background(), "homepage", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("reading back the credential: %v", err)
	}
	raw, _ := json.Marshal(object.Object)
	added, _ := vault.secrets["applications/homepage"]["added-field"].(string)
	for _, secret := range []string{"super-secret-value", added} {
		if contains(string(raw), secret) {
			t.Fatalf("credential material was persisted to Kubernetes: %s", raw)
		}
	}
}
