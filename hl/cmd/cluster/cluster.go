package cluster

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	hlexec "github.com/iamkhattar/homelab/hl/internal/exec"
	"github.com/iamkhattar/homelab/hl/internal/ui"
)

var Cmd = &cobra.Command{
	Use:   "cluster",
	Short: "Node & cluster management",
	Long:  "Commands for inspecting cluster health, managing nodes, fetching kubeconfig, and bootstrapping nodes via Ansible.",
}

var statusCmd = &cobra.Command{
	Use:     "status",
	Short:   "Show cluster health overview",
	Long:    "Display the status of all nodes in the cluster.",
	Example: "  hl cluster status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Heading.Render("Cluster Status"))
		fmt.Println()
		return hlexec.Kubectl("get", "nodes", "-o", "wide")
	},
}

var nodesCmd = &cobra.Command{
	Use:     "nodes",
	Short:   "List cluster nodes",
	Long:    "List all nodes with roles, IPs, status, and labels.",
	Example: "  hl cluster nodes",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(ui.Heading.Render("Cluster Nodes"))
		fmt.Println()
		return hlexec.Kubectl("get", "nodes", "-o", "wide", "--show-labels")
	},
}

var kubeconfigCmd = &cobra.Command{
	Use:     "kubeconfig",
	Short:   "Fetch and configure kubeconfig",
	Long:    "Display the current kubeconfig path and context from CLI configuration.",
	Example: "  hl cluster kubeconfig",
	RunE: func(cmd *cobra.Command, args []string) error {
		kubeconfig := viper.GetString("cluster.kubeconfig")
		if kubeconfig == "" {
			return fmt.Errorf("cluster.kubeconfig not set — run %s first", ui.Bold.Render("hl config init"))
		}
		ui.KeyValue("Kubeconfig", kubeconfig)
		ui.KeyValue("Context", viper.GetString("cluster.context"))
		return nil
	},
}

var sshCmd = &cobra.Command{
	Use:   "ssh <node>",
	Short: "SSH into a node by name",
	Long:  "Open an interactive SSH session to a cluster node.",
	Example: `  hl cluster ssh server-node-0
  hl cluster ssh agent-node-0`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		node := args[0]
		ui.Step(fmt.Sprintf("Connecting to %s", ui.Bold.Render(node)))
		return hlexec.Interactive("ssh", node)
	},
}

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap <inventory>",
	Short: "Run ansible playbook against an inventory",
	Long:  "Execute the site.yml Ansible playbook using the specified inventory file from the ansible/inventory/ directory.",
	Example: `  hl cluster bootstrap inventory-server.yml
  hl cluster bootstrap inventory-agent.yml`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		inventory := args[0]
		ui.Step(fmt.Sprintf("Bootstrapping with inventory %s", ui.Bold.Render(inventory)))
		return hlexec.Run("ansible-playbook", "playbooks/site.yml", "-i", fmt.Sprintf("inventory/%s", inventory))
	},
}

func init() {
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(nodesCmd)
	Cmd.AddCommand(kubeconfigCmd)
	Cmd.AddCommand(sshCmd)
	Cmd.AddCommand(bootstrapCmd)
}
