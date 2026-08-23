package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/spf13/cobra"
)

var snapshotNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func newClusterCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "Install, upgrade and inspect the K3s cluster",
	}

	var bootstrap ansibleFlags
	bootstrapCmd := &cobra.Command{
		Use:     "bootstrap",
		Aliases: []string{"install"},
		Short:   "Prepare nodes and install or reconcile K3s",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-playbook", ansibleArgs("playbooks/site.yml", bootstrap)...)
		},
	}
	bindAnsibleFlags(bootstrapCmd, &bootstrap)

	var upgrade ansibleFlags
	upgradeCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade K3s to the version pinned in inventory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-playbook", ansibleArgs("playbooks/upgrade.yml", upgrade)...)
		},
	}
	bindAnsibleFlags(upgradeCmd, &upgrade)

	var reboot ansibleFlags
	rebootCmd := &cobra.Command{
		Use:   "reboot",
		Short: "Reboot K3s servers and agents with health checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-playbook", ansibleArgs("playbooks/reboot.yml", reboot)...)
		},
	}
	bindAnsibleFlags(rebootCmd, &reboot)

	var allPods bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show nodes and non-healthy pods",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := s.run(cmd.Context(), s.root, "kubectl", "--context", s.kubeContext, "get", "nodes", "-o", "wide"); err != nil {
				return err
			}
			args := []string{"--context", s.kubeContext, "get", "pods", "--all-namespaces"}
			if !allPods {
				args = append(args, "--field-selector=status.phase!=Running,status.phase!=Succeeded")
			}
			return s.run(cmd.Context(), s.root, "kubectl", args...)
		},
	}
	statusCmd.Flags().BoolVar(&allPods, "all-pods", false, "include healthy and completed pods")

	nodesCmd := &cobra.Command{
		Use:   "nodes",
		Short: "List K3s nodes with labels",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.run(cmd.Context(), s.root, "kubectl", "--context", s.kubeContext, "get", "nodes", "-o", "wide", "--show-labels")
		},
	}

	var diagnose ansibleFlags
	diagnoseCmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Collect read-only K3s and Kubernetes diagnostics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return s.runAnsible(cmd.Context(), "ansible-playbook", ansibleArgs("playbooks/diagnose-cluster.yml", diagnose)...)
		},
	}
	bindAnsibleFlags(diagnoseCmd, &diagnose)

	snapshotCmd := newSnapshotCommand(s)
	recoveryCmd := newRecoveryCommand(s)

	cmd.AddCommand(bootstrapCmd, upgradeCmd, rebootCmd, statusCmd, nodesCmd, diagnoseCmd, snapshotCmd, recoveryCmd)
	return cmd
}

func newSnapshotCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create and inspect embedded-etcd snapshots",
	}

	var listFlags ansibleFlags
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List snapshots on Titan",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runSnapshotPlaybook(command, s, listFlags, "list", "")
		},
	}
	bindAnsibleFlags(listCmd, &listFlags)

	var saveFlags ansibleFlags
	var name string
	saveCmd := &cobra.Command{
		Use:   "save",
		Short: "Create an on-demand snapshot on Titan",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateSnapshotName(name); err != nil {
				return err
			}
			return runSnapshotPlaybook(command, s, saveFlags, "save", name)
		},
	}
	bindAnsibleFlags(saveCmd, &saveFlags)
	saveCmd.Flags().StringVar(&name, "name", "manual", "snapshot name prefix")

	cmd.AddCommand(listCmd, saveCmd)
	return cmd
}

func runSnapshotPlaybook(cmd *cobra.Command, s *state, flags ansibleFlags, action, name string) error {
	values, err := json.Marshal(map[string]string{
		"homelab_snapshot_action": action,
		"homelab_snapshot_name":   name,
	})
	if err != nil {
		return err
	}
	args := ansibleArgs("playbooks/snapshot.yml", flags)
	args = append(args, "--extra-vars", string(values))
	return s.runAnsible(cmd.Context(), "ansible-playbook", args...)
}

func newRecoveryCommand(s *state) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recovery",
		Short: "Export cluster bootstrap recovery material",
	}

	var flags ansibleFlags
	var destination string
	var name string
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Create a snapshot and fetch it with the K3s server token",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateSnapshotName(name); err != nil {
				return err
			}
			absolute, err := validateRecoveryDestination(s.root, destination)
			if err != nil {
				return err
			}
			exportDirectory := filepath.Join(absolute, time.Now().UTC().Format("20060102T150405Z"))
			if !s.dryRun {
				if err := os.MkdirAll(absolute, 0o700); err != nil {
					return fmt.Errorf("creating recovery destination: %w", err)
				}
				if err := os.Mkdir(exportDirectory, 0o700); err != nil {
					return fmt.Errorf("creating unique recovery export: %w", err)
				}
			}
			values, err := json.Marshal(map[string]string{
				"homelab_recovery_export_dir": exportDirectory,
				"homelab_snapshot_name":       name,
			})
			if err != nil {
				return err
			}
			args := ansibleArgs("playbooks/recovery-export.yml", flags)
			args = append(args, "--extra-vars", string(values))
			return s.runAnsible(command.Context(), "ansible-playbook", args...)
		},
	}
	bindAnsibleFlags(exportCmd, &flags)
	exportCmd.Flags().StringVar(&destination, "destination", "", "local directory for recovery material")
	exportCmd.Flags().StringVar(&name, "name", "recovery", "snapshot name prefix")
	_ = exportCmd.MarkFlagRequired("destination")

	cmd.AddCommand(exportCmd)
	return cmd
}
