// Package platform defines the Kubernetes resources reconciled by Butler.
package platform

import (
	"context"
	"fmt"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
)

const (
	Group   = "platform.6940469.xyz"
	Version = "v1alpha1"
)

var (
	PocketIDClientGVR  = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "pocketidclients"}
	PocketIDGroupGVR   = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "pocketidgroups"}
	CredentialGVR      = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "managedcredentials"}
	GarageBucketGVR    = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "garagebuckets"}
	ButlerOperationGVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "butleroperations"}
)

type ResourceStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	ProviderID         string             `json:"providerID,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type PocketIDClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PocketIDClientSpec `json:"spec"`
	Status            ResourceStatus     `json:"status,omitempty"`
}

type PocketIDClientSpec struct {
	Type         string   `json:"type"`
	RedirectURIs []string `json:"redirectURIs"`
	VaultPath    string   `json:"vaultPath"`
}

type PocketIDGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PocketIDGroupSpec `json:"spec"`
	Status            ResourceStatus    `json:"status,omitempty"`
}

type PocketIDGroupSpec struct {
	FriendlyName string `json:"friendlyName"`
}

type ManagedCredential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ManagedCredentialSpec `json:"spec"`
	Status            ResourceStatus        `json:"status,omitempty"`
}

type ManagedCredentialSpec struct {
	VaultPath string                     `json:"vaultPath"`
	Fields    map[string]CredentialField `json:"fields"`
}

type CredentialField struct {
	Generate  *GeneratorSpec `json:"generate,omitempty"`
	Value     *string        `json:"value,omitempty"`
	Template  string         `json:"template,omitempty"`
	SourceRef *SecretKeyRef  `json:"sourceRef,omitempty"`
}

type GeneratorSpec struct {
	Type   string `json:"type"`
	Length int    `json:"length"`
}

type SecretKeyRef struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

type GarageBucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              GarageBucketSpec `json:"spec"`
	Status            ResourceStatus   `json:"status,omitempty"`
}

type GarageBucketSpec struct {
	BucketName     string            `json:"bucketName"`
	CredentialPath string            `json:"credentialPath"`
	Permissions    BucketPermissions `json:"permissions"`
}

type BucketPermissions struct {
	Read  bool `json:"read"`
	Write bool `json:"write"`
	Owner bool `json:"owner"`
}

type Resources interface {
	ListPocketIDClients(context.Context) ([]PocketIDClient, error)
	UpdatePocketIDClientStatus(context.Context, *PocketIDClient) error
	ListPocketIDGroups(context.Context) ([]PocketIDGroup, error)
	UpdatePocketIDGroupStatus(context.Context, *PocketIDGroup) error
	ListManagedCredentials(context.Context) ([]ManagedCredential, error)
	UpdateManagedCredentialStatus(context.Context, *ManagedCredential) error
	ListGarageBuckets(context.Context) ([]GarageBucket, error)
	UpdateGarageBucketStatus(context.Context, *GarageBucket) error
}

type Store struct{ dynamic dynamic.Interface }

func NewStore(client dynamic.Interface) *Store { return &Store{dynamic: client} }

// WatchChanges returns a coalescing signal for desired-state changes to
// Butler-owned platform resources. Status-only updates preserve metadata.generation
// and are deliberately ignored so Butler does not trigger itself after writing
// a Ready condition. Add and delete events always trigger because they can
// introduce or resolve cross-resource ownership conflicts.
func (s *Store) WatchChanges(ctx context.Context) (<-chan struct{}, error) {
	events := make(chan struct{}, 1)
	factory := dynamicinformer.NewDynamicSharedInformerFactory(s.dynamic, 0)
	for _, gvr := range []schema.GroupVersionResource{
		PocketIDClientGVR,
		PocketIDGroupGVR,
		CredentialGVR,
		GarageBucketGVR,
	} {
		informer := factory.ForResource(gvr)
		if _, err := informer.Informer().AddEventHandler(newChangeHandler(events)); err != nil {
			return nil, fmt.Errorf("registering informer for %s: %w", gvr.Resource, err)
		}
	}
	factory.Start(ctx.Done())
	return events, nil
}

func newChangeHandler(events chan<- struct{}) cache.ResourceEventHandler {
	signal := func() {
		select {
		case events <- struct{}{}:
		default:
		}
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(any) { signal() },
		UpdateFunc: func(oldObject, newObject any) {
			oldMeta, oldOK := oldObject.(metav1.Object)
			newMeta, newOK := newObject.(metav1.Object)
			if !oldOK || !newOK || oldMeta.GetGeneration() != newMeta.GetGeneration() {
				signal()
			}
		},
		DeleteFunc: func(any) { signal() },
	}
}

// VaultPathOwners returns the number of declarative resources targeting each
// output path. A path may have many readers but exactly one writer.
func VaultPathOwners(ctx context.Context, resources Resources) (map[string]int, error) {
	owners := make(map[string]int)
	credentials, err := resources.ListManagedCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing ManagedCredentials for Vault ownership: %w", err)
	}
	for i := range credentials {
		owners[credentials[i].Spec.VaultPath]++
	}
	clients, err := resources.ListPocketIDClients(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing PocketIDClients for Vault ownership: %w", err)
	}
	for i := range clients {
		owners[clients[i].Spec.VaultPath]++
	}
	buckets, err := resources.ListGarageBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing GarageBuckets for Vault ownership: %w", err)
	}
	for i := range buckets {
		owners[buckets[i].Spec.CredentialPath]++
	}
	return owners, nil
}

func (s *Store) ListPocketIDClients(ctx context.Context) ([]PocketIDClient, error) {
	return listNamespaced[PocketIDClient](ctx, s.dynamic, PocketIDClientGVR)
}
func (s *Store) UpdatePocketIDClientStatus(ctx context.Context, item *PocketIDClient) error {
	return updateNamespacedStatus(ctx, s.dynamic, PocketIDClientGVR, item.Namespace, item)
}
func (s *Store) ListPocketIDGroups(ctx context.Context) ([]PocketIDGroup, error) {
	return listCluster[PocketIDGroup](ctx, s.dynamic, PocketIDGroupGVR)
}
func (s *Store) UpdatePocketIDGroupStatus(ctx context.Context, item *PocketIDGroup) error {
	return updateClusterStatus(ctx, s.dynamic, PocketIDGroupGVR, item)
}
func (s *Store) ListManagedCredentials(ctx context.Context) ([]ManagedCredential, error) {
	return listNamespaced[ManagedCredential](ctx, s.dynamic, CredentialGVR)
}
func (s *Store) UpdateManagedCredentialStatus(ctx context.Context, item *ManagedCredential) error {
	return updateNamespacedStatus(ctx, s.dynamic, CredentialGVR, item.Namespace, item)
}
func (s *Store) ListGarageBuckets(ctx context.Context) ([]GarageBucket, error) {
	return listNamespaced[GarageBucket](ctx, s.dynamic, GarageBucketGVR)
}
func (s *Store) UpdateGarageBucketStatus(ctx context.Context, item *GarageBucket) error {
	return updateNamespacedStatus(ctx, s.dynamic, GarageBucketGVR, item.Namespace, item)
}

func Ready(generation int64, providerID string) ResourceStatus {
	return ResourceStatus{ObservedGeneration: generation, ProviderID: providerID, Conditions: []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled",
		Message:            "Provider state and Vault material match the declared resource",
		ObservedGeneration: generation, LastTransitionTime: metav1.Now(),
	}}}
}

func Failed(generation int64, reason string, err error) ResourceStatus {
	// Provider errors may contain response bodies, tokens, or generated values.
	// Keep detailed errors in process-local diagnostics, never in Kubernetes.
	_ = err
	return ResourceStatus{ObservedGeneration: generation, Conditions: []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionFalse, Reason: reason,
		Message:            "Reconciliation failed; inspect Butler logs for details",
		ObservedGeneration: generation, LastTransitionTime: metav1.Now(),
	}}}
}

// ConvergeStatus preserves transition timestamps when only the observed
// generation or provider metadata changes, and reports whether an API write is
// required. This avoids rewriting every custom resource on each scheduler run.
func ConvergeStatus(current, desired ResourceStatus) (ResourceStatus, bool) {
	if len(current.Conditions) == 1 && len(desired.Conditions) == 1 {
		have, want := current.Conditions[0], desired.Conditions[0]
		if have.Type == want.Type && have.Status == want.Status && have.Reason == want.Reason && have.Message == want.Message {
			desired.Conditions[0].LastTransitionTime = have.LastTransitionTime
		}
	}
	return desired, !reflect.DeepEqual(current, desired)
}

func listNamespaced[T any](ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource) ([]T, error) {
	list, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return convertList[T](list)
}

func listCluster[T any](ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource) ([]T, error) {
	list, err := client.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return convertList[T](list)
}

func convertList[T any](list *unstructured.UnstructuredList) ([]T, error) {
	out := make([]T, 0, len(list.Items))
	for i := range list.Items {
		var item T
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[i].Object, &item); err != nil {
			return nil, fmt.Errorf("decoding %s/%s: %w", list.Items[i].GetNamespace(), list.Items[i].GetName(), err)
		}
		out = append(out, item)
	}
	return out, nil
}

func updateNamespacedStatus(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, namespace string, item any) error {
	return updateStatus(ctx, client.Resource(gvr).Namespace(namespace), item)
}

func updateClusterStatus(ctx context.Context, client dynamic.Interface, gvr schema.GroupVersionResource, item any) error {
	return updateStatus(ctx, client.Resource(gvr), item)
}

func updateStatus(ctx context.Context, resource dynamic.ResourceInterface, item any) error {
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(item)
	if err != nil {
		return err
	}
	desired := &unstructured.Unstructured{Object: object}
	status, found, err := unstructured.NestedMap(desired.Object, "status")
	if err != nil {
		return fmt.Errorf("reading desired status: %w", err)
	}
	if !found {
		status = map[string]interface{}{}
	}

	// A reconciliation starts from a list snapshot. Refetch before every status
	// write and retry conflicts so a concurrent spec edit is never overwritten.
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, getErr := resource.Get(ctx, desired.GetName(), metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		if setErr := unstructured.SetNestedMap(current.Object, runtime.DeepCopyJSONValue(status).(map[string]interface{}), "status"); setErr != nil {
			return setErr
		}
		_, updateErr := resource.UpdateStatus(ctx, current, metav1.UpdateOptions{})
		return updateErr
	})
}
