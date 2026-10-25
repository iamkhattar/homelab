package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/cli/browser"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	controlapi "github.com/iamkhattar/homelab/homelabctl/internal/control"
)

const (
	defaultCredentialRole = "k8sviewer"
	// Vault's Kubernetes secrets engine issues a token for a service account
	// that must exist in the requested namespace. The vault-managed-<tier>
	// accounts live in kube-system, so that is the namespace the request names.
	// The resulting credential is cluster-wide: the service account's
	// ClusterRoleBinding, not this namespace, decides what it can reach.
	defaultCredentialNamespace = "kube-system"
	defaultCredentialTTL       = "1h"
	maximumCredentialTTL       = 8 * time.Hour
)

// credentialRoles is the complete set of Vault Kubernetes secrets-engine roles
// a human may request. It is an allowlist, not a suggestion: anything else is
// rejected before a browser opens.
var credentialRoles = []string{"k8sadmin", "k8soperator", "k8sviewer"}

func newCredentialsCommand(s *state) *cobra.Command {
	var role, namespace, ttl, format, output, vaultAddress, server, caData, cluster string
	command := &cobra.Command{
		Use:   "credentials",
		Short: "Mint a short-lived Kubernetes credential from Vault as yourself",
		Long: `Authenticate to Vault with browser OIDC and ask Vault's Kubernetes secrets
engine for a short-lived credential. Authorization comes from the Vault policy
bound to your Pocket ID group: Butler is not involved and cannot widen it.

The default output is a ready-to-use kubeconfig on stdout, so the command can be
sliced straight into a temporary file. Nothing is written to disk unless
--output is given, and then with mode 0600.`,
		Example: `  homelabctl credentials > /tmp/homelab.kubeconfig
  KUBECONFIG=/tmp/homelab.kubeconfig kubectl get nodes
  homelabctl credentials --role k8sadmin --ttl 2h
  homelabctl credentials --format token --output /secure/token`,
		Args: cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if err := validateCredentialRole(role); err != nil {
				return err
			}
			if _, err := credentialTTLDuration(ttl); err != nil {
				return err
			}
			if err := validateCredentialNamespace(namespace); err != nil {
				return err
			}
			if format != "kubeconfig" && format != "token" {
				return fmt.Errorf("--format must be kubeconfig or token")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			lifetime, err := credentialTTLDuration(ttl)
			if err != nil {
				return err
			}
			if s.dryRun {
				fmt.Fprintf(s.runner.Stderr, "would complete Vault OIDC login for role %s\n", role)
				fmt.Fprintf(s.runner.Stderr, "would request kubernetes/creds/%s in namespace %s for %s\n", role, namespace, lifetime)
				if output != "" {
					fmt.Fprintf(s.runner.Stderr, "would write %s output to %s with mode 0600\n", format, output)
				}
				return nil
			}
			// Cluster details are only needed for the kubeconfig rendering.
			connection := kubeConnection{}
			if format == "kubeconfig" {
				connection, err = s.resolveKubeConnection(cmd.Context(), server, caData, cluster)
				if err != nil {
					return err
				}
			}

			login, err := controlapi.LoginVaultOIDC(cmd.Context(), controlapi.VaultOIDCOptions{
				Address: vaultAddress,
				Role:    role,
				OpenURL: func(address string) error {
					// Progress goes to stderr so stdout stays a clean payload.
					fmt.Fprintln(s.runner.Stderr, "If the browser does not open, visit: "+address)
					return browser.OpenURL(address)
				},
			})
			if err != nil {
				return err
			}
			// The login token is intentionally not revoked: revoking it would
			// also revoke the leases it created, including the credential just
			// minted. It expires with the role's TTL.
			credential, err := controlapi.ReadKubernetesCredential(cmd.Context(), nil, vaultAddress, login.Token, role, namespace, lifetime.String())
			if err != nil {
				// Vault's permission error is the authorization failure: surface
				// it verbatim instead of replacing it with a generic message.
				return err
			}

			var payload []byte
			switch format {
			case "token":
				payload = []byte(credential.Token + "\n")
			default:
				payload, err = renderKubeconfig(connection, credential.Token)
				if err != nil {
					return err
				}
			}
			if output == "" {
				_, err = s.out().Write(payload)
				return err
			}
			return writeCredentialFile(output, payload)
		},
	}
	command.Flags().StringVar(&role, "role", defaultCredentialRole, "Vault Kubernetes role: k8sadmin, k8soperator or k8sviewer")
	command.Flags().StringVar(&namespace, "namespace", defaultCredentialNamespace, "namespace holding the Vault-managed service account (not the workload namespace)")
	command.Flags().StringVar(&ttl, "ttl", defaultCredentialTTL, "credential lifetime (maximum 8h)")
	command.Flags().StringVar(&format, "format", "kubeconfig", "kubeconfig or token")
	command.Flags().StringVar(&output, "output", "", "write to this path with mode 0600 instead of stdout")
	defaultVaultAddress := os.Getenv("VAULT_ADDR")
	if defaultVaultAddress == "" {
		defaultVaultAddress = "https://vault.6940469.xyz"
	}
	command.Flags().StringVar(&vaultAddress, "vault-address", defaultVaultAddress, "Vault HTTPS address")
	command.Flags().StringVar(&server, "server", "", "Kubernetes API server for the kubeconfig (defaults to the current context)")
	command.Flags().StringVar(&caData, "certificate-authority-data", "", "base64 cluster CA for the kubeconfig (defaults to the current context)")
	command.Flags().StringVar(&cluster, "cluster", "homelab", "cluster name written into the kubeconfig")
	return command
}

