package operations

import (
	"context"
	"testing"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestKubernetesStorePersistsAndRestoresJournal(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		platform.ButlerOperationGVR: "ButlerOperationList",
	})
	ctx := context.Background()
	store, err := NewKubernetesStore(ctx, client, "security", 20)
	if err != nil {
		t.Fatal(err)
	}
	store.Record("identity.user.updated", "operator@example.test", "Pocket ID user updated")
	store.Start(ctx, "reconcile", "operator@example.test", func(context.Context) error { return nil })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if operations := store.Operations(); len(operations) == 1 && operations[0].State == Succeeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	restored, err := NewKubernetesStore(ctx, client, "security", 20)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Operations(); len(got) != 1 || got[0].State != Succeeded {
		t.Fatalf("unexpected operations: %#v", got)
	}
	if got := restored.Events(); len(got) < 4 {
		t.Fatalf("expected standalone and lifecycle events, got %#v", got)
	}

	objects, err := client.Resource(platform.ButlerOperationGVR).Namespace("security").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.Items) < 5 {
		t.Fatalf("expected Kubernetes journal objects, got %d", len(objects.Items))
	}
}

func TestKubernetesStorePrunesEachJournalCategory(t *testing.T) {
	t.Parallel()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		platform.ButlerOperationGVR: "ButlerOperationList",
	})
	ctx := context.Background()
	store, err := NewKubernetesStore(ctx, client, "security", 2)
	if err != nil {
		t.Fatal(err)
	}
	store.Record("one", "operator", "first")
	time.Sleep(time.Millisecond)
	store.Record("two", "operator", "second")
	time.Sleep(time.Millisecond)
	store.Record("three", "operator", "third")

	objects, err := client.Resource(platform.ButlerOperationGVR).Namespace("security").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects.Items) != 2 {
		t.Fatalf("expected two retained events, got %d", len(objects.Items))
	}
	restored, err := NewKubernetesStore(ctx, client, "security", 2)
	if err != nil {
		t.Fatal(err)
	}
	if events := restored.Events(); len(events) != 2 || events[0].Type != "three" || events[1].Type != "two" {
		t.Fatalf("unexpected retained events: %#v", events)
	}
}

func TestTruncatePreservesUTF8(t *testing.T) {
	t.Parallel()
	if got := truncate("titan-🔐-control", 7); got != "titan-🔐" {
		t.Fatalf("truncate() = %q", got)
	}
}
