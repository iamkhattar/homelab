# Homelab

Home-first infrastructure for a private, single-node K3s cluster on `titan`, an
AMD mini PC running Debian. The repository is designed so that additional
Hetzner workers can be introduced later with explicit labels and taints, after
private Tailscale networking is in place.

[`homelabctl`](docs/homelabctl/index.md) is the supported operator and CI
interface. It validates intent, discovers repository state and delegates to
Ansible, Helmfile, Terraform, Docker and npm without replacing their native
configuration.

The cluster foundation comes before workloads. Vault, Butler, Pocket ID,
Prometheus, Grafana, Loki and Tempo are represented in the repository but must
still pass the staged Titan bootstrap runbook before they count as deployed.
Home Assistant, authenticated MQTT and Zigbee2MQTT are wired as disabled-by-
default increments for the Sonoff zStack coordinator; physical deployment and
restore testing remain later work.

## Repository map

| Path | Responsibility |
| --- | --- |
| [`homelabctl/`](homelabctl/) | Go CLI used locally and by GitHub Actions |
| [`ansible/`](ansible/) | Debian baseline, SSH hardening and K3s lifecycle |
| [`cluster/`](cluster/) | Helmfile-managed cluster foundations and workloads |
| [`infra/`](infra/) | Optional Hetzner infrastructure managed with Terraform |
| [`butler/`](butler/) | Butler control-plane service, API and embedded operator UI |
| [`docs/`](docs/) | Isolated VitePress handbook and internal documentation site |

Desired state remains in the native configuration for each layer. Concrete
provider intent is declared through Butler's `PocketIDClient`,
`ManagedCredential`, and `GarageBucket` Kubernetes APIs beside the chart that
owns it; generated values go directly to Vault. The CLI owns the safe,
repeatable workflow around those files.

## Start here

Install a checksum-verified Linux or macOS binary from [GitHub
Releases](https://github.com/iamkhattar/homelab/releases), then prepare the
operator workstation:

```bash
homelabctl version
homelabctl update --check
homelabctl setup
homelabctl doctor
```

`homelabctl setup` installs the repository's pinned Ansible, documentation, Go
module and reporting dependencies. Contributors can instead build the CLI from
`homelabctl/` with Go 1.27. The documentation toolchain uses Node.js 24 and is
fully contained under `docs/`.

For a new physical machine, follow the [complete Titan setup
runbook](docs/getting-started/titan-setup.md). It covers Debian installation,
the `titan` hostname, DHCP reservation, SSH host verification, operator keys,
Ansible inventory, OS hardening, K3s installation and the first off-node
recovery export.

The normal bootstrap sequence is:

```bash
homelabctl setup ansible
homelabctl inventory init
homelabctl inventory show
homelabctl inventory check
homelabctl node prepare --check --limit titan --ask-become-pass
homelabctl node prepare --limit titan --ask-become-pass
homelabctl cluster bootstrap --limit titan --ask-become-pass
homelabctl cluster status --all-pods
```

Do not copy this sequence past a failed step. Fill in the private inventory and
establish SSH trust first; the runbook explains each acceptance condition.

## Common operations

Use command-specific help for examples and mutation boundaries:

```bash
homelabctl --help
homelabctl node prepare --help
homelabctl cluster recovery export --help
homelabctl deploy diff --help
```

Frequently used workflows include:

```bash
# Validate the repository.
homelabctl ci check

# Preview and reconcile cluster releases.
homelabctl deploy diff
homelabctl deploy apply

# Inspect and maintain K3s.
homelabctl cluster status
homelabctl cluster snapshot save --name before-maintenance --ask-become-pass
homelabctl cluster upgrade --ask-become-pass

# Bootstrap and operate Butler over private port-forwards.
homelabctl control recovery
homelabctl control bootstrap --confirm
homelabctl control login
homelabctl control status

# Update the installed CLI from a verified release.
homelabctl update
```

Terraform `apply` and `destroy` are intentionally absent from homelabctl. The
existing Hetzner layer is retained for future work, but should not be applied
until its networking and secret-delivery model has been redesigned.

## Checks, reports and security scans

The same check orchestrator runs locally and in GitHub Actions:

```bash
homelabctl setup reports
homelabctl ci check --reports
```

Reporting mode runs tests and security checks through homelabctl and writes
portable evidence to fixed repository directories:

| Output | Directory |
| --- | --- |
| JUnit XML and Go test JSON | `test-results/` |
| gosec and Trivy SARIF | `sarif/` |
| SPDX JSON software bill of materials | `sbom/` |

CI retains all three directories as workflow artifacts and submits SARIF to
GitHub Code Scanning. Upload steps run even when a preceding test or scan fails,
so the failure evidence is not lost. Trivy scans vulnerabilities,
misconfigurations and secrets through a digest-pinned container with a
read-only checkout mount.

Limit a local run when investigating one area:

```bash
homelabctl ci check --only workflows
homelabctl ci check --reports --only go-test,gosec
homelabctl ci check --reports --only trivy,sbom
```

## Builds, releases and deployment

When `--tag` is omitted, builds use the full Git commit SHA. Main-branch
publication gives homelabctl and Butler the same semantic version and publishes
the SHA and explicitly requested aliases alongside it.

```bash
homelabctl build services
homelabctl build homelabctl --tag dev
homelabctl build docs --tag dev
homelabctl ci build --changed --base origin/main
```

Image publication is not deployment. CI can build and publish images and create
checksum-protected homelabctl releases, but it has no implicit path to mutate
Titan. Cluster changes remain explicit `homelabctl deploy` operations.

## Documentation

The [`docs/`](docs/index.md) site is the source of truth for installation,
operation and development procedures. Useful entry points are:

- [Titan setup runbook](docs/getting-started/titan-setup.md)
- [Ansible design and lifecycle](docs/ansible/index.md)
- [homelabctl manual](docs/homelabctl/index.md)
- [Releases and updates](docs/homelabctl/releases-update.md)
- [Deployments, builds and CI](docs/homelabctl/deploy-build-ci.md)
- [Current project state](docs/project/current-state.md)
- [Roadmap](docs/project/roadmap.md)

Run the site through homelabctl from the repository root:

```bash
homelabctl docs setup
homelabctl docs dev
homelabctl docs build
homelabctl docs preview
```

Build and serve its unprivileged Nginx image with:

```bash
homelabctl build docs --tag dev
homelabctl docs serve --image iamkhattar/homelab-docs:dev --port 8080
```

## Safety model

- Secrets, private SSH keys, kubeconfig and recovery exports do not belong in
  the repository.
- Vault is the application-secret source of truth; it cannot replace host SSH
  trust, Debian accounts, sudo policy or off-node recovery material.
- Titan stays private on the home network with no router port forwards.
- Mutating commands support narrowly scoped targets where the underlying tool
  permits them; use previews before host or cluster changes.
- A single-node cluster has unavoidable downtime during host reboot and some
  upgrades. Create and export a fresh snapshot before maintenance.

See the [safety and execution
model](docs/homelabctl/safety-internals.md) for subprocess, dry-run, secret and
validation boundaries.
