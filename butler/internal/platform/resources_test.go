package platform

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestConvergeStatusPreservesTransitionAndSkipsNoop(t *testing.T) {
	t.Parallel()
	transition := metav1.NewTime(time.Unix(100, 0).UTC())
	current := Ready(1, "provider-1")
	current.Conditions[0].LastTransitionTime = transition

	next, changed := ConvergeStatus(current, Ready(2, "provider-1"))
	if !changed {
		t.Fatal("generation change must update status")
	}
	if !next.Conditions[0].LastTransitionTime.Equal(&transition) {
		t.Fatal("non-transition changed lastTransitionTime")
	}
	if _, changed := ConvergeStatus(next, next); changed {
		t.Fatal("identical status should not be rewritten")
	}
}

func TestStoreListsNamespacedResourcesAndUpdatesStatus(t *testing.T) {
	t.Parallel()
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": Group + "/" + Version,
		"kind":       "ManagedCredential",
		"metadata":   map[string]interface{}{"name": "homepage", "namespace": "homepage", "generation": int64(3)},
		"spec": map[string]interface{}{
			"vaultPath": "applications/homepage",
			"fields":    map[string]interface{}{"auth-secret": map[string]interface{}{"generate": map[string]interface{}{"type": "password", "length": int64(48)}}},
		},
	}}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{CredentialGVR: "ManagedCredentialList"}, object)
	store := NewStore(client)
	items, err := store.ListManagedCredentials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Spec.VaultPath != "applications/homepage" {
		t.Fatalf("unexpected resources: %#v", items)
	}
	items[0].Status = Ready(items[0].Generation, "")
	if err := store.UpdateManagedCredentialStatus(context.Background(), &items[0]); err != nil {
		t.Fatal(err)
	}
	updated, err := client.Resource(CredentialGVR).Namespace("homepage").Get(context.Background(), "homepage", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	conditions, found, err := unstructured.NestedSlice(updated.Object, "status", "conditions")
	if err != nil || !found || len(conditions) != 1 {
		t.Fatalf("status was not persisted: %#v, %v", updated.Object["status"], err)
	}
	condition, ok := conditions[0].(map[string]interface{})
	if !ok || condition["status"] != "True" {
		t.Fatalf("unexpected Ready condition: %#v", conditions[0])
	}
}

func TestVaultPathOwnersCountsAcrossResourceKinds(t *testing.T) {
	t.Parallel()
	objects := []runtime.Object{
		platformObject("ManagedCredential", "app", "one", map[string]interface{}{"vaultPath": "shared/path", "fields": map[string]interface{}{"value": map[string]interface{}{"value": "safe"}}}),
		platformObject("PocketIDClient", "app", "two", map[string]interface{}{"type": "confidential", "redirectURIs": []interface{}{"https://app.example/callback"}, "vaultPath": "shared/path"}),
		platformObject("GarageBucket", "storage", "three", map[string]interface{}{"bucketName": "backups", "credentialPath": "garage/backups", "permissions": map[string]interface{}{"read": true}}),
	}
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		CredentialGVR: "ManagedCredentialList", PocketIDClientGVR: "PocketIDClientList", GarageBucketGVR: "GarageBucketList",
	}, objects...)
	owners, err := VaultPathOwners(context.Background(), NewStore(client))
	if err != nil {
		t.Fatal(err)
	}
	if owners["shared/path"] != 2 || owners["garage/backups"] != 1 {
		t.Fatalf("unexpected owners: %#v", owners)
	}
}

func TestChangeHandlerTriggersOnlyForDesiredStateChanges(t *testing.T) {
	t.Parallel()
	events := make(chan struct{}, 1)
	handler := newChangeHandler(events)
	object := platformObject("ManagedCredential", "security", "pocket-id", map[string]interface{}{
		"vaultPath": "security/pocket-id",
		"fields":    map[string]interface{}{},
	})
	object.SetGeneration(1)

	handler.OnAdd(object, false)
	expectChangeSignal(t, events)

	statusOnly := object.DeepCopy()
	statusOnly.SetResourceVersion("2")
	handler.OnUpdate(object, statusOnly)
	expectNoChangeSignal(t, events)

	specChange := statusOnly.DeepCopy()
	specChange.SetGeneration(2)
	handler.OnUpdate(statusOnly, specChange)
	expectChangeSignal(t, events)

	handler.OnDelete(specChange)
	expectChangeSignal(t, events)
}

func TestChangeHandlerCoalescesBursts(t *testing.T) {
	t.Parallel()
	events := make(chan struct{}, 1)
	handler := newChangeHandler(events)
	object := platformObject("PocketIDGroup", "", "homelab-admin", map[string]interface{}{"friendlyName": "Administrators"})
	handler.OnAdd(object, false)
	handler.OnAdd(object, false)
	if got := len(events); got != 1 {
		t.Fatalf("queued signals = %d, want one coalesced signal", got)
	}
}

func expectChangeSignal(t *testing.T, events <-chan struct{}) {
	t.Helper()
	select {
	case <-events:
	default:
		t.Fatal("expected desired-state change signal")
	}
}

func expectNoChangeSignal(t *testing.T, events <-chan struct{}) {
	t.Helper()
	select {
	case <-events:
		t.Fatal("status-only update triggered reconciliation")
	default:
	}
}

func platformObject(kind, namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": Group + "/" + Version,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec":       spec,
	}}
}
