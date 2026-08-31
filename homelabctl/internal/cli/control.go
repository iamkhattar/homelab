package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cli/browser"
	"github.com/spf13/cobra"

	controlapi "github.com/iamkhattar/homelab/homelabctl/internal/control"
)

type controlOptions struct {
	address         string
	recoveryAddress string
	namespace       string
	token           string
	issuer          string
	clientID        string
	session         string
}

func newControlCommand(s *state) *cobra.Command {
	options := &controlOptions{}
	cmd := &cobra.Command{Use: "control", Short: "Operate Butler through its versioned API"}
	cmd.PersistentFlags().StringVar(&options.address, "address", "", "existing Butler base URL; otherwise create a private kubectl port-forward")
	cmd.PersistentFlags().StringVar(&options.recoveryAddress, "recovery-address", "", "existing Butler recovery URL; otherwise create a private kubectl port-forward")
	cmd.PersistentFlags().StringVar(&options.namespace, "namespace", "security", "namespace containing Butler")
	cmd.PersistentFlags().StringVar(&options.token, "token", "", "Pocket ID ID token override (or BUTLER_TOKEN)")
	cmd.PersistentFlags().StringVar(&options.issuer, "issuer", "https://auth.6940469.xyz", "Pocket ID OIDC issuer")
	cmd.PersistentFlags().StringVar(&options.clientID, "client-id", "homelabctl", "Pocket ID public OIDC client ID")
	cmd.PersistentFlags().StringVar(&options.session, "session-file", "", "OIDC session path (defaults to the private user config directory)")

	cmd.AddCommand(newControlLoginCommand(s, options))
	cmd.AddCommand(newControlLogoutCommand(s, options))
	cmd.AddCommand(newControlBootstrapCommand(s, options))
	cmd.AddCommand(newControlVerifyIdentityCommand(s, options))
	cmd.AddCommand(newControlRecoveryCommand(s, options))
	cmd.AddCommand(newControlGetCommand(s, options, "status", "Show reconciler status", "/api/v1/status"))
	cmd.AddCommand(newControlGetCommand(s, options, "operations", "List recent control-plane operations", "/api/v1/operations"))
	cmd.AddCommand(newControlGetCommand(s, options, "events", "List recent audit-safe events", "/api/v1/events"))
	cmd.AddCommand(newControlUsersCommand(s, options))
	cmd.AddCommand(newControlGetCommand(s, options, "groups", "List Pocket ID groups", "/api/v1/identity/groups"))
	cmd.AddCommand(newControlClientsCommand(s, options))
	cmd.AddCommand(newControlApplicationsCommand(s, options))
	cmd.AddCommand(newControlCredentialsCommand(s, options))
	return cmd
}

func newControlVerifyIdentityCommand(s *state, options *controlOptions) *cobra.Command {
	var vaultAddress, vaultRole string
	var confirm bool
	command := &cobra.Command{
		Use:     "verify-identity",
		Short:   "Prove Pocket ID login works for both Butler and Vault",
		Long:    "Authenticate to Butler with the cached Pocket ID session, complete a separate browser OIDC login to Vault, verify its administrator policies, revoke that temporary Vault token, and submit only non-secret acceptance evidence to Butler recovery.",
		Example: "  homelabctl control login\n  homelabctl control verify-identity --confirm",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required to finalize identity bootstrap")
			}
			if s.dryRun {
				s.info("would verify the cached Pocket ID session against Butler")
				s.info("would complete Vault OIDC role " + vaultRole + " at " + vaultAddress + " and revoke the resulting token")
				return withRecoveryClient(cmd.Context(), s, options, func(*controlapi.Client) error { return nil })
			}
			var principal struct {
				Subject string `json:"subject"`
				Role    string `json:"role"`
			}
			if err := withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
				return client.Do(cmd.Context(), http.MethodGet, "/api/v1/me", nil, &principal)
			}); err != nil {
				return fmt.Errorf("verifying Pocket ID login to Butler: %w", err)
			}
			if principal.Subject == "" || principal.Role != "admin" {
				return fmt.Errorf("Pocket ID identity must map to Butler admin before bootstrap can complete")
			}
			evidence, err := controlapi.VerifyVaultOIDCLogin(cmd.Context(), controlapi.VaultOIDCOptions{
				Address: vaultAddress,
				Role:    vaultRole,
				OpenURL: func(address string) error {
					s.info("If the browser does not open, visit: " + address)
					return browser.OpenURL(address)
				},
			})
			if err != nil {
				return err
			}
			return withRecoveryClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
				var status interface{}
				if err := client.Do(cmd.Context(), http.MethodPost, "/api/v1/bootstrap/identity-verification", map[string]interface{}{
					"pocketIdSubject": principal.Subject,
					"butlerRole":      principal.Role,
					"vaultPolicies":   evidence.Policies,
				}, &status); err != nil {
					return err
				}
				s.success("Pocket ID login to Butler and Vault verified; bootstrap is operational")
				return printJSON(s, status)
			})
		},
	}
	defaultVaultAddress := os.Getenv("VAULT_ADDR")
	if defaultVaultAddress == "" {
		defaultVaultAddress = "https://vault.6940469.xyz"
	}
	command.Flags().StringVar(&vaultAddress, "vault-address", defaultVaultAddress, "Vault HTTPS address")
	command.Flags().StringVar(&vaultRole, "vault-role", "homelab-admin", "Vault OIDC role expected to grant administrator policies")
	command.Flags().BoolVar(&confirm, "confirm", false, "confirm final identity acceptance and bootstrap completion")
	return command
}

func newControlCredentialsCommand(s *state, options *controlOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "Issue bounded credentials through Butler",
		Long:  "Issue credentials only from Butler's server-side allowlist. The CLI cannot widen the role, namespace, or maximum lifetime.",
	}
	var role, ttl, format string
	issue := &cobra.Command{Use: "issue", Short: "Issue a short-lived Vault-backed Kubernetes token", Long: "Ask Butler to use an approved Vault Kubernetes secrets-engine role and return a short-lived token without persisting it.", Example: "  homelabctl control credentials issue --role homelab-viewer --ttl 15m\n  homelabctl control credentials issue --role homelab-operator --format json", Args: cobra.NoArgs, RunE: func(command *cobra.Command, _ []string) error {
		if err := validateReleaseName(role); err != nil {
			return fmt.Errorf("credential role: %w", err)
		}
		if _, err := time.ParseDuration(ttl); err != nil {
			return fmt.Errorf("invalid credential TTL: %w", err)
		}
		return withNormalClient(command.Context(), s, options, func(client *controlapi.Client) error {
			var credential struct {
				Role, ServiceAccount, Namespace, Token, LeaseID string
				TTLSeconds                                      int64
				ExpiresAt                                       time.Time
			}
			if err := client.Do(command.Context(), http.MethodPost, "/api/v1/access/kubernetes-credentials", map[string]string{"role": role, "ttl": ttl}, &credential); err != nil {
				return err
			}
			if format == "json" {
				return printJSON(s, credential)
			}
			return printJSON(s, map[string]interface{}{
				"apiVersion": "client.authentication.k8s.io/v1", "kind": "ExecCredential",
				"status": map[string]interface{}{"expirationTimestamp": credential.ExpiresAt, "token": credential.Token},
			})
		})
	}}
	issue.Flags().StringVar(&role, "role", "homelab-viewer", "approved Vault Kubernetes role")
	issue.Flags().StringVar(&ttl, "ttl", "15m", "requested token lifetime")
	issue.Flags().StringVar(&format, "format", "exec-credential", "exec-credential or json")
	issue.PreRunE = func(_ *cobra.Command, _ []string) error {
		if format != "exec-credential" && format != "json" {
			return fmt.Errorf("format must be exec-credential or json")
		}
		return nil
	}
	cmd.AddCommand(issue)
	return cmd
}

func newControlLoginCommand(s *state, options *controlOptions) *cobra.Command {
	login := &cobra.Command{Use: "login", Short: "Sign in to Pocket ID with browser-based PKCE", Long: "Open Pocket ID in a browser, complete Authorization Code with PKCE on a loopback callback, and cache the validated short-lived ID token with private permissions.", Example: "  homelabctl control login\n  homelabctl control login --issuer https://auth.6940469.xyz", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if s.dryRun {
			s.info("would open Pocket ID in a browser and wait on http://" + controlapi.LoginCallbackAddress + "/callback")
			return nil
		}
		session, err := controlapi.InteractiveLogin(cmd.Context(), controlapi.LoginOptions{
			Issuer: options.issuer, ClientID: options.clientID, Timeout: 3 * time.Minute,
			OpenURL: func(address string) error {
				s.info("If the browser does not open, visit: " + address)
				return browser.OpenURL(address)
			},
		})
		if err != nil {
			return err
		}
		path, err := controlSessionPath(options)
		if err != nil {
			return err
		}
		if err := controlapi.SaveSession(path, session); err != nil {
			return err
		}
		s.success("Pocket ID session stored with private permissions until " + session.ExpiresAt.Local().Format(time.RFC3339))
		return nil
	}}
	return login
}

func newControlLogoutCommand(s *state, options *controlOptions) *cobra.Command {
	return &cobra.Command{Use: "logout", Short: "Remove the locally cached Pocket ID session", Long: "Delete the private local Pocket ID session file. This does not revoke independently issued Kubernetes credentials.", Example: "  homelabctl control logout", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		path, err := controlSessionPath(options)
		if err != nil {
			return err
		}
		if s.dryRun {
			s.info("would remove " + path)
			return nil
		}
		if err := controlapi.RemoveSession(path); err != nil {
			return err
		}
		s.success("Pocket ID session removed")
		return nil
	}}
}

func controlSessionPath(options *controlOptions) (string, error) {
	if options.session != "" {
		return filepath.Abs(options.session)
	}
	return controlapi.SessionPath()
}

func newControlClientsCommand(s *state, options *controlOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "clients", Short: "Inspect and rotate Pocket ID OIDC clients through Butler"}
	cmd.AddCommand(newControlGetCommand(s, options, "list", "List Pocket ID OIDC clients", "/api/v1/identity/clients"))
	rotate := &cobra.Command{Use: "rotate <id>", Short: "Rotate a confidential OIDC client secret directly into Vault", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAPIIdentifier(args[0], "client ID"); err != nil {
			return err
		}
		return withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
			var result interface{}
			if err := client.Do(cmd.Context(), http.MethodPost, "/api/v1/identity/clients/"+args[0]+"/rotate", nil, &result); err != nil {
				return err
			}
			return printJSON(s, result)
		})
	}}
	cmd.AddCommand(rotate)
	return cmd
}

func newControlBootstrapCommand(s *state, options *controlOptions) *cobra.Command {
	var confirm bool
	var pocketIDKeyFile string
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Advance the private, resumable Vault bootstrap",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !confirm {
				return fmt.Errorf("--confirm is required to advance bootstrap")
			}
			return withRecoveryClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
				var status map[string]interface{}
				if err := client.Do(cmd.Context(), http.MethodPost, "/api/v1/bootstrap/advance", map[string]bool{"confirm": true}, &status); err != nil {
					return err
				}
				if pocketIDKeyFile != "" {
					raw, err := os.ReadFile(pocketIDKeyFile) // #nosec G304 -- explicit operator-provided secret file.
					if err != nil {
						return fmt.Errorf("reading Pocket ID API key file: %w", err)
					}
					if len(raw) > 8<<10 {
						return fmt.Errorf("Pocket ID API key file is too large")
					}
					if err := client.Do(cmd.Context(), http.MethodPut, "/api/v1/bootstrap/pocket-id-api-key", map[string]string{"apiKey": strings.TrimSpace(string(raw))}, nil); err != nil {
						return err
					}
					s.success("Pocket ID management API key stored directly in Vault")
					if err := client.Do(cmd.Context(), http.MethodPost, "/api/v1/bootstrap/advance", map[string]bool{"confirm": true}, &status); err != nil {
						return err
					}
				}
				return printJSON(s, status)
			})
		},
	}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm the one-time privileged bootstrap step")
	cmd.Flags().StringVar(&pocketIDKeyFile, "pocket-id-api-key-file", "", "read the Pocket ID API key from a local file and write it directly to Vault")
	return cmd
}

func newControlRecoveryCommand(s *state, options *controlOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recovery",
		Short: "Inspect the lower-layer recovery plane without Pocket ID or Vault auth",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withRecoveryClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
				var status map[string]interface{}
				if err := client.Do(cmd.Context(), http.MethodGet, "/api/v1/recovery/status", nil, &status); err != nil {
					return err
				}
				return printJSON(s, status)
			})
		},
	}
	var output, recipient string
	export := &cobra.Command{
		Use:   "export",
		Short: "Export Vault initialization material directly into an age-encrypted bundle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			absolute, err := filepath.Abs(output)
			if err != nil {
				return fmt.Errorf("resolving recovery output: %w", err)
			}
			if _, err := validateRecoveryDestination(s.root, filepath.Dir(absolute)); err != nil {
				return err
			}
			if s.dryRun {
				return s.run(cmd.Context(), s.root, "kubectl", "--context", s.kubeContext, "--namespace", options.namespace, "get", "secret", "butler-vault-init", "--output=json")
			}
			raw, err := s.output(cmd.Context(), s.root, "kubectl", "--context", s.kubeContext, "--namespace", options.namespace, "get", "secret", "butler-vault-init", "--output=json")
			if err != nil {
				return err
			}
			var secret struct {
				Data map[string]string `json:"data"`
			}
			if err := json.Unmarshal([]byte(raw), &secret); err != nil {
				return fmt.Errorf("decoding recovery Secret: %w", err)
			}
			if len(secret.Data) == 0 {
				return fmt.Errorf("recovery Secret contained no data")
			}
			bundle := controlapi.RecoveryBundle{Version: 1, Context: s.kubeContext, Namespace: options.namespace, SecretName: "butler-vault-init", ExportedAt: time.Now().UTC(), Data: secret.Data}
			if err := controlapi.EncryptRecoveryBundle(absolute, recipient, bundle); err != nil {
				return err
			}
			s.success("encrypted recovery bundle written to " + absolute)
			return nil
		},
	}
	export.Flags().StringVar(&output, "output", "", "new .age recovery bundle path outside the repository")
	export.Flags().StringVar(&recipient, "age-recipient", "", "age X25519 recipient")
	_ = export.MarkFlagRequired("output")
	_ = export.MarkFlagRequired("age-recipient")
	cmd.AddCommand(export)
	return cmd
}

func newControlGetCommand(s *state, options *controlOptions, use, short, path string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
			var result interface{}
			if err := client.Do(cmd.Context(), http.MethodGet, path, nil, &result); err != nil {
				return err
			}
			return printJSON(s, result)
		})
	}}
}

func newControlUsersCommand(s *state, options *controlOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "users", Short: "Manage Pocket ID users through Butler"}
	cmd.AddCommand(newControlGetCommand(s, options, "list", "List Pocket ID users", "/api/v1/identity/users"))

	var username, email, firstName, lastName, displayName string
	create := &cobra.Command{Use: "create", Short: "Create a non-administrator Pocket ID user", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateNonBlank(username, "username"); err != nil {
			return err
		}
		body := map[string]interface{}{"username": username, "firstName": firstName, "lastName": lastName, "displayName": displayName, "isAdmin": false, "disabled": false}
		if email != "" {
			body["email"] = email
		}
		return withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
			var result interface{}
			if err := client.Do(cmd.Context(), http.MethodPost, "/api/v1/identity/users", body, &result); err != nil {
				return err
			}
			return printJSON(s, result)
		})
	}}
	create.Flags().StringVar(&username, "username", "", "unique Pocket ID username")
	create.Flags().StringVar(&email, "email", "", "email address")
	create.Flags().StringVar(&firstName, "first-name", "", "first name")
	create.Flags().StringVar(&lastName, "last-name", "", "last name")
	create.Flags().StringVar(&displayName, "display-name", "", "display name")

	var disabled bool
	update := &cobra.Command{Use: "update <id>", Short: "Update or disable a Pocket ID user", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAPIIdentifier(args[0], "user ID"); err != nil {
			return err
		}
		body := map[string]interface{}{"username": username, "firstName": firstName, "lastName": lastName, "displayName": displayName, "isAdmin": false, "disabled": disabled}
		if email != "" {
			body["email"] = email
		}
		return withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
			var result interface{}
			if err := client.Do(cmd.Context(), http.MethodPut, "/api/v1/identity/users/"+args[0], body, &result); err != nil {
				return err
			}
			return printJSON(s, result)
		})
	}}
	update.Flags().StringVar(&username, "username", "", "Pocket ID username (required by Pocket ID update API)")
	update.Flags().StringVar(&email, "email", "", "email address")
	update.Flags().StringVar(&firstName, "first-name", "", "first name")
	update.Flags().StringVar(&lastName, "last-name", "", "last name")
	update.Flags().StringVar(&displayName, "display-name", "", "display name")
	update.Flags().BoolVar(&disabled, "disabled", false, "disable the account")

	var groups []string
	setGroups := &cobra.Command{Use: "set-groups <id>", Short: "Replace a user's Pocket ID group memberships", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAPIIdentifier(args[0], "user ID"); err != nil {
			return err
		}
		return withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
			var result interface{}
			if err := client.Do(cmd.Context(), http.MethodPut, "/api/v1/identity/users/"+args[0]+"/groups", map[string][]string{"groupIds": groups}, &result); err != nil {
				return err
			}
			return printJSON(s, result)
		})
	}}
	setGroups.Flags().StringSliceVar(&groups, "group", nil, "Pocket ID group ID; repeat as required")
	cmd.AddCommand(create, update, setGroups)
	return cmd
}

func newControlApplicationsCommand(s *state, options *controlOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "applications", Short: "Manage application integration metadata"}
	cmd.AddCommand(newControlGetCommand(s, options, "list", "List managed application integrations", "/api/v1/applications"))
	var namespace, authentication, owner, host string
	var vaultPaths []string
	put := &cobra.Command{Use: "put <name>", Short: "Create or update an ApplicationIntegration", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateReleaseName(args[0]); err != nil {
			return fmt.Errorf("application name: %w", err)
		}
		body := map[string]interface{}{"name": args[0], "namespace": namespace, "authentication": authentication, "owner": owner, "ingressHost": host, "vaultPaths": vaultPaths}
		return withNormalClient(cmd.Context(), s, options, func(client *controlapi.Client) error {
			var result interface{}
			if err := client.Do(cmd.Context(), http.MethodPut, "/api/v1/applications/"+args[0], body, &result); err != nil {
				return err
			}
			return printJSON(s, result)
		})
	}}
	put.Flags().StringVar(&namespace, "app-namespace", "", "application namespace")
	put.Flags().StringVar(&authentication, "authentication", "", "native-oidc, forward-auth, or none")
	put.Flags().StringVar(&owner, "owner", "", "owning Pocket ID group")
	put.Flags().StringVar(&host, "host", "", "internal ingress hostname")
	put.Flags().StringSliceVar(&vaultPaths, "vault-path", nil, "approved Vault path; repeat as required")
	cmd.AddCommand(put)
	return cmd
}

func withNormalClient(ctx context.Context, s *state, options *controlOptions, fn func(*controlapi.Client) error) error {
	token := options.token
	if token == "" {
		token = os.Getenv("BUTLER_TOKEN")
	}
	if strings.TrimSpace(token) == "" {
		path, err := controlSessionPath(options)
		if err != nil {
			return err
		}
		session, err := controlapi.LoadSession(path, time.Now())
		if err != nil {
			return err
		}
		if session.Issuer != options.issuer || session.ClientID != options.clientID {
			return fmt.Errorf("cached Pocket ID session is for issuer %s and client %s; run homelabctl control login", session.Issuer, session.ClientID)
		}
		token = session.IDToken
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("Pocket ID login is required")
	}
	return withControlClient(ctx, s, options, "butler", 8080, token, fn)
}

func withRecoveryClient(ctx context.Context, s *state, options *controlOptions, fn func(*controlapi.Client) error) error {
	if s.dryRun {
		return s.run(ctx, s.root, "kubectl", "--context", s.kubeContext, "--namespace", options.namespace, "create", "token", "butler-recovery-client", "--audience=butler-recovery", "--duration=10m")
	}
	token, err := s.output(ctx, s.root, "kubectl", "--context", s.kubeContext, "--namespace", options.namespace, "create", "token", "butler-recovery-client", "--audience=butler-recovery", "--duration=10m")
	if err != nil {
		return err
	}
	if options.recoveryAddress != "" {
		return fn(controlapi.NewClient(options.recoveryAddress, token))
	}
	return withControlClient(ctx, s, options, "butler-recovery", 8081, token, fn)
}

func withControlClient(ctx context.Context, s *state, options *controlOptions, service string, port int, token string, fn func(*controlapi.Client) error) error {
	address := options.address
	if address != "" {
		return fn(controlapi.NewClient(address, token))
	}
	if s.dryRun {
		return s.run(ctx, s.root, "kubectl", "--context", s.kubeContext, "--namespace", options.namespace, "port-forward", "service/"+service, fmt.Sprintf("%d:%d", port, port), "--address", "127.0.0.1")
	}
	tunnel, err := controlapi.StartTunnel(ctx, s.runner.Stderr, s.kubeContext, options.namespace, service, port)
	if err != nil {
		return err
	}
	defer func() { _ = tunnel.Close() }()
	return fn(controlapi.NewClient(tunnel.URL, token))
}

func printJSON(s *state, value interface{}) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("formatting response: %w", err)
	}
	s.print("%s\n", raw)
	return nil
}
