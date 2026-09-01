package recovery

import (
	"context"
	"strings"
	"testing"

	"github.com/iamkhattar/homelab/butler/internal/certificates"
	"github.com/iamkhattar/homelab/butler/internal/pocketid"
	"github.com/iamkhattar/homelab/butler/internal/vault"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type recoveryVault struct {
	status  vault.LifecycleStatus
	secrets map[string]map[string]interface{}
}

func (v *recoveryVault) Status(context.Context) (vault.LifecycleStatus, error) { return v.status, nil }
func (v *recoveryVault) ReadSecretIfExists(_ context.Context, path string) (map[string]interface{}, error) {
	data := map[string]interface{}{}
	for key, value := range v.secrets[path] {
		data[key] = value
	}
	return data, nil
}
func (v *recoveryVault) WriteSecret(_ context.Context, path string, data map[string]interface{}) error {
	copy := map[string]interface{}{}
	for key, value := range data {
		copy[key] = value
	}
	v.secrets[path] = copy
	return nil
}

type bootstrapFunc func(context.Context) error

func (f bootstrapFunc) Reconcile(ctx context.Context) error { return f(ctx) }

type certificateStore map[string]map[string]interface{}

func (s certificateStore) ReadSecretIfExists(_ context.Context, path string) (map[string]interface{}, error) {
	return s[path], nil
}
func (s certificateStore) WriteSecret(_ context.Context, path string, data map[string]interface{}) error {
	s[path] = data
	return nil
}

type certificateRegistrar struct{ registration certificates.Registration }

func (r certificateRegistrar) Register(context.Context) (certificates.Registration, error) {
	return r.registration, nil
}

type certificateResolver struct{ target string }

func (r certificateResolver) LookupCNAME(context.Context, string) (string, error) {
	return r.target, nil
}

func TestAdvanceRequiresExplicitConfirmation(t *testing.T) {
	service := &Service{}
	if err := service.Advance(context.Background(), false); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("error = %v", err)
	}
}

func TestConfirmIdentityRequiresBothRealLoginPolicySets(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: "security"},
		Data:       map[string]string{"phase": "awaiting-identity-verification"},
	})
	service := &Service{k8s: client, namespace: "security"}
	if err := service.ConfirmIdentity(t.Context(), IdentityEvidence{PocketIDSubject: "owner", ButlerRole: "admin", VaultPolicies: []string{"vault-admin"}}); err == nil {
		t.Fatal("accepted Vault login without Kubernetes administrator policy")
	}
	evidence := IdentityEvidence{PocketIDSubject: "owner", ButlerRole: "admin", VaultPolicies: []string{"default", "vault-admin", "k8s-admin"}}
	if err := service.ConfirmIdentity(t.Context(), evidence); err != nil {
		t.Fatal(err)
	}
	state, err := client.CoreV1().ConfigMaps("security").Get(t.Context(), stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Data["phase"] != "operational" || state.Data["butlerLoginVerified"] != "true" || state.Data["vaultLoginVerified"] != "true" {
		t.Fatalf("state = %#v", state.Data)
	}
}

func TestPocketIDImportRejectsEmptyCredential(t *testing.T) {
	service := &Service{}
	if err := service.ImportPocketIDAPIKey(context.Background(), " "); err == nil {
		t.Fatal("expected empty key rejection")
	}
}

func TestPocketIDImportPreservesRuntimeCredentialFields(t *testing.T) {
	store := &recoveryVault{secrets: map[string]map[string]interface{}{
		pocketid.ManagementCredentialVaultPath: {"encryption-key": "keep-me"},
	}}
	service := &Service{vault: store}
	if err := service.ImportPocketIDAPIKey(t.Context(), " replacement-key "); err != nil {
		t.Fatal(err)
	}
	got := store.secrets[pocketid.ManagementCredentialVaultPath]
	if got["encryption-key"] != "keep-me" || got[pocketid.ManagementCredentialField] != "replacement-key" {
		t.Fatalf("credential = %#v", got)
	}
}

