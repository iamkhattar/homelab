# Command and validation reference

Every runnable `homelabctl` command provides a detailed description, realistic
examples and command-specific flags through Cobra:

```bash
homelabctl --help
homelabctl node prepare --help
homelabctl cluster recovery export --help
homelabctl ci publish --help
```

Help examples use supported operator workflows. They are not placeholders:
copy them only after replacing example paths, revisions, users and addresses
with the intended values.

## Global contract

```text
homelabctl [--repo-root PATH] [--context NAME] [--dry-run] COMMAND
```

| Flag | Validation and effect |
| --- | --- |
| `--repo-root PATH` | Must resolve to a Git repository root; normal runs discover it through Git |
| `--context NAME` | Must be non-empty; currently targets kubectl-backed inspection only |
| `--dry-run` | Prints supported external commands and suppresses supported file creation; it does not validate remote state |
| `--help` | Displays detailed command behaviour, side-effect boundaries, flags and examples |

Argument validation happens before an external process is started. Native tools
still perform authoritative semantic validation of their own configuration.

Interactive status output uses colour and semantic glyphs while retaining text
labels. Redirected output and CI logs automatically become plain text. Set
`NO_COLOR=1` to disable styling explicitly or `CLICOLOR_FORCE=1` to exercise
styled output in a non-terminal. These variables change presentation only;
they never change command behaviour or exit status.

## Workstation and inventory

| Command | Purpose | Important validation |
| --- | --- | --- |
| `setup [all\|ansible\|docs]` | Install exact repository dependencies | `ansible --reset` recreates generated runtime; `ansible --uninstall` removes it; the flags are mutually exclusive |
| `doctor [--strict]` | Report tools and repository files | Strict mode fails on every missing or unsupported item |
| `inventory init` | Create private `hosts.yml` with mode `0600` | Refuses to overwrite an existing inventory |
| `inventory show` | Render effective group membership | Does not contact nodes |
| `inventory check [-v]` | Render inventory and run Ansible ping | Native inventory parsing and SSH identity checks remain active |
| `version` | Print version, commit and build date | Does not require a repository |
| `self-update` | Check or install a verified GitHub Release | Does not require a repository; supports Linux/macOS on amd64/arm64 |
| `completion SHELL` | Generate Cobra completion | Does not require a repository |

Examples:

```bash
homelabctl setup ansible
homelabctl setup ansible --reset
homelabctl setup ansible --uninstall
homelabctl inventory init
homelabctl inventory show
homelabctl inventory check --verbose
homelabctl doctor --strict
homelabctl self-update --check
```

`self-update --version v0.1.42` selects an exact semantic release, including an
older rollback target. `--force` reinstalls an already-running version. Global
dry-run performs release discovery but suppresses replacement. See [releases
and self-update](/homelabctl/releases-self-update) for checksum and ownership
details.

## Debian nodes

| Command | Purpose | Important validation |
| --- | --- | --- |
| `node connect HOST` | Open inventory-aware interactive SSH | Host uses inventory-safe letters, numbers, dots, underscores and hyphens |
| `node authorize-key HOST --public-key PATH` | Bootstrap one operator public key | File must contain a supported OpenSSH public-key type and valid base64 data; private-key files fail |
| `node prepare` | Apply or preview the Debian baseline | An explicitly supplied `--limit` cannot be blank |
| `node diagnose` | Gather read-only host and SSH evidence | Uses the same safe limit handling |
| `node reboot` | Reboot before K3s installation | Uses the same safe limit handling; waits through Ansible |

Common Ansible-backed flags:

| Flag | Meaning |
| --- | --- |
| `--limit HOST_OR_GROUP` | Restrict a play; an explicit empty value is rejected so it cannot accidentally target every node |
| `--ask-become-pass` | Prompt interactively for sudo; the value is never stored by the CLI |
| `node prepare --check` | Adds Ansible check and diff mode; not every package or command task can be predicted |

```bash
homelabctl node authorize-key titan \
  --public-key "$HOME/.ssh/homelab_titan_ed25519.pub"
homelabctl node prepare --check --limit titan --ask-become-pass
homelabctl node prepare --limit titan --ask-become-pass
```

## K3s lifecycle and recovery

