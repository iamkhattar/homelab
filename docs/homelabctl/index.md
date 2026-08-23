# homelabctl

`homelabctl` is the repository’s operator library and command-line interface.
It turns supported homelab workflows into typed Cobra commands, validates local
intent, and delegates execution to the tool that owns each infrastructure
domain.

This section is for two audiences:

- operators use the workflow guides to prepare nodes, manage K3s, build images
  and run checks;
- contributors use the execution model and reference pages to extend the CLI
  without creating a second configuration system.

## Start here

| Goal | Next page |
| --- | --- |
| Install the CLI and prepare a workstation | [Install and configure](/homelabctl/getting-started) |
| Understand releases, Debian installation and self-update | [Releases and self-update](/homelabctl/releases-self-update) |
| Understand safety, subprocesses and Go-library boundaries | [Safety and execution model](/homelabctl/safety-internals) |
| Prepare Titan and establish SSH trust | [Inventory and nodes](/homelabctl/inventory-nodes) |
| Install, inspect, upgrade or recover K3s | [Cluster lifecycle and recovery](/homelabctl/cluster-lifecycle) |
| Build, deploy or understand CI | [Deployments, builds and CI](/homelabctl/deploy-build-ci) |
| Look up exact commands and flags | [Command reference](/homelabctl/command-reference) |

Read the first three pages in sidebar order before adding a command. Workflow
pages assume that execution and secret-handling boundaries are already
understood.

## What the component owns

`homelabctl` owns:

- command names, arguments, flags, examples and operator-facing errors;
- validation that can be completed safely before contacting an external
  system;
- repository discovery, Git revision handling and changed-service selection;
- checksum-verified discovery and installation of its own GitHub Releases;
- fixed working directories and environment passed to external tools;
- dry-run behavior and explicit mutation boundaries;
- the high-level sequence used both locally and by GitHub Actions.

It does not own Ansible inventory semantics, Kubernetes desired state,
Terraform state, Docker builds or npm dependency resolution. Those remain with
their native engines and declarative files.

## Request lifecycle

Every repository-aware command follows the same path:

| Stage | Responsibility | Failure means |
| --- | --- | --- |
| 1. Parse | Cobra selects a typed command and flags | The request is not a supported CLI shape |
| 2. Locate | The repository package opens the checkout | The command is not running against a valid homelab repository |
| 3. Validate | CLI helpers check names, paths and mutation guards | The request is unsafe or incomplete before execution |
| 4. Resolve | Go libraries derive SHA, refs, files or workflow policy | Repository state cannot support the requested operation |
| 5. Execute | The command runner invokes a canonical external tool | Ansible, Docker, Terraform, kubectl, Helmfile or npm rejected or failed the operation |
| 6. Verify | The workflow runs a fixed health or validation step where defined | The requested change did not reach its acceptance condition |

External commands are printed before execution. A child process is created
directly without a shell, so user values are arguments rather than executable
shell text.

## Package map

```text
homelabctl/
├── .goreleaser.yaml         cross-platform release contract
├── cmd/homelabctl/          executable entry point
└── internal/
    ├── cli/                 Cobra commands, validation and workflows
    ├── command/             subprocess execution and dry-run rendering
    ├── repository/          go-git and repository file discovery
    └── workflow/            GitHub Actions YAML and policy validation
```

All implementation packages are under `internal/`; there is no promised public
Go API yet. The supported interface is the CLI contract documented here. A
future reusable client package should be introduced only when the control-plane
API has a stable contract and a second real consumer.

## Tool ownership

| Domain | Implementation used by homelabctl |
| --- | --- |
| Repository root, SHA and merge-base diff | `go-git` in process |
| Workflow parsing and repository CI policy | Go YAML parsing in process |
| CLI release discovery, checksums and atomic replacement | `go-selfupdate` in process |
| Terminal status, colour degradation and plain-text output | Lip Gloss in process |
| Debian and K3s lifecycle | Ansible subprocess |
| Kubernetes inspection | kubectl subprocess |
| Workload reconciliation | Helmfile subprocess |
| Optional cloud planning | Terraform subprocess |
| Container images | Docker subprocess |
| Documentation dependencies and build | npm subprocess |

The default image tag is the full current Git commit SHA. Publication requires
the `CI` environment marker, and CI publication does not imply deployment to
Titan.

## Contributor contract

When adding a workflow:

1. add a typed command instead of documenting a loose native command;
2. validate local intent before preparing a child process;
3. preserve the native tool as the desired-state and semantic authority;
4. add table-driven validation tests and a complete dry-run command test;
5. document the operator procedure separately from command reference;
6. add the command to CI only when local and CI behavior should be identical.

Run the complete acceptance check with:

```bash
homelabctl ci check
```

Continue with [Install and configure](/homelabctl/getting-started) for operator
setup, or [Safety and execution model](/homelabctl/safety-internals) for
implementation details.
