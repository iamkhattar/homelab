package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
)

func TestCredentialsIsATopLevelCommand(t *testing.T) {
	root := New(BuildInfo{}, command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}))
	if _, _, err := root.Find([]string{"credentials"}); err != nil {
		t.Fatalf("credentials must be a top-level command: %v", err)
	}
	control, _, err := root.Find([]string{"control"})
	if err != nil {
		t.Fatal(err)
	}
	for _, child := range control.Commands() {
		if child.Name() == "credentials" {
			t.Fatal("credentials must not remain under control")
		}
	}
}

func TestControlCredentialsCommandIsRemoved(t *testing.T) {
	control := newControlCommand(&state{})
	if _, _, err := control.Find([]string{"credentials"}); err == nil {
		t.Fatal("Butler no longer brokers credentials; the control command must be gone")
	}
}

func TestCredentialsRejectsRolesOutsideTheAllowlist(t *testing.T) {
	for _, role := range []string{"cluster-admin", "butleradmin", "vaultadmin", "k8s-root", ""} {
		if err := validateCredentialRole(role); err == nil {
			t.Fatalf("role %q was accepted", role)
		}
	}
	for _, role := range credentialRoles {
		if err := validateCredentialRole(role); err != nil {
			t.Fatalf("role %q rejected: %v", role, err)
		}
	}
	if strings.Join(credentialRoles, ",") != "k8sadmin,k8soperator,k8sviewer" {
		t.Fatalf("allowlist = %v, want exactly the three Kubernetes groups", credentialRoles)
	}
}

func TestCredentialsRejectsUnusableTTL(t *testing.T) {
	for _, ttl := range []string{"", "0s", "-1h", "9h", "12h", "forever"} {
		if _, err := credentialTTLDuration(ttl); err == nil {
			t.Fatalf("ttl %q was accepted", ttl)
		}
	}
	for _, ttl := range []string{"15m", "1h", "8h"} {
		if _, err := credentialTTLDuration(ttl); err != nil {
			t.Fatalf("ttl %q rejected: %v", ttl, err)
		}
	}
}

func TestCredentialsCommandValidationRunsBeforeAnyClusterCall(t *testing.T) {
	var stderr bytes.Buffer
	root := New(BuildInfo{}, command.NewRunner(strings.NewReader(""), &bytes.Buffer{}, &stderr))
	root.SetArgs([]string{"--repo-root", testRepository(t), "--dry-run", "credentials", "--role", "k8sroot"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "k8sadmin, k8soperator, k8sviewer") {
		t.Fatalf("error = %v, want the role allowlist", err)
	}
	if strings.Contains(stderr.String(), "kubectl") {
		t.Fatalf("an invalid role must fail before consulting the cluster: %q", stderr.String())
	}
}

func TestRenderKubeconfigIsUsable(t *testing.T) {
	ca := strings.Repeat("QUJD", 200)
	payload, err := renderKubeconfig(kubeConnection{
		Cluster: "homelab",
		Server:  "https://192.168.1.163:6443",
		CAData:  ca,
	}, "vault-issued-token")
	if err != nil {
		t.Fatal(err)
	}

	var rendered kubeconfigFile
	if err := yaml.Unmarshal(payload, &rendered); err != nil {
		t.Fatalf("rendered kubeconfig is not valid YAML: %v", err)
	}
	if rendered.APIVersion != "v1" || rendered.Kind != "Config" {
		t.Fatalf("header = %s/%s", rendered.APIVersion, rendered.Kind)
	}
	if len(rendered.Clusters) != 1 || rendered.Clusters[0].Cluster.Server != "https://192.168.1.163:6443" {
		t.Fatalf("clusters = %#v", rendered.Clusters)
	}
	// A long CA must survive YAML line folding byte for byte.
	if rendered.Clusters[0].Cluster.CertificateAuthorityData != ca {
		t.Fatal("certificate-authority-data did not round-trip")
	}
	if len(rendered.Users) != 1 || rendered.Users[0].User.Token != "vault-issued-token" {
		t.Fatalf("users = %#v", rendered.Users)
	}
	if rendered.CurrentContext != "homelab" || len(rendered.Contexts) != 1 {
		t.Fatalf("contexts = %#v, current = %q", rendered.Contexts, rendered.CurrentContext)
	}
	if rendered.Contexts[0].Context.Cluster != "homelab" || rendered.Contexts[0].Context.User != "vault-credential" {
		t.Fatalf("context reference = %#v", rendered.Contexts[0].Context)
	}
	for _, forbidden := range []string{"password", "client-key-data", "exec:"} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("kubeconfig contains unexpected credential material %q", forbidden)
		}
	}
}

func TestRenderKubeconfigRequiresClusterAndToken(t *testing.T) {
	if _, err := renderKubeconfig(kubeConnection{Server: "https://example.test:6443"}, ""); err == nil {
		t.Fatal("a kubeconfig without a token must be refused")
	}
	if _, err := renderKubeconfig(kubeConnection{}, "token"); err == nil {
		t.Fatal("a kubeconfig without a server must be refused")
	}
}

func TestWriteCredentialFileIsPrivate(t *testing.T) {
	path := t.TempDir() + "/credential"
	if err := writeCredentialFile(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
	if err := writeCredentialFile(path, []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode after rewrite = %o, want 0600", info.Mode().Perm())
	}
}
