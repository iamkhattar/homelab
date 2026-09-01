package cli

import "github.com/spf13/cobra"

type commandDocumentation struct {
	long    string
	example string
}

var commandDocumentationByPath = map[string]commandDocumentation{
	"homelabctl": {
		long:    "homelabctl is the repository-aware operator interface for Titan and the homelab. It coordinates native tools, prints external commands for auditability, and keeps mutations behind explicit verbs.",
		example: "  homelabctl doctor\n  homelabctl --dry-run node prepare --check --limit titan",
	},
	"homelabctl setup": {
		long:    "Install the repository's pinned Ansible, documentation, Go module and CI reporting dependencies. Select one environment while iterating, or omit the target to install all four. For the ansible target, --uninstall removes only generated local runtime state and --reset removes then recreates it.",
		example: "  homelabctl setup\n  homelabctl setup ansible\n  homelabctl setup ansible --reset\n  homelabctl setup ansible --uninstall\n  homelabctl setup docs\n  homelabctl setup go\n  homelabctl setup reports",
	},
	"homelabctl inventory": {long: "Create and inspect the private, Git-ignored Ansible inventory used for local and future remote nodes."},
	"homelabctl inventory init": {
		long:    "Copy the committed inventory example to private hosts.yml with mode 0600. The command refuses to overwrite an existing inventory.",
		example: "  homelabctl inventory init",
	},
	"homelabctl inventory show": {
		long:    "Render effective Ansible group membership without contacting any managed node.",
		example: "  homelabctl inventory show",
	},
	"homelabctl inventory check": {
		long:    "Render the inventory graph, then use Ansible's non-mutating ping module to verify SSH and Python connectivity.",
		example: "  homelabctl inventory check\n  homelabctl inventory check --verbose",
	},
	"homelabctl doctor": {
		long:    "Report repository files and native tools used across the complete homelab workflow. Strict mode fails when any reported requirement is missing.",
		example: "  homelabctl doctor\n  homelabctl doctor --strict",
	},
	"homelabctl node": {long: "Bootstrap trust, prepare Debian, inspect host health, and reboot nodes before K3s installation."},
	"homelabctl node prepare": {
		long:    "Apply the homelab Debian baseline: package updates, hostname policy, SSH keys, optional hardening, swap and sleep controls, logs, time, and SSD maintenance. Check mode previews supported changes only.",
		example: "  homelabctl node prepare --check --limit titan --ask-become-pass\n  homelabctl node prepare --limit titan --ask-become-pass",
	},
	"homelabctl node reboot": {
		long:    "Reboot Debian nodes and wait for managed connectivity to return. Use cluster reboot after K3s is installed.",
		example: "  homelabctl node reboot --limit titan --ask-become-pass",
	},
	"homelabctl node connect": {
		long:    "Open interactive SSH using address, port, user, and optional identity file resolved natively from the private YAML inventory. This bootstrap path does not require Ansible.",
		example: "  homelabctl node connect titan",
	},
	"homelabctl node authorize-key": {
		long:    "Validate and install one OpenSSH public key during first-node bootstrap. The command resolves the host natively from the private YAML inventory and delegates password-authenticated installation to ssh-copy-id without requiring Ansible.",
		example: "  homelabctl node authorize-key titan \\\n    --public-key \"$HOME/.ssh/homelab_titan_ed25519.pub\"",
	},
	"homelabctl node diagnose": {
		long:    "Collect read-only hostname, service, SSH, disk, and time diagnostics through the managed Ansible connection.",
		example: "  homelabctl node diagnose --limit titan --ask-become-pass",
	},
	"homelabctl cluster": {long: "Install, inspect, back up, upgrade, and safely reboot the K3s cluster described by inventory."},
	"homelabctl cluster bootstrap": {
		long:    "Apply the Debian baseline, install or reconcile the inventory-pinned K3s version through the official collection, configure kubeconfig, and wait for node readiness.",
		example: "  homelabctl cluster bootstrap --ask-become-pass",
	},
	"homelabctl cluster upgrade": {
		long:    "Run the upstream controlled upgrade to the exact k3s_version in inventory, then wait for all nodes to become Ready. Export recovery material first.",
		example: "  homelabctl cluster recovery export --destination /secure/homelab --name pre-upgrade --ask-become-pass\n  homelabctl cluster upgrade --ask-become-pass",
	},
	"homelabctl cluster reboot": {
		long:    "Use the upstream cluster-aware reboot order and health checks. A single-node cluster is unavailable while Titan restarts.",
		example: "  homelabctl cluster reboot --ask-become-pass",
	},
	"homelabctl cluster status": {
		long:    "Show node details and, by default, only pods outside Running or Succeeded phases for the selected Kubernetes context.",
		example: "  homelabctl cluster status\n  homelabctl cluster status --all-pods",
	},
	"homelabctl cluster nodes": {
		long:    "List Kubernetes nodes with addresses, versions, roles, and labels for the selected context.",
		example: "  homelabctl cluster nodes\n  homelabctl --context homelab cluster nodes",
	},
	"homelabctl cluster diagnose": {
		long:    "Collect a fixed read-only evidence bundle from K3s, systemd, and Kubernetes without resetting or repairing the cluster.",
		example: "  homelabctl cluster diagnose --ask-become-pass",
	},
	"homelabctl cluster snapshot": {long: "List or create embedded-etcd snapshots on the K3s server."},
	"homelabctl cluster snapshot list": {
		long:    "List embedded-etcd snapshots known to K3s on the server group.",
		example: "  homelabctl cluster snapshot list --ask-become-pass",
	},
	"homelabctl cluster snapshot save": {
		long:    "Create an on-demand embedded-etcd snapshot with a validated, recognisable name prefix.",
		example: "  homelabctl cluster snapshot save --name before-maintenance --ask-become-pass",
	},
	"homelabctl cluster recovery": {long: "Create and move the minimum K3s bootstrap recovery set off the cluster."},
	"homelabctl cluster recovery export": {
		long:    "Create a new snapshot and fetch it with the K3s server token into a unique private directory outside the repository. The destination is staging space and must be encrypted and moved off-device.",
		example: "  homelabctl cluster recovery export \\\n    --destination /secure/homelab-recovery \\\n    --name first-install \\\n    --ask-become-pass",
	},
	"homelabctl control": {long: "Use Butler's versioned API through a private Kubernetes port-forward. Normal commands require a Pocket ID token; bootstrap and recovery use a short-lived, audience-bound Kubernetes service-account token."},
	"homelabctl control bootstrap": {
		long:    "Advance Butler's idempotent Vault bootstrap state machine through the isolated recovery service. The operation requires explicit confirmation and can optionally import a Pocket ID management API key from a local file directly into Vault.",
		example: "  homelabctl control bootstrap --confirm\n  homelabctl control bootstrap --confirm --pocket-id-api-key-file /secure/pocket-id-api-key",
	},
	"homelabctl control recovery": {
		long:    "Inspect the no-ingress recovery service using a freshly issued, ten-minute Kubernetes token. This path remains available when Pocket ID, VSO, normal Butler, or Vault authentication is unavailable.",
		example: "  homelabctl control recovery",
	},
	"homelabctl control recovery export": {
		long:    "Read the named Vault initialization Secret into memory and write only an age-encrypted, mode-0600 bundle outside the repository. Existing output files are never overwritten.",
		example: "  homelabctl control recovery export --output /secure/titan-vault.age --age-recipient age1example",
	},
	"homelabctl control status": {
		long:    "List Butler domain reconcilers and their most recent outcome through the Pocket ID-protected normal API.",
		example: "  BUTLER_TOKEN=... homelabctl control status",
	},
	"homelabctl control operations": {
		long:    "List bounded asynchronous operation metadata. Butler never stores request bodies, provider credentials, or issued tokens in operation records.",
		example: "  BUTLER_TOKEN=... homelabctl control operations",
	},
	"homelabctl control events": {
		long:    "List bounded audit-safe control-plane events with actor, type, message, operation and timestamp fields.",
		example: "  BUTLER_TOKEN=... homelabctl control events",
	},
	"homelabctl control users": {long: "Manage non-administrator Pocket ID user lifecycle and group membership through Butler's identity domain."},
	"homelabctl control groups": {
		long:    "List Pocket ID authorization groups and their stable IDs before assigning user membership through Butler.",
		example: "  BUTLER_TOKEN=... homelabctl control groups",
	},
	"homelabctl control users list": {
		long:    "List Pocket ID users through Butler without exposing passkeys or management credentials.",
		example: "  BUTLER_TOKEN=... homelabctl control users list",
	},
	"homelabctl control users create": {
		long:    "Create a non-administrator Pocket ID user. Butler deliberately rejects administrator creation and promotion to reduce owner-lockout and privilege-escalation risk.",
		example: "  BUTLER_TOKEN=... homelabctl control users create --username sam --display-name 'Sam' --email sam@example.com",
	},
	"homelabctl control users update": {
		long:    "Update the complete Pocket ID user record or disable the account. Pocket ID requires the username on updates, so pass the current value explicitly.",
		example: "  BUTLER_TOKEN=... homelabctl control users update USER_ID --username sam --display-name 'Sam' --disabled",
	},
	"homelabctl control users set-groups": {
		long:    "Replace one Pocket ID user's complete group membership set using provider group IDs. An empty list removes all managed memberships.",
		example: "  BUTLER_TOKEN=... homelabctl control users set-groups USER_ID --group GROUP_ID",
	},
	"homelabctl control clients": {long: "Inspect reconciled Pocket ID OIDC clients and explicitly rotate confidential client credentials without returning secret values."},
	"homelabctl control clients list": {
		long:    "List Pocket ID OIDC client metadata through Butler. Generated client-secret values are never returned.",
		example: "  BUTLER_TOKEN=... homelabctl control clients list",
	},
	"homelabctl control clients rotate": {
		long:    "Rotate one confidential Pocket ID OIDC client secret and write the one-time value directly to its Vault oauth path. The CLI receives only a success status.",
		example: "  BUTLER_TOKEN=... homelabctl control clients rotate grafana",
	},
	"homelabctl deploy": {long: "Preview or reconcile Helmfile desired state. Deployment remains separate from image publication and CI. The shared image tag defaults to the full committed Git SHA and must already exist in the registry."},
	"homelabctl deploy diff": {
		long:    "Run Helmfile diff from cluster/ without applying releases. Select either one validated release name or one dependency stage for a bounded preview.",
		example: "  homelabctl deploy diff\n  homelabctl deploy diff cert-manager\n  homelabctl deploy diff --stage secrets",
	},
	"homelabctl deploy apply": {
		long:    "Apply changed Helmfile releases, optionally selecting one validated release name or one dependency stage. A release and --stage are mutually exclusive.",
		example: "  homelabctl deploy apply\n  homelabctl deploy apply cert-manager\n  homelabctl deploy apply --stage data",
	},
	"homelabctl deploy sync": {
		long:    "Force declared Helmfile releases to desired state without diff gating. Select either one validated release name or one dependency stage to bound reconciliation. Prefer diff followed by apply for routine changes.",
		example: "  homelabctl deploy sync\n  homelabctl deploy sync traefik\n  homelabctl deploy sync --stage networking",
	},
	"homelabctl infra":          {long: "Expose read-only Terraform checks and planning for the optional Hetzner layer. Apply and destroy remain intentionally unavailable."},
	"homelabctl infra fmt":      {long: "Check Terraform formatting recursively without rewriting source files.", example: "  homelabctl infra fmt"},
	"homelabctl infra validate": {long: "Initialise Terraform with the backend disabled, then validate configuration.", example: "  homelabctl infra validate"},
	"homelabctl infra plan":     {long: "Create a Terraform plan using the configured backend and provider credentials without applying it.", example: "  homelabctl infra plan"},
	"homelabctl build":          {long: "Build service, homelabctl, and documentation container artifacts using repository conventions."},
	"homelabctl build services": {
		long:    "Discover the top-level Butler image and Dockerfiles under services/, then build all or explicitly named images. Changed mode selects images from a Git comparison; pushing requires CI.",
		example: "  homelabctl build services --tag dev\n  homelabctl build services api --registry example\n  homelabctl build services --changed --base origin/main --tag revision",
	},
	"homelabctl build docs": {
		long:    "Build the isolated VitePress-to-Nginx documentation image from the docs/ Docker context. Pushing requires CI.",
		example: "  homelabctl build docs --tag dev\n  homelabctl build docs --image example/homelab-docs --tag revision",
	},
	"homelabctl build homelabctl": {
		long:    "Build the non-root homelabctl operator image from the isolated homelabctl/ Docker context. The image contains the CLI, Git, SSH, and trusted CA certificates; pushing requires CI.",
		example: "  homelabctl build homelabctl --tag dev\n  homelabctl build homelabctl --image example/homelabctl --tag revision",
	},
	"homelabctl docs":         {long: "Install, develop, build, preview, and locally serve the isolated VitePress documentation site."},
	"homelabctl docs setup":   {long: "Install exact documentation dependencies from docs/package-lock.json.", example: "  homelabctl docs setup"},
	"homelabctl docs dev":     {long: "Start the VitePress development server from the isolated docs project.", example: "  homelabctl docs dev"},
	"homelabctl docs build":   {long: "Create the static production documentation site and fail on rendering errors.", example: "  homelabctl docs build"},
	"homelabctl docs preview": {long: "Serve the already built VitePress production output for local verification.", example: "  homelabctl docs preview"},
	"homelabctl docs serve": {
		long:    "Run a built documentation container locally, publishing its unprivileged port 8080 on a validated workstation port.",
		example: "  homelabctl docs serve --image iamkhattar/homelab-docs:dev --port 8080",
	},
	"homelabctl ci": {long: "Provide the same high-level validation and image workflows used by GitHub Actions."},
	"homelabctl ci check": {
		long:    "Run repository checks while aggregating failures. Reporting mode replaces plain Go tests with JUnit/JSON output and adds gosec SARIF, Trivy SARIF and an SPDX SBOM. Select or skip named areas when iterating; --only and --skip are mutually exclusive.",
		example: "  homelabctl ci check\n  homelabctl ci check --reports\n  homelabctl ci check --only go-format,go-test\n  homelabctl ci check --reports --only gosec,trivy,sbom",
	},
	"homelabctl ci build": {
		long:    "Build every service image plus the homelabctl and docs images without pushing. Changed mode limits service images while always building homelabctl and docs.",
		example: "  homelabctl ci build --tag dev\n  homelabctl ci build --changed --base origin/main --tag revision",
	},
	"homelabctl ci publish": {
		long:    "Build and push service, homelabctl, and documentation images. This command requires CI and defaults to the current Git commit SHA when no tag is supplied.",
		example: "  CI=true homelabctl ci publish --changed --base HEAD~1 --tag latest --tag revision",
	},
	"homelabctl ci release-tag": {
		long:    "Create and push an annotated semantic release tag at the exact checked-out commit, or verify an existing tag resolves there. The command is CI-only, authenticates with GITHUB_TOKEN through go-git, and never moves a tag.",
		example: "  CI=true GITHUB_TOKEN=... homelabctl ci release-tag \\\n    --tag v0.1.54 \\\n    --commit 3b6ec87a44312bfce2bb3e7aec2dfd2686255226",
	},
	"homelabctl version": {long: "Print the semantic version, source commit, and build date embedded in the binary.", example: "  homelabctl version"},
	"homelabctl update": {
		long:    "Resolve a compatible immutable release from GitHub, require its checksums.txt asset, verify the downloaded archive, and atomically replace the running executable. This command does not require a repository checkout.",
		example: "  homelabctl update --check\n  sudo homelabctl update\n  sudo homelabctl update --version v0.1.42",
	},
}

func applyCommandDocumentation(root *cobra.Command) {
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if documentation, ok := commandDocumentationByPath[command.CommandPath()]; ok {
			command.Long = documentation.long
			command.Example = documentation.example
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}
