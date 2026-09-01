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
| `setup [all\|ansible\|docs\|go\|reports]` | Install exact repository dependencies and reporting tools | `setup ansible --reset` recreates generated runtime; `setup ansible --uninstall` removes it; the flags are mutually exclusive |
| `doctor [--strict]` | Report tools and repository files | Strict mode fails on every missing or unsupported item |
| `inventory init` | Create private `hosts.yml` with mode `0600` | Refuses to overwrite an existing inventory |
| `inventory show` | Render effective group membership | Does not contact nodes |
| `inventory check [-v]` | Render inventory and run Ansible ping | Native inventory parsing and SSH identity checks remain active |
| `version` | Print version, commit and build date | Does not require a repository |
| `update` | Check or install a verified GitHub Release | Does not require a repository; supports Linux/macOS on amd64/arm64 |
| `completion SHELL` | Generate Cobra completion | Does not require a repository |

Examples:

```bash
homelabctl setup ansible
homelabctl setup ansible --reset
homelabctl setup ansible --uninstall
homelabctl setup go
homelabctl setup reports
homelabctl inventory init
homelabctl inventory show
homelabctl inventory check --verbose
homelabctl doctor --strict
homelabctl update --check
```

`update --version v0.1.42` selects an exact semantic release, including an
older rollback target. `--force` reinstalls an already-running version. Global
dry-run performs release discovery but suppresses replacement. See [releases
and updates](/homelabctl/releases-update) for checksum and ownership
details.

## Debian nodes

| Command | Purpose | Important validation |
| --- | --- | --- |
| `node connect HOST` | Open inventory-aware interactive SSH without Ansible | Native resolution validates the host, connection values and port before invoking `ssh` |
| `node authorize-key HOST --public-key PATH` | Bootstrap one operator public key without Ansible | File must contain a supported OpenSSH public-key type and valid base64 data; private-key files fail |
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

## Butler control plane

All control-plane commands use Butler's versioned JSON API. Normal commands
default to the private `https://butler.6940469.xyz` ingress and its publicly
trusted certificate. `--address` overrides that target and `--port-forward`
selects an authenticated, loopback-only Kubernetes tunnel for diagnostics.
Recovery remains non-ingressed and uses its tunnel by default. `control login`
obtains a short-lived Pocket ID ID token with Authorization Code and PKCE and
stores it in the private user config directory. `--token` or `BUTLER_TOKEN` are
non-persistent overrides.

| Command | Purpose | Safety boundary |
| --- | --- | --- |
| `control login`, `logout` | Create or remove the local Pocket ID session | Fixed loopback callback, PKCE, issuer/audience/state/nonce validation and private file permissions |
| `control recovery` | Read bootstrap and Vault lifecycle status | Uses a 10-minute, audience-bound Kubernetes TokenRequest and the non-ingressed recovery service |
| `control recovery ui` | Open the break-glass browser console | Random loopback-only session; CLI injects the recovery token and the browser never receives it |
| `control bootstrap --confirm` | Advance the resumable Vault bootstrap | Explicit confirmation is mandatory; secrets are never returned |
| `control verify-identity --confirm` | Prove Pocket ID access to Butler and Vault before accepting bootstrap | Requires Butler admin; the temporary Vault token is verified, revoked and never sent to Butler |
| `control recovery export` | Write the recovery Secret directly to an age-encrypted off-repository file | Refuses repository and filesystem-root destinations and existing output files |
| `control status`, `operations`, `events` | Inspect reconciliation and audit-safe activity | Viewer access; no request bodies or secret values are recorded |
| `control users ...`, `groups`, `clients ...` | Manage Pocket ID identities and rotate client secrets | Mutations require Butler admin; rotated secrets go directly to Vault |
| `control credentials issue` | Issue an approved short-lived Kubernetes token | Admin-only; role, namespace and maximum TTL are server-side; default output is an `ExecCredential` |

```bash
# One-time private bootstrap. Butler generates Pocket ID's machine credential
# directly in Vault; no API key is passed on the command line.
homelabctl control recovery
homelabctl control recovery ui
homelabctl control certificate status
homelabctl control certificate verify-dns --confirm
homelabctl control bootstrap --confirm
homelabctl control recovery export \
  --output /secure/butler-vault-init.age \
  --age-recipient age1example...

# Normal Pocket ID-authenticated administration.
homelabctl control login
homelabctl control verify-identity --confirm
homelabctl control status
homelabctl control users list
homelabctl control users set-groups USER_ID --group GROUP_ID
homelabctl control clients rotate CLIENT_ID
homelabctl control credentials issue --role homelab-viewer --ttl 15m
homelabctl control logout
```

`control certificate status` prints only the CNAME host/target and readiness;
the acme-dns username and password never cross the recovery API. DNS acceptance
requires `control certificate verify-dns --confirm` and an exact public match.

