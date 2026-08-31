package cli

import (
	"context"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/iamkhattar/homelab/homelabctl/internal/command"
	"github.com/iamkhattar/homelab/homelabctl/internal/repository"
	"github.com/iamkhattar/homelab/homelabctl/internal/ui"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type state struct {
	runner      *command.Runner
	build       BuildInfo
	repoFlag    string
	kubeContext string
	dryRun      bool
	root        string
	ui          *ui.Printer
}

func New(build BuildInfo, runner *command.Runner) *cobra.Command {
	s := &state{runner: runner, build: build, ui: ui.New(runner.Stdout)}

	root := &cobra.Command{
		Use:           "homelabctl",
		Short:         "Operate and validate the homelab control plane",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `homelabctl is the repository-aware operator interface for the homelab.

It coordinates Ansible, K3s, kubectl, Helmfile, Terraform, Docker and CI
without hiding the underlying commands. Mutating operations remain explicit.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			s.runner.DryRun = s.dryRun
			if err := validateNonBlank(s.kubeContext, "Kubernetes context"); err != nil {
				return err
			}
			if cmd.Name() == "version" || cmd.Name() == "update" || cmd.Name() == "completion" || cmd.Name() == "help" {
				return nil
			}
			resolved, err := repository.Root(cmd.Context(), s.repoFlag)
			if err != nil {
				return err
			}
			s.root = resolved
			return nil
		},
	}

	root.PersistentFlags().StringVar(&s.repoFlag, "repo-root", "", "repository root (auto-detected by default)")
	root.PersistentFlags().StringVar(&s.kubeContext, "context", "homelab", "kubectl context")
	root.PersistentFlags().BoolVar(&s.dryRun, "dry-run", false, "print external commands without executing them")

	root.AddCommand(newVersionCommand(s))
	root.AddCommand(newUpdateCommand(s, productionSelfUpdateDependencies()))
	root.AddCommand(newDoctorCommand(s))
	root.AddCommand(newSetupCommand(s))
	root.AddCommand(newInventoryCommand(s))
	root.AddCommand(newNodeCommand(s))
	root.AddCommand(newClusterCommand(s))
	root.AddCommand(newTrustCommand(s))
	root.AddCommand(newControlCommand(s))
	root.AddCommand(newDeployCommand(s))
	root.AddCommand(newInfraCommand(s))
	root.AddCommand(newBuildCommand(s))
	root.AddCommand(newDocsCommand(s))
	root.AddCommand(newCICommand(s))
	applyCommandDocumentation(root)

	return root
}

func (s *state) dir(parts ...string) string {
	all := append([]string{s.root}, parts...)
	return filepath.Join(all...)
}

func (s *state) run(ctx context.Context, dir, name string, args ...string) error {
	return s.runner.Run(ctx, dir, name, args...)
}

func (s *state) runEnv(ctx context.Context, dir string, environment map[string]string, name string, args ...string) error {
	return s.runner.RunEnv(ctx, dir, environment, name, args...)
}

func (s *state) output(ctx context.Context, dir, name string, args ...string) (string, error) {
	return s.runner.Output(ctx, dir, name, args...)
}

func (s *state) out() io.Writer { return s.runner.Stdout }

func (s *state) print(format string, args ...any) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.Printf(format, args...)
}

func (s *state) heading(title string) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.Heading(title)
}

func (s *state) status(status ui.Status, label, message string) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.Status(status, label, message)
}

func (s *state) keyValue(key, value string) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.KeyValue(key, value)
}

func (s *state) success(message string) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.Success(message)
}

func (s *state) info(message string) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.Info(message)
}

func (s *state) warning(message string) {
	if s.ui == nil {
		s.ui = ui.New(s.runner.Stdout)
	}
	s.ui.Warning(message)
}
