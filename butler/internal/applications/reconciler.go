package applications

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Reconciler verifies the provider-independent application contract before
// provider-specific reconcilers are allowed to consume it. Secret values are
// never part of this resource.
type Reconciler struct {
	store *Store
	k8s   kubernetes.Interface
}

func NewReconciler(store *Store, k8s kubernetes.Interface) *Reconciler {
	return &Reconciler{store: store, k8s: k8s}
}

func (r *Reconciler) Name() string { return "application-integrations" }

func (r *Reconciler) Reconcile(ctx context.Context) error {
	integrations, err := r.store.List(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, integration := range integrations {
		if _, err := r.k8s.CoreV1().Namespaces().Get(ctx, integration.Namespace, metav1.GetOptions{}); err != nil {
			failures = append(failures, fmt.Errorf("application %s namespace %s: %w", integration.Name, integration.Namespace, err))
		}
	}
	return errors.Join(failures...)
}
