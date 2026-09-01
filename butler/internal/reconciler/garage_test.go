package reconciler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/config"
	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/vault"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestGarageSkipsProviderUntilBucketIsDeclared(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	vc, err := vault.NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{platform.GarageBucketGVR: "GarageBucketList"},
	)
	reconciler := NewGarage(vc, config.GarageConfig{
		Enabled:        true,
		AdminTokenPath: "storage/garage",
		AdminTokenKey:  "admin-token",
	}, platform.NewStore(dynamicClient))
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("Garage made %d provider or Vault requests before a bucket was declared", got)
	}
}