func validateCredentialRole(role string) error {
	if !slices.Contains(credentialRoles, role) {
		return fmt.Errorf("--role must be one of %s", strings.Join(credentialRoles, ", "))
	}
	return nil
}

func credentialTTLDuration(raw string) (time.Duration, error) {
	lifetime, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --ttl: %w", err)
	}
	if lifetime <= 0 {
		return 0, fmt.Errorf("--ttl must be positive")
	}
	if lifetime > maximumCredentialTTL {
		return 0, fmt.Errorf("--ttl must not exceed %s", maximumCredentialTTL)
	}
	return lifetime, nil
}

func validateCredentialNamespace(namespace string) error {
	if err := validateReleaseName(namespace); err != nil {
		return fmt.Errorf("--namespace: %w", err)
	}
	return nil
}

// kubeConnection is the non-secret half of a kubeconfig.
type kubeConnection struct {
	Cluster  string
	Server   string
	CAData   string
	Insecure bool
}

// resolveKubeConnection fills the cluster endpoint from the operator's current
// kubectl context, so the rendered kubeconfig trusts the same CA as the
// kubeconfig they already use. Explicit flags always win.
func (s *state) resolveKubeConnection(ctx context.Context, server, caData, cluster string) (kubeConnection, error) {
	connection := kubeConnection{Cluster: cluster, Server: server, CAData: caData}
	if connection.Server != "" && connection.CAData != "" {
		return connection, nil
	}
	raw, err := s.output(ctx, s.root, "kubectl", "--context", s.kubeContext, "config", "view", "--minify", "-o", "json")
	if err != nil {
		return kubeConnection{}, fmt.Errorf("reading the current kubeconfig (pass --server and --certificate-authority-data to skip it): %w", err)
	}
	var view struct {
		Clusters []struct {
			Cluster struct {
				Server                   string `json:"server"`
				CertificateAuthorityData string `json:"certificate-authority-data"`
				InsecureSkipTLSVerify    bool   `json:"insecure-skip-tls-verify"`
			} `json:"cluster"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		return kubeConnection{}, fmt.Errorf("parsing kubectl config view: %w", err)
	}
	if len(view.Clusters) > 0 {
		current := view.Clusters[0].Cluster
		if connection.Server == "" {
			connection.Server = current.Server
		}
		if connection.CAData == "" {
			connection.CAData = current.CertificateAuthorityData
			connection.Insecure = current.InsecureSkipTLSVerify
		}
	}
	if connection.Server == "" {
		return kubeConnection{}, fmt.Errorf("the current kubeconfig has no server; pass --server")
	}
	if connection.CAData == "" && !connection.Insecure {
		return kubeConnection{}, fmt.Errorf("the current kubeconfig has no certificate authority; pass --certificate-authority-data")
	}
	return connection, nil
}

type kubeconfigCluster struct {
	Name    string             `yaml:"name"`
	Cluster kubeconfigEndpoint `yaml:"cluster"`
}

type kubeconfigEndpoint struct {
	Server                   string `yaml:"server"`
	CertificateAuthorityData string `yaml:"certificate-authority-data,omitempty"`
	InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify,omitempty"`
}

type kubeconfigUser struct {
	Name string             `yaml:"name"`
	User kubeconfigIdentity `yaml:"user"`
}

type kubeconfigIdentity struct {
	Token string `yaml:"token"`
}

type kubeconfigContext struct {
	Name    string              `yaml:"name"`
	Context kubeconfigReference `yaml:"context"`
}

type kubeconfigReference struct {
	Cluster string `yaml:"cluster"`
	User    string `yaml:"user"`
}

type kubeconfigFile struct {
	APIVersion     string              `yaml:"apiVersion"`
	Kind           string              `yaml:"kind"`
	Clusters       []kubeconfigCluster `yaml:"clusters"`
	Users          []kubeconfigUser    `yaml:"users"`
	Contexts       []kubeconfigContext `yaml:"contexts"`
	CurrentContext string              `yaml:"current-context"`
}

// renderKubeconfig emits a single-context kubeconfig whose user carries the
// Vault-minted token. The token is never placed on disk by this function.
func renderKubeconfig(connection kubeConnection, token string) ([]byte, error) {
	if connection.Server == "" {
		return nil, fmt.Errorf("a Kubernetes API server is required to render a kubeconfig")
	}
	if token == "" {
		return nil, fmt.Errorf("a Kubernetes token is required to render a kubeconfig")
	}
	name := connection.Cluster
	if name == "" {
		name = "homelab"
	}
	file := kubeconfigFile{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: []kubeconfigCluster{{
			Name: name,
			Cluster: kubeconfigEndpoint{
				Server:                   connection.Server,
				CertificateAuthorityData: connection.CAData,
				InsecureSkipTLSVerify:    connection.Insecure,
			},
		}},
		Users: []kubeconfigUser{{
			Name: "vault-credential",
			User: kubeconfigIdentity{Token: token},
		}},
		Contexts: []kubeconfigContext{{
			Name:    name,
			Context: kubeconfigReference{Cluster: name, User: "vault-credential"},
		}},
		CurrentContext: name,
	}
	payload, err := yaml.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("rendering kubeconfig: %w", err)
	}
	return payload, nil
}

// writeCredentialFile writes the payload with mode 0600. An existing file is
// truncated, never appended, and its permissions are tightened even if it was
// created more permissively.
func writeCredentialFile(path string, payload []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- the operator explicitly chooses the output path.
	if err != nil {
		return fmt.Errorf("writing credential output: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("setting credential output permissions: %w", err)
	}
	if _, err := file.Write(payload); err != nil {
		return fmt.Errorf("writing credential output: %w", err)
	}
	return nil
}
