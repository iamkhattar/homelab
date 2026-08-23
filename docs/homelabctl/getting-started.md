# Install and configure homelabctl

`homelabctl` is the supported interface for repository setup and homelab
operations. It is run from the operator workstation, not installed on Titan.

## Prerequisites

The initial workstation needs:

- Git and a checkout of this repository;
- Go 1.27 or newer, matching `homelabctl/go.mod`, only when building or testing the CLI from source;
- Python 3 with virtual-environment support;
- OpenSSH, including `ssh-copy-id`, for first-node trust bootstrap;
- Node.js 24 or newer for documentation development;
- Docker, kubectl, Helm, Helmfile and Terraform for the commands that use them.

`homelabctl doctor` reports the resolved executable paths and missing repository
files. A tool may be optional for one command but required for another; for
example, Terraform is irrelevant to `node prepare` but required by a complete
strict doctor check.

## Bootstrap the binary

Install the checksum-verified release binary into `/usr/local/bin`. The
[release and update guide](/homelabctl/releases-update) gives exact
Linux and macOS asset names, Debian verification commands, update behaviour and
rollback procedure.

Contributors can instead build the current checkout. This is the one bootstrap
exception that cannot go through the CLI itself:

```bash
cd homelabctl
go build -trimpath -o ../bin/homelabctl ./cmd/homelabctl
cd ..
export PATH="$PWD/bin:$PATH"
```

Confirm that the expected binary is active:

```bash
homelabctl version
homelabctl --help
```

The `version` output is `dev` for a local build unless version, commit and build
date are injected at link time. Release builds contain all three values.

## Install shell completion

Completion generation is built into the CLI and does not require a repository.
For Zsh, generate a user-local completion file and ensure that directory is on
the shell's `fpath`:

```bash
mkdir -p "$HOME/.zsh/completions"
homelabctl completion zsh > "$HOME/.zsh/completions/_homelabctl"
```

For Bash or Fish, inspect the shell-specific installation guidance before
choosing the destination:

```bash
homelabctl completion bash --help
homelabctl completion fish --help
```

Completion installation is workstation-owned because shell startup and package
manager conventions differ. It is not part of the Debian host role.

## Install repository dependencies

Install every pinned workstation dependency:

```bash
homelabctl setup
```

The default target is `all`. More focused forms are available:

```bash
homelabctl setup ansible
homelabctl setup docs
homelabctl setup reports
```

The Ansible setup creates `ansible/.venv`, installs
`ansible/requirements.txt`, and installs the collections pinned by
`ansible/requirements.yml` under `ansible/collections`. The docs setup runs the
locked package installation inside `docs/`. Neither environment needs to be
activated manually when using `homelabctl`. Reporting setup installs the pinned
`gotestsum` and `gosec` Go tools. Trivy runs from an immutable digest-pinned
container, so reporting setup does not compile or install its large dependency
graph.

Re-run setup after changing either requirements file or lockfile. Setup never
creates the private node inventory.

To rebuild a suspect Ansible environment or remove it from this checkout:

```bash
homelabctl setup ansible --reset
homelabctl setup ansible --uninstall
```

Both operations preserve private inventory and remote node state. The complete
[reset and uninstall runbook](/ansible/reset-uninstall) explains the boundary
between local dependencies, inventory and changes already applied to Titan.

## Run workstation checks

```bash
homelabctl doctor
homelabctl doctor --strict
```

Without `--strict`, missing items are reported but the command succeeds. Strict
mode returns an error when anything is missing, which is useful before a planned
maintenance window. Go versions older than 1.27 and Node versions older than 24
are reported as unsupported.

## Repository discovery

Normal commands discover the repository root using Git, so they can be invoked
from a subdirectory. Override discovery only when deliberately operating on a
different checkout:

```bash
homelabctl --repo-root /path/to/homelab doctor
```

`version`, `update`, help and shell-completion generation do not require a repository.
All other current commands do. Future control-plane API commands will be able to
run without a checkout.

## Global flags

| Flag | Meaning |
| --- | --- |
| `--repo-root PATH` | Operate on an explicit checkout instead of Git discovery |
| `--dry-run` | Print external commands and suppress supported file or directory creation |
| `--context NAME` | Select the kubectl context for direct cluster inspection; default `homelab` |
| `--help` | Show command-specific usage and flags |

`--context` currently affects kubectl-backed inspection commands. It does not
yet define a complete environment profile for Ansible, Helmfile and the future
control-plane API; that context model remains planned work.

## Preview execution

Use global dry-run before an unfamiliar workflow:

```bash
homelabctl --dry-run node prepare --check --limit titan
homelabctl --dry-run cluster recovery export \
  --destination /path/to/recovery-staging \
  --name rehearsal
```

The CLI prints commands to standard error with their working directory. It
constructs child processes without an intermediate shell, so printed quoting is
for clarity rather than shell evaluation.

Dry-run proves command construction; it does not test network access,
credentials, remote state or whether the underlying tool accepts the operation.

## First operator session

After Debian is installed and Titan has a router reservation:

```bash
homelabctl setup
homelabctl inventory init
homelabctl inventory show
homelabctl node connect titan
homelabctl inventory check
homelabctl node prepare --check --ask-become-pass
```

Stop at the first unexpected result. Continue with [inventory and node
management](/homelabctl/inventory-nodes) before installing K3s.
