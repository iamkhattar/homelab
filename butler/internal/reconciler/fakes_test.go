package reconciler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/vault"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// providerCall records one provider-side mutation so tests can assert both
// that a mutation happened and that it happened in the right order.
type providerCall struct {
	Method string
	Path   string
	Body   string
}

// callLog is a concurrency-safe ordered record of provider mutations.
type callLog struct {
	mu    sync.Mutex
	calls []providerCall
}

func (l *callLog) add(method, path, body string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, providerCall{Method: method, Path: path, Body: body})
}

func (l *callLog) snapshot() []providerCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]providerCall(nil), l.calls...)
}

// mutations returns only non-GET calls, which is what "did the reconciler
// change provider state" means for these tests.
func (l *callLog) mutations() []providerCall {
	var out []providerCall
	for _, c := range l.snapshot() {
		if c.Method != http.MethodGet {
			out = append(out, c)
		}
	}
	return out
}

func (l *callLog) indexOf(method, path string) int {
	for i, c := range l.snapshot() {
		if c.Method == method && c.Path == path {
			return i
		}
	}
	return -1
}

func (l *callLog) String() string {
	var b strings.Builder
	for i, c := range l.snapshot() {
		b.WriteString("\n  ")
		b.WriteString(itoa(i))
		b.WriteString(" ")
		b.WriteString(c.Method)
		b.WriteString(" ")
		b.WriteString(c.Path)
	}
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

// vaultHandler answers one KV-v2 request. Returning a body of nil with status
// 404 models a path that does not exist.
type vaultHandler func(method, path string, body []byte) (int, []byte)

// newVaultServer serves the KV-v2 endpoints the reconcilers use and records the
// order of every mutation so create/persist/revoke ordering can be asserted.
func newVaultServer(t *testing.T, handler vaultHandler, log *callLog) *vault.Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		path := strings.TrimPrefix(r.URL.Path, "/v1/")
		path = strings.TrimPrefix(path, "secret/")
		path = strings.TrimPrefix(path, "data/")
		status, body := handler(r.Method, path, raw)
		if r.Method != http.MethodGet {
			log.add("vault:"+r.Method, path, string(raw))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(server.Close)

	client, err := vault.NewClient(server.URL)
	if err != nil {
		t.Fatalf("creating vault client: %v", err)
	}
	client.SetToken("test-token")
	return client
}

// kvSecret builds a KV-v2 read response body.
func kvSecret(data map[string]any) []byte {
	body, _ := json.Marshal(map[string]any{"data": map[string]any{"data": data, "metadata": map[string]any{"version": 1}}})
	return body
}

// newPlatformStore returns a store backed by the dynamic fake with every list
// kind the reconcilers read registered, so ownership checks do not fail on an
// unregistered kind.
func newPlatformStore(objs ...runtime.Object) *platform.Store {
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			platform.PocketIDClientGVR: "PocketIDClientList",
			platform.PocketIDGroupGVR:  "PocketIDGroupList",
			platform.CredentialGVR:     "ManagedCredentialList",
			platform.GarageBucketGVR:   "GarageBucketList",
		},
		objs...,
	)
	return platform.NewStore(client)
}

// unstructuredClient builds a PocketIDClient custom resource for the fake
// dynamic client. The typed structs deliberately do not implement
// runtime.Object, so the store is always seeded with unstructured objects.
func unstructuredClient(namespace, name, clientType string, redirectURIs []string, vaultPath string) *unstructured.Unstructured {
	uris := make([]interface{}, 0, len(redirectURIs))
	for _, uri := range redirectURIs {
		uris = append(uris, uri)
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": platform.Group + "/" + platform.Version,
		"kind":       "PocketIDClient",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace, "generation": int64(1)},
		"spec": map[string]interface{}{
			"type":         clientType,
			"redirectURIs": uris,
			"vaultPath":    vaultPath,
		},
	}}
}

// unstructuredGroup builds a cluster-scoped PocketIDGroup custom resource.
func unstructuredGroup(name, friendlyName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": platform.Group + "/" + platform.Version,
		"kind":       "PocketIDGroup",
		"metadata":   map[string]interface{}{"name": name, "generation": int64(1)},
		"spec":       map[string]interface{}{"friendlyName": friendlyName},
	}}
}

// statusByName reads back the persisted status of a decoded resource.
func statusOf(t *testing.T, store *platform.Store, kind string) map[string]platform.ResourceStatus {
	t.Helper()
	ctx := t.Context()
	out := map[string]platform.ResourceStatus{}
	switch kind {
	case "PocketIDClient":
		items, err := store.ListPocketIDClients(ctx)
		if err != nil {
			t.Fatalf("listing PocketIDClients: %v", err)
		}
		for i := range items {
			out[items[i].Namespace+"/"+items[i].Name] = items[i].Status
		}
	case "PocketIDGroup":
		items, err := store.ListPocketIDGroups(ctx)
		if err != nil {
			t.Fatalf("listing PocketIDGroups: %v", err)
		}
		for i := range items {
			out[items[i].Name] = items[i].Status
		}
	default:
		t.Fatalf("unsupported kind %q", kind)
	}
	return out
}

// conditionReason returns the Ready condition reason, or "" when absent.
func conditionReason(status platform.ResourceStatus) string {
	for _, c := range status.Conditions {
		if c.Type == "Ready" {
			return c.Reason
		}
	}
	return ""
}

func conditionMessage(status platform.ResourceStatus) string {
	for _, c := range status.Conditions {
		if c.Type == "Ready" {
			return c.Message
		}
	}
	return ""
}
