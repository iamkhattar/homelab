package certificates

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type memoryStore map[string]map[string]interface{}

func (s memoryStore) ReadSecretIfExists(_ context.Context, path string) (map[string]interface{}, error) {
	return s[path], nil
}
func (s memoryStore) WriteSecret(_ context.Context, path string, data map[string]interface{}) error {
	s[path] = data
	return nil
}

type fakeRegistrar struct {
	registration Registration
	calls        int
}

func (f *fakeRegistrar) Register(context.Context) (Registration, error) {
	f.calls++
	return f.registration, nil
}

type fakeResolver struct{ target string }

func (f fakeResolver) LookupCNAME(context.Context, string) (string, error) { return f.target, nil }

func TestManagerRegistersOnceStoresCertManagerJSONAndVerifiesCNAME(t *testing.T) {
	registration := Registration{Username: "user", Password: "password", FullDomain: "id.auth.acme-dns.io", Subdomain: "id"}
	registrar := &fakeRegistrar{registration: registration}
	store := memoryStore{}
	manager, err := NewManager(Config{APIURL: "https://auth.acme-dns.io", Domain: "6940469.xyz", CredentialPath: "infrastructure/acme-dns"}, store, registrar, fakeResolver{target: "id.auth.acme-dns.io."})
	if err != nil {
		t.Fatal(err)
	}
	status, err := manager.EnsureRegistration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.CNAMEHost != "_acme-challenge.6940469.xyz" || status.CNAMETarget != registration.FullDomain {
		t.Fatalf("unexpected status: %#v", status)
	}
	var accounts map[string]Registration
	if err := json.Unmarshal([]byte(store["infrastructure/acme-dns"][accountJSONKey].(string)), &accounts); err != nil {
		t.Fatal(err)
	}
	if accounts["6940469.xyz"].Password != "password" {
		t.Fatalf("cert-manager account JSON was not stored")
	}
	verified, err := manager.VerifyDelegation(context.Background())
	if err != nil || !verified.DelegationValid {
		t.Fatalf("VerifyDelegation() = %#v, %v", verified, err)
	}
	if _, err := manager.EnsureRegistration(context.Background()); err != nil || registrar.calls != 1 {
		t.Fatalf("registration must be idempotent; calls=%d err=%v", registrar.calls, err)
	}
}

func TestClientRejectsNonHTTPSAndIncompleteRegistration(t *testing.T) {
	if _, err := NewClient("http://auth.example.test"); err == nil {
		t.Fatal("expected non-HTTPS URL to be rejected")
	}
	client, err := NewClient("https://auth.example.test")
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"username":"only-one-field"}`)), Header: http.Header{}}, nil
	})
	if _, err := client.Register(context.Background()); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("expected incomplete registration error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }
