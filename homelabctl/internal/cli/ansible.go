package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type ansibleFlags struct {
	limit         string
	askBecomePass bool
}

func newNodeCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Prepare and reboot managed Debian nodes",
	}

	var prepare ansibleFlags
	var check bool
	prepareCmd := &cobra.Command{
		Use:   "prepare",
		Short: "Apply Debian updates and the host hardening baseline",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			args := ansibleArgs("playbooks/prepare.yml", prepare)
			if check {
				args = append(args, "--check", "--diff")
			}
			return s.runAnsible(cmd.Context(), "ansible-playbook", args...)
		},
	}
	bindAnsibleFlags(prepareCmd, &prepare)
	prepareCmd.Flags().BoolVar(&check, "check", false, "preview supported changes with --check --diff")

	var reboot ansibleFlags
	rebootCmd := &cobra.Command{
		Use:   "reboot",
		Short: "Reboot managed Debian nodes before K3s is installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-playbook", ansibleArgs("playbooks/reboot-node.yml", reboot)...)
		},
	}
	bindAnsibleFlags(rebootCmd, &reboot)

	connectCmd := &cobra.Command{
		Use:   "connect HOST",
		Short: "Open an SSH session using a host from the Ansible inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateInventoryHost(args[0]); err != nil {
				return err
			}
			connection, err := s.inventoryConnection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			sshArgs := []string{}
			if connection.IdentityFile != "" {
				sshArgs = append(sshArgs, "-i", connection.IdentityFile)
			}
			if connection.Port != 22 {
				sshArgs = append(sshArgs, "-p", strconv.Itoa(connection.Port))
			}
			target := connection.Address
			if connection.User != "" {
				target = connection.User + "@" + target
			}
			sshArgs = append(sshArgs, target)
			return s.run(cmd.Context(), s.root, "ssh", sshArgs...)
		},
	}

	var publicKeyPath string
	authorizeKeyCmd := &cobra.Command{
		Use:   "authorize-key HOST",
		Short: "Install a public SSH key during first-node bootstrap",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateInventoryHost(args[0]); err != nil {
				return err
			}
			keyPath, err := validatePublicKeyFile(publicKeyPath)
			if err != nil {
				return err
			}
			connection, err := s.inventoryConnection(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			sshArgs := []string{"-i", keyPath}
			if connection.Port != 22 {
				sshArgs = append(sshArgs, "-p", strconv.Itoa(connection.Port))
			}
			target := connection.Address
			if connection.User != "" {
				target = connection.User + "@" + target
			}
			sshArgs = append(sshArgs, target)
			return s.run(cmd.Context(), s.root, "ssh-copy-id", sshArgs...)
		},
	}
	authorizeKeyCmd.Flags().StringVar(&publicKeyPath, "public-key", "", "path to the public key file")
	_ = authorizeKeyCmd.MarkFlagRequired("public-key")

	var diagnose ansibleFlags
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Collect read-only Debian and SSH diagnostics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-playbook", ansibleArgs("playbooks/diagnose-node.yml", diagnose)...)
		},
	}
	bindAnsibleFlags(diagnoseCmd, &diagnose)

	cmd.AddCommand(prepareCmd, rebootCmd, connectCmd, authorizeKeyCmd, diagnoseCmd)
	return cmd
}

func validatePublicKeyFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("--public-key is required")
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving public key path: %w", err)
	}
	// #nosec G304 -- this is an explicit operator-supplied public-key path; its
	// contents and supported key type are validated before any SSH invocation.
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("reading public key: %w", err)
	}
	fields := strings.Fields(string(content))
	if len(fields) < 2 || !isPublicKeyType(fields[0]) {
		return "", fmt.Errorf("%s does not contain a supported OpenSSH public key", resolved)
	}
	decoded, err := base64.StdEncoding.DecodeString(fields[1])
	if err != nil || len(decoded) < 16 {
		return "", fmt.Errorf("%s contains invalid OpenSSH public-key data", resolved)
	}
	return resolved, nil
}

func isPublicKeyType(value string) bool {
	return value == "ssh-ed25519" || value == "ssh-rsa" ||
		strings.HasPrefix(value, "ecdsa-sha2-") ||
		strings.HasPrefix(value, "sk-ssh-") ||
		strings.HasPrefix(value, "sk-ecdsa-")
}

func bindAnsibleFlags(cmd *cobra.Command, flags *ansibleFlags) {
	cmd.Flags().StringVar(&flags.limit, "limit", "", "limit the play to an inventory host or group")
	cmd.Flags().BoolVar(&flags.askBecomePass, "ask-become-pass", false, "ask for the sudo password")
	cmd.PreRunE = func(command *cobra.Command, _ []string) error {
		if command.Flags().Changed("limit") {
			if err := validateNonBlank(flags.limit, "Ansible limit"); err != nil {
				return err
			}
		}
		return nil
	}
}

func ansibleArgs(playbook string, flags ansibleFlags) []string {
	args := []string{playbook}
	if flags.limit != "" {
		args = append(args, "--limit", flags.limit)
	}
	if flags.askBecomePass {
		args = append(args, "--ask-become-pass")
	}
	return args
}

func (s *state) ansibleEnvironment() map[string]string {
	return map[string]string{"ANSIBLE_HOME": filepath.Join(s.dir("ansible"), ".ansible")}
}

func (s *state) runAnsible(ctx context.Context, name string, args ...string) error {
	dir := s.dir("ansible")
	return s.runEnv(ctx, dir, s.ansibleEnvironment(), ansibleExecutable(dir, name), args...)
}

func (s *state) outputAnsible(ctx context.Context, name string, args ...string) (string, error) {
	dir := s.dir("ansible")
	return s.outputEnv(ctx, dir, s.ansibleEnvironment(), ansibleExecutable(dir, name), args...)
}

func ansibleExecutable(ansibleDir, name string) string {
	local := filepath.Join(ansibleDir, ".venv", "bin", name)
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		return local
	}
	return name
}

type inventoryConnection struct {
	Address      string
	User         string
	Port         int
	IdentityFile string
}

func (s *state) inventoryConnection(ctx context.Context, host string) (inventoryConnection, error) {
	if s.dryRun {
		return inventoryConnection{Address: host, Port: 22}, nil
	}
	out, err := s.outputAnsible(ctx, "ansible-inventory", "--host", host)
	if err != nil {
		return inventoryConnection{}, err
	}
	values := map[string]any{}
	if err := json.Unmarshal([]byte(out), &values); err != nil {
		return inventoryConnection{}, fmt.Errorf("parsing inventory variables for %s: %w", host, err)
	}
	if len(values) == 0 {
		return inventoryConnection{}, fmt.Errorf("inventory host %q was not found", host)
	}
	connection := inventoryConnection{Address: host, Port: 22}
	if value, ok := values["ansible_host"].(string); ok && value != "" {
		connection.Address = value
	}
	if value, ok := values["ansible_user"].(string); ok {
		connection.User = value
	}
	if value, ok := values["ansible_ssh_private_key_file"].(string); ok {
		connection.IdentityFile = value
	}
	if value, ok := values["ansible_port"].(float64); ok {
		connection.Port = int(value)
	} else if value, ok := values["ansible_port"].(string); ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return inventoryConnection{}, fmt.Errorf("invalid ansible_port %q for %s", value, host)
		}
		connection.Port = port
	}
	return connection, nil
}
