package applications

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPutAndListIntegration(t *testing.T) {
	store := NewStore(fake.NewSimpleClientset(), "security")
	want := Integration{Name: "paperless", Namespace: "paperless", Authentication: "forward-auth", Owner: "homelab-admin"}
	if err := store.Put(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != want.Name {
		t.Fatalf("items = %#v", items)
	}
}

func TestReconcilerValidatesReferencedNamespaces(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "paperless"}})
	store := NewStore(client, "security")
	if err := store.Put(context.Background(), Integration{Name: "paperless", Namespace: "paperless", Authentication: "forward-auth", Owner: "homelab-admin"}); err != nil {
		t.Fatal(err)
	}
	if err := NewReconciler(store, client).Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPutRejectsUnsafeInput(t *testing.T) {
	store := NewStore(fake.NewSimpleClientset(), "security")
	err := store.Put(context.Background(), Integration{Name: "Bad", Namespace: "default", Authentication: "none", Owner: "owner"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
