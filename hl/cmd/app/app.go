package app

import (
	"fmt"
	"os"
	osExec "os/exec"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	hlexec "github.com/iamkhattar/homelab/hl/internal/exec"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var Cmd = &cobra.Command{
	Use:   "app",
	Short: "Interact with deployed applications",
	Long: `Generic interface for all deployed services. Use app subcommands to list,
inspect, tail logs, restart, port-forward, or exec into any application
running on the cluster.`,
}

// Namespace is exported for testing.
var Namespace string

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all apps with status and endpoints",
	Long:    "List all deployments across the cluster, or within a specific namespace.",
	Example: `  hl app list
  hl app list -n applications`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Heading.Render("Applications"))
		fmt.Println()
		kubectlArgs := []string{"get", "deployments", "-o", "wide"}
		if Namespace != "" {
			kubectlArgs = append(kubectlArgs, "-n", Namespace)
		} else {
			kubectlArgs = append(kubectlArgs, "--all-namespaces")
		}
		return hlexec.Kubectl(kubectlArgs...)
	},
}

var statusCmd = &cobra.Command{
	Use:     "status <app>",
	Short:   "Show detailed status for an app",
	Long:    "Show pods, events, and detailed status for a specific application.",
	Example: "  hl app status vault",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		fmt.Println(ui.Heading.Render(fmt.Sprintf("Status: %s", app)))
		fmt.Println()

		ns := ResolveNamespace()

		ui.Step("Pods")
		_ = hlexec.Kubectl("get", "pods", "-n", ns, "-l", fmt.Sprintf("app.kubernetes.io/name=%s", app), "-o", "wide")
		fmt.Println()

		ui.Step("Events")
		return hlexec.Kubectl("get", "events", "-n", ns, "--field-selector", fmt.Sprintf("involvedObject.name=%s", app), "--sort-by=.lastTimestamp")
	},
}

var logsCmd = &cobra.Command{
	Use:     "logs <app>",
	Short:   "Tail logs for an app",
	Long:    "Stream the last 100 lines of logs and follow new output for an application.",
	Example: "  hl app logs grafana",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		ns := ResolveNamespace()
		ui.Step(fmt.Sprintf("Tailing logs for %s", ui.Bold.Render(app)))
		return hlexec.Kubectl("logs", "-n", ns, "-l", fmt.Sprintf("app.kubernetes.io/name=%s", app), "--tail=100", "-f")
	},
}

var restartCmd = &cobra.Command{
	Use:     "restart <app>",
	Short:   "Rollout restart an app",
	Long:    "Trigger a rolling restart of the specified deployment.",
	Example: "  hl app restart homeassistant",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		ns := ResolveNamespace()
		ui.Step(fmt.Sprintf("Restarting %s", ui.Bold.Render(app)))
		if err := hlexec.Kubectl("rollout", "restart", fmt.Sprintf("deployment/%s", app), "-n", ns); err != nil {
			ui.StepFail(fmt.Sprintf("Failed to restart %s", app))
			return err
		}
		ui.StepDone(fmt.Sprintf("%s restarted", ui.Bold.Render(app)))
		return nil
	},
}

var forwardCmd = &cobra.Command{
	Use:   "forward <app> [port]",
	Short: "Port-forward to an app and open browser",
	Long:  "Set up a kubectl port-forward to a service. Defaults to port 8080 if not specified.",
	Example: `  hl app forward vault
  hl app forward grafana 3000`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		port := "8080"
		if len(args) == 2 {
			port = args[1]
		}
		ns := ResolveNamespace()
		ui.Step(fmt.Sprintf("Forwarding %s → localhost:%s", ui.Bold.Render(app), port))
		return hlexec.Kubectl("port-forward", fmt.Sprintf("svc/%s", app), fmt.Sprintf("%s:%s", port, port), "-n", ns)
	},
}

var execCmd = &cobra.Command{
	Use:   "exec <app> [-- cmd...]",
	Short: "Exec into an app pod",
	Long:  "Open an interactive shell in a pod, or run a specific command. Defaults to /bin/sh if no command is given.",
	Example: `  hl app exec vault
  hl app exec postgres -- psql -U admin`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		ns := ResolveNamespace()

		execArgs := []string{"exec", "-it", fmt.Sprintf("deployment/%s", app), "-n", ns}
		if len(args) > 1 {
			execArgs = append(execArgs, args[1:]...)
		} else {
			execArgs = append(execArgs, "--", "/bin/sh")
		}

		ui.Step(fmt.Sprintf("Exec into %s", ui.Bold.Render(app)))
		c := osExec.Command("kubectl", execArgs...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

// ResolveNamespace returns the namespace to use, checking the flag first,
// then the config, and falling back to "default".
func ResolveNamespace() string {
	if Namespace != "" {
		return Namespace
	}
	ns := viper.GetString("cluster.namespace")
	if ns != "" {
		return ns
	}
	return "default"
}

func init() {
	Cmd.PersistentFlags().StringVarP(&Namespace, "namespace", "n", "", "Kubernetes namespace (defaults to config value)")

	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(logsCmd)
	Cmd.AddCommand(restartCmd)
	Cmd.AddCommand(forwardCmd)
	Cmd.AddCommand(execCmd)
}