| Command | Purpose | Safety boundary |
| --- | --- | --- |
| `cluster bootstrap` | Prepare nodes and install/reconcile pinned K3s | No check mode; first server generates its token |
| `cluster status [--all-pods]` | Show nodes and unhealthy or all pods | Uses global kubectl context |
| `cluster nodes` | List nodes and labels | Read-only Kubernetes request |
| `cluster diagnose` | Gather K3s and Kubernetes evidence | Does not reset or repair |
| `cluster snapshot list` | List embedded-etcd snapshots | Read-only server operation |
| `cluster snapshot save --name NAME` | Create an on-demand snapshot | Name is 1–64 safe characters |
| `cluster recovery export --destination PATH` | Fetch a new snapshot and token | Destination must be non-empty, outside the repository and not a filesystem root |
| `cluster upgrade` | Upgrade to inventory-pinned K3s | Recovery export should exist first |
| `cluster reboot` | Cluster-aware reboot and readiness checks | Single-node downtime is expected |

Snapshot names may contain letters, numbers, dots, underscores and hyphens and
must begin with a letter or number. Recovery exports create a new UTC timestamp
directory with private permissions rather than overwriting an older export.

```bash
homelabctl cluster status --all-pods
homelabctl cluster snapshot save \
  --name before-maintenance \
  --ask-become-pass
homelabctl cluster recovery export \
  --destination /secure/homelab-recovery \
  --name first-install \
  --ask-become-pass
```

## Deployment and infrastructure

| Command | Purpose | Validation and mutation |
| --- | --- | --- |
| `deploy diff` | Preview Helmfile changes | Read-only preview |
| `deploy apply [release]` | Apply changed releases | Optional release uses lowercase letters, numbers, dots and hyphens |
| `deploy sync` | Reconcile every release without diff gating | Mutating; intentionally explicit |
| `infra fmt` | Check Terraform formatting | Does not rewrite files |
| `infra validate` | Backend-free init and validation | May download providers |
| `infra plan` | Query and plan configured infrastructure | Does not apply; may access backend/provider APIs |

Terraform apply and destroy are deliberately absent. The inherited cloud-init
path remains unsafe until token delivery and Tailscale networking are replaced.

## Builds and documentation

| Command | Purpose | Important validation |
| --- | --- | --- |
| `build services [service...]` | Build discovered or selected service images | Tags follow Docker's 1–128 character tag shape; registry cannot be blank or contain whitespace |
| `build docs` | Build the VitePress Nginx image | Image cannot be blank or contain whitespace; tags are validated |
| `docs setup` | Install locked docs dependencies | Runs only inside `docs/` |
| `docs dev` | Start VitePress development | Node version guard runs first |
| `docs build` | Render the production site | Rendering failures return non-zero |
| `docs preview` | Preview production output | Requires a completed build |
| `docs serve` | Run the docs container locally | Image is non-blank; port must be 1–65535 |

`--changed` and explicit service names are mutually exclusive. Changed mode
requires a non-empty `--base`. Any push through a build primitive requires the
`CI` environment marker. When `--tag` is omitted, service, docs and aggregate CI
builds resolve the full current Git commit SHA. An explicit `--tag dev` remains
useful for local build-and-serve loops.

```bash
homelabctl build services --tag dev
homelabctl build docs --tag dev
homelabctl docs build
homelabctl docs serve --image iamkhattar/homelab-docs:dev --port 8080
```

## CI orchestration

| Command | Purpose | Important validation |
| --- | --- | --- |
| `ci check` | Aggregate Go, docs, workflow, Ansible and Terraform checks | `--only` and `--skip` are mutually exclusive and accept known check names only |
| `ci build` | Build all service and docs images without pushing | Uses the same tag, registry, image and Git-base validation as direct builds |
| `ci publish` | Build and push the complete image set | Requires `CI`; defaults to the current Git SHA when tags are omitted |

```bash
homelabctl ci check --only go-format,go-test
homelabctl ci check --only workflows
homelabctl ci build --changed --base origin/main --tag revision
CI=true homelabctl ci publish
CI=true homelabctl ci publish \
  --changed \
  --base HEAD~1 \
  --tag latest \
  --tag revision
```

Publication is not deployment. CI never gains an implicit path to mutate Titan;
cluster reconciliation remains under the top-level `deploy` commands.

The SHA default is immutable and reproducible. Mutable names such as `latest`
are never added implicitly; request them with an explicit repeated `--tag`.

## Validation ownership

The CLI validates operator intent that can be checked without contacting an
external system:

- required and mutually exclusive flags;
- safe names, tags, ports, contexts and paths;
- public-key file shape;
- CI-only publication boundaries;
- recovery output placement;
- unknown inventory services, checks and setup targets.

Ansible, kubectl, Helmfile, Terraform, Docker and npm remain responsible for
parsing and validating their native desired-state files. A successful CLI
validation means the request is well-formed, not that the remote operation will
succeed.
