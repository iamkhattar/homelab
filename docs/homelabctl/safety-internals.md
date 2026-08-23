# Safety and execution model

`homelabctl` is an orchestration boundary, not a replacement configuration
language. Understanding where commands execute makes failures and recovery more
predictable.

## Execution map

| Command family | Execution path | Primary state |
| --- | --- | --- |
| `setup`, `docs` | Local Python/npm processes | Pinned repository dependencies |
| `inventory init` | Direct local file operation | Private Ansible inventory |
| `inventory`, `node` | Local Ansible, SSH and `ssh-copy-id` processes | Debian hosts and inventory |
| `cluster bootstrap/upgrade/reboot` | Local Ansible invoking remote K3s lifecycle | K3s installation and node state |
| `cluster status/nodes` | Local kubectl | Kubernetes API |
| `cluster snapshot/recovery/diagnose` | Local Ansible invoking fixed remote tasks | K3s datastore and evidence |
| `deploy` | Local Helmfile | Cluster workload desired state |
| `infra` | Local Terraform | Optional external infrastructure plan |
| `build` | Local Docker | Container images |
| `ci check` | Go-native repository/workflow checks plus pinned local tools | Repository correctness |

The CLI always sets the intended working directory. Ansible commands prefer
`ansible/.venv/bin` and set repository-local Ansible runtime paths, so shell
activation is unnecessary.

## Go libraries and external tools

Repository plumbing is implemented in-process. `homelabctl` uses `go-git` to
discover and open the checkout, resolve `HEAD`, calculate merge bases and select
changed services. It parses GitHub Actions YAML with the Go YAML library. These
operations do not spawn the `git` executable, so their output is typed,
testable and consistent across local machines and CI. Git is still useful to
clone the repository and for normal developer commits; it is not a runtime
dependency for these CLI operations.

The infrastructure engines intentionally remain external processes:

| Concern | Owner | Why |
| --- | --- | --- |
| Repository SHA, refs, merge-base diff | `go-git` inside `homelabctl` | Stable Go API and no shell parsing |
| Workflow YAML and repository CI policy | Go YAML parser inside `homelabctl` | Fast local validation with focused tests |
| Human terminal presentation | Lip Gloss inside `homelabctl` | Consistent semantic styling with automatic plain-text fallback |
| K3s host lifecycle | Ansible | Inventory, idempotence, privilege escalation and check mode |
| Kubernetes resources | kubectl and Helmfile | Native contexts, plugins, diffs and release semantics |
| Hetzner infrastructure | Terraform | Provider graph, state locking and plans |
| Images | Docker | BuildKit, registry authentication and platform support |
| Documentation dependencies/build | npm and VitePress | Lockfile and frontend toolchain semantics |

Using a Go library is preferred when it cleanly replaces local parsing or
plumbing. Reimplementing Ansible, Terraform, Helmfile or Docker behavior through
SDKs would create a second configuration engine and make native troubleshooting
harder, so `homelabctl` invokes those canonical CLIs with typed arguments and a
fixed working directory.

## Test strategy

The homelabctl suite is split along the same ownership boundaries:

- CLI contract tests execute complete Cobra command paths in dry-run mode and
  assert the constructed Ansible, kubectl, Helmfile, Terraform, Docker and npm
  invocations. They also prove invalid flag combinations fail before a child
  process is prepared.
- command-runner tests execute a controlled helper process to verify working
  directories, environment propagation, output trimming, error wrapping and
  safe argument rendering.
- repository tests use temporary real `go-git` repositories. They cover root
  discovery, unborn repositories, SHA resolution, merge-base diffs,
  uncommitted-file exclusion, and changed service additions, renames and
  deletions.
- workflow tests validate the checked-in workflows and mutated fixtures for
  permissions, triggers, toolchain versions, action pins, timeouts, checkout
  depth, publication ordering and forbidden cluster or infrastructure changes.
- filesystem boundary tests cover private inventory creation, public-key
  parsing and recovery exports that must remain outside the checkout.

Run the supported test entry point from the repository root:

```bash
homelabctl ci check --only go-test
```

CI reporting is also owned by the CLI rather than encoded as scanner arguments
inside workflow YAML. `homelabctl ci check --reports` constructs JUnit, raw test
JSON, gosec SARIF, Trivy SARIF and SPDX output. Trivy receives a read-only
checkout mount plus narrowly writable report and cache mounts. Its SARIF pass
is followed by a cached table-format pass: the table makes findings actionable
in the job log and its exit status enforces the HIGH/CRITICAL gate.