## Deployment and infrastructure

| Command | Purpose | Validation and mutation |
| --- | --- | --- |
| `deploy diff [release] [--stage stage] [--image-tag tag]` | Preview all changes, one release or one stage | Read-only; release/stage are exclusive and the image tag defaults to the committed Git SHA |
| `deploy apply [release] [--stage stage] [--image-tag tag]` | Apply all changes, one release or one stage | Release/stage are exclusive; selected releases include declared Helmfile dependencies |
| `deploy sync [release] [--stage stage] [--image-tag tag]` | Force reconciliation of all releases, one release or one stage without diff gating | Release/stage are exclusive; selected releases include declared Helmfile dependencies |
| `deploy sync [--image-tag tag]` | Reconcile every release without diff gating | Mutating; intentionally explicit; image tag must already be published |
| `deploy platform --through stage --confirm` | Apply stages in the supported dependency order | Defaults through identity and refuses later stages until Butler bootstrap is `operational` |
| `infra fmt` | Check Terraform formatting | Does not rewrite files |
| `infra validate` | Backend-free init and validation | May download providers |
| `infra plan` | Query and plan configured infrastructure | Does not apply; may access backend/provider APIs |

Terraform apply and destroy are deliberately absent. The inherited cloud-init
path remains unsafe until token delivery and Tailscale networking are replaced.

The ordered stages are `foundation`, `networking`, `secrets`, `identity`,
`data`, `observability`, `cicd`, `applications` and opt-in `smart-home`. A first installation stops
after identity, runs `control bootstrap`, enrolls the Pocket ID owner, runs
`control login` and `control verify-identity`, then continues through the
remaining stages. `--recovery-address` can target an explicitly private
recovery endpoint; otherwise the CLI creates a loopback-only port-forward.

## Builds and documentation

| Command | Purpose | Important validation |
| --- | --- | --- |
| `build services [service...]` | Build discovered or selected service images | Tags follow Docker's 1–128 character tag shape; registry cannot be blank or contain whitespace |
| `build homelabctl` | Build the non-root operator image | Image cannot be blank; the current Git SHA is embedded; pushing requires `CI` |
| `build docs` | Build the VitePress Nginx image | Image cannot be blank or contain whitespace; tags are validated |
| `docs setup` | Install locked docs dependencies | Runs only inside `docs/` |
| `docs dev` | Start VitePress development | Node version guard runs first |
| `docs build` | Render the production site | Rendering failures return non-zero |
| `docs preview` | Preview production output | Requires a completed build |
| `docs serve` | Run the docs container locally | Image is non-blank; port must be 1–65535 |

`--changed` and explicit service names are mutually exclusive. Changed mode
requires a non-empty `--base`. Any push through a build primitive requires the
`CI` environment marker. When `--tag` is omitted, service, homelabctl, docs and
aggregate CI builds resolve the full current Git commit SHA. An explicit
`--tag dev` remains useful for local build-and-serve loops.

```bash
homelabctl build services --tag dev
homelabctl build homelabctl --tag dev
homelabctl build docs --tag dev
homelabctl docs build
homelabctl docs serve --image iamkhattar/homelab-docs:dev --port 8080
```

## CI orchestration

| Command | Purpose | Important validation |
| --- | --- | --- |
| `ci check` | Aggregate checks and optionally generate CI reports | `--reports` writes test/SARIF/SBOM evidence and adds gosec, Trivy and SBOM checks |
| `ci build` | Build all service, homelabctl and docs images without pushing | Uses the same tag, registry, image and Git-base validation as direct builds |
| `ci publish` | Build and push the complete image set | Requires `CI`; defaults to the current Git SHA when tags are omitted |
| `ci release-tag` | Create or verify the annotated tag used by GoReleaser | Requires `CI`, `GITHUB_TOKEN`, a semantic `--tag` and the full checked-out `--commit`; never moves a tag |

```bash
homelabctl ci check --only go-format,go-test
homelabctl ci check --only workflows
homelabctl ci check --reports
homelabctl ci check --reports --only gosec,trivy,sbom
homelabctl ci build --changed --base origin/main --tag revision
CI=true homelabctl ci publish
CI=true homelabctl ci publish \
  --changed \
  --base HEAD~1 \
  --tag v0.1.42 \
  --tag latest \
  --tag revision
CI=true GITHUB_TOKEN=... homelabctl ci release-tag \
  --tag v0.1.42 \
  --commit 0123456789abcdef0123456789abcdef01234567
```

Publication is not deployment. CI never gains an implicit path to mutate Titan;
cluster reconciliation remains under the top-level `deploy` commands.

The first tag is the version embedded into Go service and homelabctl binaries;
aggregate builds therefore share one version. The SHA default is immutable and
reproducible. Mutable names such as `latest` are never added implicitly; request
them with an explicit repeated `--tag`.

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