func TestAdvanceGeneratesPocketIDMachineCredentialBeforeIdentity(t *testing.T) {
	store := &recoveryVault{
		status:  vault.LifecycleStatus{Initialized: true, Sealed: false},
		secrets: map[string]map[string]interface{}{},
	}
	k8s := fake.NewSimpleClientset(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: initSecretName, Namespace: "security"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "homelab-wildcard-tls", Namespace: "networking"}, Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: []byte("cert"), corev1.TLSPrivateKeyKey: []byte("key")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: "security"}, Data: map[string]string{"phase": "awaiting-pocket-id-credential", "dnsDelegationVerified": "true"}},
	)
	registration := certificates.Registration{Username: "user", Password: "secret", FullDomain: "generated.auth.acme-dns.io", Subdomain: "generated"}
	manager, err := certificates.NewManager(certificates.Config{APIURL: "https://auth.acme-dns.io", Domain: "6940469.xyz", CredentialPath: "infrastructure/acme-dns", CertificateNS: "networking", TLSSecretName: "homelab-wildcard-tls"}, certificateStore{}, certificateRegistrar{registration}, certificateResolver{registration.FullDomain + "."})
	if err != nil {
		t.Fatal(err)
	}
	var foundationRuns, identityRuns int
	foundation := bootstrapFunc(func(context.Context) error { foundationRuns++; return nil })
	credential := bootstrapFunc(func(context.Context) error {
		store.secrets[pocketid.ManagementCredentialVaultPath] = map[string]interface{}{
			"encryption-key": "existing", pocketid.ManagementCredentialField: "generated",
		}
		return nil
	})
	identity := bootstrapFunc(func(context.Context) error { identityRuns++; return nil })
	service := NewService(store, k8s, "security", foundation, credential, identity)
	service.UseCertificates(manager)
	if err := service.Advance(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	state, err := k8s.CoreV1().ConfigMaps("security").Get(t.Context(), stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Data["phase"] != "awaiting-identity-verification" || identityRuns != 1 || foundationRuns != 2 {
		t.Fatalf("phase=%q identityRuns=%d foundationRuns=%d", state.Data["phase"], identityRuns, foundationRuns)
	}
}

func TestAdvanceWaitsWhenPocketIDCredentialResourceIsNotInstalled(t *testing.T) {
	store := &recoveryVault{
		status:  vault.LifecycleStatus{Initialized: true, Sealed: false},
		secrets: map[string]map[string]interface{}{},
	}
	k8s := fake.NewSimpleClientset(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: initSecretName, Namespace: "security"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "homelab-wildcard-tls", Namespace: "networking"}, Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: []byte("cert"), corev1.TLSPrivateKeyKey: []byte("key")}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stateConfigMapName, Namespace: "security"}, Data: map[string]string{"phase": "awaiting-certificate", "dnsDelegationVerified": "true"}},
	)
	registration := certificates.Registration{Username: "user", Password: "secret", FullDomain: "generated.auth.acme-dns.io", Subdomain: "generated"}
	manager, err := certificates.NewManager(certificates.Config{APIURL: "https://auth.acme-dns.io", Domain: "6940469.xyz", CredentialPath: "infrastructure/acme-dns", CertificateNS: "networking", TLSSecretName: "homelab-wildcard-tls"}, certificateStore{}, certificateRegistrar{registration}, certificateResolver{registration.FullDomain + "."})
	if err != nil {
		t.Fatal(err)
	}
	identityRuns := 0
	service := NewService(
		store,
		k8s,
		"security",
		bootstrapFunc(func(context.Context) error { return nil }),
		bootstrapFunc(func(context.Context) error { return nil }),
		bootstrapFunc(func(context.Context) error { identityRuns++; return nil }),
	)
	service.UseCertificates(manager)
	if err := service.Advance(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	state, err := k8s.CoreV1().ConfigMaps("security").Get(t.Context(), stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Data["phase"] != "awaiting-pocket-id-credential" || identityRuns != 0 {
		t.Fatalf("phase=%q identityRuns=%d", state.Data["phase"], identityRuns)
	}
}

func TestVerifyDNSDelegationRecordsOnlyAcceptanceState(t *testing.T) {
	k8s := fake.NewSimpleClientset()
	registration := certificates.Registration{Username: "user", Password: "secret", FullDomain: "generated.auth.acme-dns.io", Subdomain: "generated"}
	manager, err := certificates.NewManager(certificates.Config{APIURL: "https://auth.acme-dns.io", Domain: "6940469.xyz", CredentialPath: "infrastructure/acme-dns", CertificateNS: "networking", TLSSecretName: "homelab-wildcard-tls"}, certificateStore{}, certificateRegistrar{registration}, certificateResolver{registration.FullDomain + "."})
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{k8s: k8s, namespace: "security", certificates: manager}
	status, err := service.VerifyDNSDelegation(t.Context(), true)
	if err != nil || !status.DelegationValid {
		t.Fatalf("VerifyDNSDelegation() = %#v, %v", status, err)
	}
	state, err := k8s.CoreV1().ConfigMaps("security").Get(t.Context(), stateConfigMapName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Data["phase"] != "awaiting-certificate" || state.Data["dnsDelegationVerified"] != "true" {
		t.Fatalf("state = %#v", state.Data)
	}
	for _, value := range state.Data {
		if strings.Contains(value, registration.Password) {
			t.Fatal("password leaked into bootstrap state")
		}
	}
}