Gosec suppressions must be local, rule-specific and include a justification.
Suppression tracking remains enabled in SARIF. The current exceptions cover the
audited subprocess runner, fixed repository paths and explicit operator-selected
configuration/public-key files; they do not globally disable a rule.

Dry-run tests prove validation and command construction. They deliberately do
not claim that a remote Debian node, registry, cloud account or Kubernetes API
is available; Ansible syntax checks, Terraform tests and explicit operator
runbooks cover the next layers.

## Command visibility

Before execution, each external process is printed with its working directory
and safely quoted arguments. Child processes are created directly without a
shell. This reduces accidental expansion of metacharacters and makes the native
operation reproducible during break-glass diagnosis.

Standard input, output and error remain attached to the terminal so sudo,
Ansible and SSH prompts work normally. A non-zero child exit becomes a non-zero
`homelabctl` exit with the failed executable identified.

## Output contract

Human-facing output goes through one presentation package under `internal/ui`.
Interactive terminals receive a restrained colour palette, aligned labels and
semantic markers:

```text
◆ Repository checks
– SKIP  go-format
◆ RUN   workflows
✓ PASS  workflows
✗ FAIL  go-test: exit status 1
```

Green means completed successfully, cyan means active or informational, amber
means attention is required, red means failure, and muted text means skipped or
supporting context. External commands retain the leading `+` and working
directory so they remain easy to distinguish from CLI status.

Lip Gloss detects the terminal's colour profile and removes ANSI sequences when
output is redirected to a file, pipe, test buffer or CI log. It also follows the
standard environment controls:

```bash
NO_COLOR=1 homelabctl doctor
CLICOLOR_FORCE=1 homelabctl ci check --only workflows
```

Use `NO_COLOR` for accessibility, terminal incompatibility or plain captured
logs. `CLICOLOR_FORCE` is mainly useful while testing presentation; do not use
it for machine parsing. Text labels such as `PASS`, `FAIL`, `RUN`, `SKIP`,
`MISSING` and `OLD` remain present without relying on colour alone.

Output is still a human interface, not a stable data API. Automation should use
exit codes and native structured output where a command explicitly provides
it. A future JSON mode must bypass presentation styles and define its own
versioned schema.

## Mutation boundaries

Read-only or preview operations include:

- `doctor`, `inventory show`, `inventory check` and diagnostics;
- `node prepare --check`;
- `cluster status`, `cluster nodes` and snapshot listing;
- `deploy diff`;
- `infra fmt`, `infra validate` and `infra plan` with respect to managed cloud
  resources, although validation and planning may write local cache data;
- global `--dry-run` command construction.

Explicit mutations include:

- inventory initialisation;
- dependency setup;
- node preparation and reboot;
- cluster bootstrap, upgrade, reboot and snapshot creation;
- recovery export to a local destination;
- deploy apply/sync;
- container builds and pushes.

Not every preview is a semantic dry-run. Ansible check mode, Helm diff,
Terraform plan and global command printing provide different guarantees.

## Secret handling

The CLI is not a secret store. It does not accept sudo passwords, K3s tokens or
Vault recovery keys as persistent configuration. Interactive prompts remain
owned by the underlying tool.

Recovery export is the deliberate exception where secret material passes
through a workflow. It fetches the K3s server token with Ansible output
suppressed, writes it with restrictive permissions, and never prints the token.
The operator remains responsible for encryption, off-device storage and removal
of plaintext staging files.

Do not add secret-bearing generic extra-variable flags or arbitrary remote
execution commands merely for convenience.

## Why native commands remain visible

The CLI prints the underlying operation and repository files remain usable by
their native tools. This is essential when diagnosing the wrapper itself or
following upstream recovery documentation. Normal runbooks still use
`homelabctl` so safety defaults, working directories and pinned dependencies are
consistent.

## Current limitations

- almost every command still requires a repository checkout;
- `--context` represents only a kubectl context, not a complete environment;
- output is human-oriented; stable JSON output is planned;
- updates are operator-triggered rather than an unattended background process;
- deployment has no confirmation or rollback command yet;
- restore remains intentionally manual;
- control-plane authentication and API-backed commands are not implemented.

These limitations should be resolved through typed commands and documented
contracts rather than hidden shell behaviour.
