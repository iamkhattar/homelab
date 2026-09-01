package identity

import (
	"context"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/platform"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeSecrets map[string]interface{}

type fakeClientRegistry []platform.PocketIDClient

func (f fakeClientRegistry) ListPocketIDClients(context.Context) ([]platform.PocketIDClient, error) {
	return f, nil
}

func (f fakeSecrets) ReadSecret(context.Context, string) (map[string]interface{}, error) {
	return f, nil
}

func (f fakeSecrets) WriteSecret(context.Context, string, map[string]interface{}) error { return nil }

func TestCreateUserRejectsAdminPromotion(t *testing.T) {
	service := NewService(fakeSecrets{"api-key": "unused"}, "http://127.0.0.1", nil)
	_, err := service.CreateUser(context.Background(), pocketid.User{Username: "owner", IsAdmin: true})
	if err == nil || !strings.Contains(err.Error(), "does not create") {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateUserRequiresUsername(t *testing.T) {
	service := NewService(fakeSecrets{"api-key": "unused"}, "http://127.0.0.1", nil)
	_, err := service.CreateUser(context.Background(), pocketid.User{})
	if err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientVaultPathUsesDeclarativeResource(t *testing.T) {
	service := NewService(fakeSecrets{"api-key": "unused"}, "http://127.0.0.1", fakeClientRegistry{{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "monitoring"},
		Spec:       platform.PocketIDClientSpec{VaultPath: "custom/identity/grafana"},
		Status:     platform.ResourceStatus{ProviderID: "provider-123"},
	}})
	got, err := service.clientVaultPath(context.Background(), pocketid.OIDCClient{ID: "provider-123", Name: "grafana"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "custom/identity/grafana" {
		t.Fatalf("path = %q", got)
	}
}

func TestClientVaultPathRejectsAmbiguousDeclaration(t *testing.T) {
	registry := fakeClientRegistry{
		{ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "one"}, Spec: platform.PocketIDClientSpec{VaultPath: "oauth/one"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "two"}, Spec: platform.PocketIDClientSpec{VaultPath: "oauth/two"}},
	}
	service := NewService(fakeSecrets{"api-key": "unused"}, "http://127.0.0.1", registry)
	_, err := service.clientVaultPath(context.Background(), pocketid.OIDCClient{ID: "provider-123", Name: "grafana"})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error = %v", err)
	}
}
