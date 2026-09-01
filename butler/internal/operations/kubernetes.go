package operations

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
)

type kubernetesBackend struct {
	client dynamic.ResourceInterface
	limit  int
}

// NewKubernetesStore restores the bounded audit-safe journal from Kubernetes.
// The CRs contain metadata only; credentials and request bodies are forbidden.
func NewKubernetesStore(ctx context.Context, client dynamic.Interface, namespace string, limit int) (*Store, error) {
	store := NewStore(limit)
	backend := &kubernetesBackend{client: client.Resource(platform.ButlerOperationGVR).Namespace(namespace), limit: store.limit}
	list, err := backend.client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing ButlerOperations: %w", err)
	}
	for i := range list.Items {
		category, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "category")
		if category == "operation" {
			if operation, ok := decodeOperation(list.Items[i]); ok {
				store.operations = append(store.operations, operation)
			}
		} else if category == "event" {
			if event, ok := decodeEvent(list.Items[i]); ok {
				store.events = append(store.events, event)
			}
		}
	}
	sort.Slice(store.operations, func(i, j int) bool { return store.operations[i].CreatedAt.After(store.operations[j].CreatedAt) })
	sort.Slice(store.events, func(i, j int) bool { return store.events[i].CreatedAt.After(store.events[j].CreatedAt) })
	if len(store.operations) > store.limit {
		store.operations = store.operations[:store.limit]
	}
	if len(store.events) > store.limit {
		store.events = store.events[:store.limit]
	}
	store.backend = backend
	return store, nil
}

func (b *kubernetesBackend) SaveOperation(ctx context.Context, operation Operation) error {
	spec := map[string]interface{}{"category": "operation", "kind": truncate(operation.Kind, 128), "actor": truncate(operation.Actor, 320), "state": string(operation.State), "createdAt": operation.CreatedAt.Format(time.RFC3339Nano)}
	if !operation.CompletedAt.IsZero() {
		spec["completedAt"] = operation.CompletedAt.Format(time.RFC3339Nano)
	}
	if operation.Error != "" {
		spec["error"] = truncate(operation.Error, 4096)
	}
	return b.save(ctx, "operation-"+operation.ID, spec)
}

func (b *kubernetesBackend) SaveEvent(ctx context.Context, event Event) error {
	spec := map[string]interface{}{"category": "event", "eventType": truncate(event.Type, 128), "actor": truncate(event.Actor, 320), "message": truncate(event.Message, 4096), "createdAt": event.CreatedAt.Format(time.RFC3339Nano)}
	if event.Operation != "" {
		spec["operation"] = event.Operation
	}
	return b.save(ctx, "event-"+event.ID, spec)
}

func (b *kubernetesBackend) save(ctx context.Context, name string, spec map[string]interface{}) error {
	err := retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
	}, func() error {
		current, getErr := b.client.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			_, createErr := b.client.Create(ctx, &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": platform.Group + "/" + platform.Version, "kind": "ButlerOperation", "metadata": map[string]interface{}{"name": name}, "spec": spec}}, metav1.CreateOptions{})
			return createErr
		}
		if getErr != nil {
			return getErr
		}
		current.Object["spec"] = spec
		_, updateErr := b.client.Update(ctx, current, metav1.UpdateOptions{})
		return updateErr
	})
	if err != nil {
		return err
	}
	return b.prune(ctx, stringValue(spec, "category"))
}

func (b *kubernetesBackend) prune(ctx context.Context, category string) error {
	list, err := b.client.List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	items := make([]unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		value, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "category")
		if value == category {
			items = append(items, list.Items[i])
		}
	}
	sort.Slice(items, func(i, j int) bool {
		a, _, _ := unstructured.NestedString(items[i].Object, "spec", "createdAt")
		other, _, _ := unstructured.NestedString(items[j].Object, "spec", "createdAt")
		return a > other
	})
	for i := b.limit; i < len(items); i++ {
		if err := b.client.Delete(ctx, items[i].GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func decodeOperation(item unstructured.Unstructured) (Operation, bool) {
	spec, _, _ := unstructured.NestedMap(item.Object, "spec")
	created, err := time.Parse(time.RFC3339Nano, stringValue(spec, "createdAt"))
	if err != nil {
		return Operation{}, false
	}
	completed, _ := time.Parse(time.RFC3339Nano, stringValue(spec, "completedAt"))
	return Operation{ID: trimPrefix(item.GetName(), "operation-"), Kind: stringValue(spec, "kind"), Actor: stringValue(spec, "actor"), State: State(stringValue(spec, "state")), CreatedAt: created, CompletedAt: completed, Error: stringValue(spec, "error")}, true
}

func decodeEvent(item unstructured.Unstructured) (Event, bool) {
	spec, _, _ := unstructured.NestedMap(item.Object, "spec")
	created, err := time.Parse(time.RFC3339Nano, stringValue(spec, "createdAt"))
	if err != nil {
		return Event{}, false
	}
	return Event{ID: trimPrefix(item.GetName(), "event-"), Operation: stringValue(spec, "operation"), Type: stringValue(spec, "eventType"), Actor: stringValue(spec, "actor"), Message: stringValue(spec, "message"), CreatedAt: created}, true
}

func stringValue(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return value
}
func trimPrefix(value, prefix string) string {
	if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
		return value[len(prefix):]
	}
	return value
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
