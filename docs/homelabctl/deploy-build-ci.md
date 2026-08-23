# Deployments, builds and CI

These commands keep repository validation, artifact creation and cluster
mutation separate. GitHub Actions consumes the same primitives used locally.

## Preview Helm changes

```bash
homelabctl deploy diff
```

This runs Helmfile's diff workflow from `cluster/` and does not apply releases.
Use it before every routine deployment. A clean diff is not a substitute for
application-specific backup or migration checks.

## Apply desired state

Apply every changed release:

```bash
homelabctl deploy apply
```

Select one release by its exact Helmfile release name:

```bash
homelabctl deploy apply cert-manager
```

The optional argument becomes a `name=<release>` Helmfile selector. It does not
select a namespace, chart directory or Kubernetes resource.

`deploy sync` forces every declared release to the desired state without diff
gating:

```bash
homelabctl deploy sync
```

Use sync only when intentionally reconciling the complete environment. Normal
operations should prefer diff followed by apply.

::: warning Current context limitation
The global `--context` flag currently controls kubectl-backed cluster inspection
only. Helmfile uses the context configured by its own desired-state files and
environment. Do not assume `homelabctl --context other deploy apply` retargets a
deployment; unified profiles are future work.
:::

## Terraform safety boundary

The optional Hetzner layer exposes read-only planning commands:

```bash
homelabctl infra fmt
homelabctl infra validate
homelabctl infra plan
```

Their exact behaviour is:

| Command | Operation |
| --- | --- |
| `infra fmt` | Checks Terraform formatting recursively without rewriting files |
| `infra validate` | Initialises provider dependencies with the backend disabled, then validates configuration |
| `infra plan` | Runs a normal plan using the existing backend/provider environment |

These commands do not intentionally change managed cloud resources, but
initialisation and planning can create local `.terraform` cache data, access the
remote backend and query provider APIs. `infra plan` requires the relevant
Terraform credentials and prior backend initialisation.

Apply and destroy are intentionally absent. The inherited cloud-init flow still
needs redesign so K3s tokens cannot enter Terraform state or instance logs.

## Build service images

Build every directory under `services/` that contains a Dockerfile:

```bash
homelabctl build services --tag dev
```

The syntax `homelabctl build services [service...]` accepts one or more exact
directory names when a selected service implementation and image contract are
current. With no names, it discovers and builds every service.

Important flags:

| Flag | Behaviour |
| --- | --- |
| `--tag TAG` | Add an image tag; repeat for multiple tags; the first tag is the embedded build version; defaults to the current full Git commit SHA |
| `--registry NAME` | Set the image namespace; default `iamkhattar` |
| `--changed` | Build services changed between `--base` and `HEAD` |
| `--base REVISION` | Required with `--changed` |
| `--push` | Push every generated image tag; allowed only when `CI` is set |

Explicit service names and `--changed` are mutually exclusive. Unknown service
names fail before Docker is invoked. Use `--tag dev` deliberately when a stable
local-development tag is more convenient than the immutable default.

## Build the homelabctl image

```bash
homelabctl build homelabctl --tag dev
```

The image is built from the isolated `homelabctl/` context and defaults to
`iamkhattar/homelabctl`. Override the complete name or add multiple tags with
the same interface used by the docs image:

```bash
homelabctl build homelabctl \
  --image iamkhattar/homelabctl \
  --tag latest \
  --tag revision-id
```

The multi-stage Dockerfile compiles a static Linux binary with Go 1.27. The
runtime is an Alpine image containing trusted CA certificates, Git and the
OpenSSH client. It runs as the unprivileged UID/GID `65532`, uses `/workspace`
as its working directory, and starts `homelabctl` directly. The current Git SHA
is embedded in the binary and OCI image metadata by the build command.

Run a repository-independent command directly:

```bash
docker run --rm iamkhattar/homelabctl:dev version
```

Repository-aware commands require an explicit checkout mount:

```bash
docker run --rm \
  --volume "$PWD:/workspace" \
  iamkhattar/homelabctl:dev \
  --repo-root /workspace ci check --only workflows
```

This is deliberately a minimal operator image, not yet a privileged CI runner.
It does not contain Ansible, Docker, Helmfile, kubectl or Terraform, and it does
not receive a Docker socket, kubeconfig or cluster-wide credentials by default.
The future runner or Kubernetes operator must add only the tools and narrowly
scoped credentials required by its execution model. Treat the container as
immutable: deploy a newer image tag instead of running `homelabctl update`
inside it.

## Build the docs image

```bash
homelabctl build docs --tag dev
```

Customise its complete image name or add multiple tags:

```bash
homelabctl build docs \
  --image iamkhattar/homelab-docs \
  --tag latest \
  --tag revision-id
```

The build uses `docs/` as an isolated Docker context and produces an
unprivileged Nginx runtime image. `--push` has the same CI-only guard as service
images. An omitted tag resolves to the same current Git commit SHA used by
service builds.

## Run repository checks

```bash
homelabctl ci check
```

The aggregate currently runs:

1. Go formatting across every module;
2. Go tests across every module;
3. the VitePress production build;
4. GitHub Actions YAML and repository CI safety policy;
5. offline Ansible lint and syntax checks for every playbook;
6. Terraform format, backend-free initialisation, validation and tests.

Failures are aggregated so an early broken area does not hide all later areas.
Run a focused subset while iterating:

```bash
homelabctl ci check --only ansible
homelabctl ci check --only go-format,go-test
homelabctl ci check --skip terraform
```

`--only` and `--skip` cannot be combined. Supported names are `go-format`,
`go-test`, `docs`, `workflows`, `ansible` and `terraform`.

### Generate CI reports and run security scans

Install the pinned reporting tools through the same operator interface:

```bash
homelabctl setup reports
```

Then run reporting mode:

```bash
homelabctl ci check --reports
```

Reporting mode keeps every normal check, replaces plain `go test` execution
with `gotestsum`, and appends three security/reporting stages. Its generated
paths are stable and Git-ignored:

| Path | Format and contents | CI consumer |
| --- | --- | --- |
| `test-results/<module>.xml` | JUnit result for each Go module | Downloadable report artifact |
| `test-results/<module>.json` | Complete line-delimited `go test -json` stream | Debugging, timing and flaky-test analysis |
| `sarif/gosec-<module>.sarif` | Go AST/SSA findings with tracked suppressions | GitHub code scanning and report artifact |
| `sarif/trivy.sarif` | High/critical dependency, IaC and secret findings | GitHub code scanning and report artifact |
| `sbom/homelab.spdx.json` | Repository-wide SPDX JSON software bill of materials | Report artifact and downstream tooling |

`gotestsum` and `gosec` are installed from exact Go module versions. Trivy
runs from `ghcr.io/aquasecurity/trivy:0.74.0` pinned to an immutable digest;
its version-notice check is disabled because upgrades are reviewed and pinned
in this repository rather than selected dynamically during CI.
The checkout is mounted read-only at `/workspace`; only `sarif/`, `sbom/` and
the ignored `trivy-cache/` receive writable mounts. The cache avoids downloading
the vulnerability databases twice during the security and SBOM stages.

The filesystem scan excludes generated controller dependencies and build
output: the Ansible virtual environment and downloaded collections, Terraform
provider cache, npm modules, VitePress cache/output, compiled `bin/` artifacts
and report directories. Their committed manifests and lockfiles remain in
scope, as do homelab code, Dockerfiles, Terraform source, Helm charts and the
VitePress configuration and custom components. This prevents upstream Ansible
integration-test fixtures from being reported as if they were homelab source.

Security findings are gating failures. The aggregate runner continues after a
failed stage, however, so all possible reports are produced. GitHub Actions
uses `if: always()` to retain `test-results/`, `sarif/` and `sbom/` for 14 days
even when checks fail. Each SARIF file is uploaded separately with a stable,
unique category (`gosec-homelabctl`, `gosec-services-butler` or
`trivy-repository`) because GitHub treats each as an independent analysis. The
workflow uploads SARIF to code scanning when the event has
permission. Fork pull requests still receive the downloadable artifact but do
not receive elevated `security-events` access.

Use focused reporting during development:

```bash
homelabctl ci check --reports --only go-test
homelabctl ci check --reports --only gosec,trivy,sbom
```

Without `--reports`, the six normal check names remain the complete selection
set and Go tests use the native `go test` command. With `--reports`, the
additional selectable names are `gosec`, `trivy` and `sbom`.

## Build and publish the complete CI image set

Build every service image plus the homelabctl and documentation images with one
command:

```bash
homelabctl ci build --tag dev
```

For a pull-request-style incremental build, service selection uses the embedded
`go-git` implementation while the homelabctl and docs images are always built:

```bash
homelabctl ci build --changed --base origin/main --tag revision-id
```

Publication is deliberately CI-only. With no `--tag`, it publishes only the
current Git commit SHA:

```bash
CI=true homelabctl ci publish
```

Add mutable or release tags explicitly when they are intended:

```bash
CI=true homelabctl ci publish \
  --changed \
  --base previous-revision \
  --tag v0.1.42 \
  --tag latest \
  --tag revision-id
```

The first resolved tag is embedded as the build version in every Go image.
Aggregate builds therefore give Butler and the homelabctl image exactly the
same version. On `main`, GitHub Actions deliberately supplies the shared
semantic `v0.1.<workflow run number>` tag first, followed by `latest` and the
source SHA. Butler reports that version, commit and build date in its startup
log; `homelabctl version` reports the same version and commit.

`ci build` and `ci publish` call the same internal build operations exposed by
`build services`, `build homelabctl` and `build docs`. Publication adds Docker
pushes; it does not deploy anything to Kubernetes. Use `--registry` to change
the service image namespace, `--homelabctl-image` to change the CLI image name,
and `--docs-image` to change the complete docs image name.

The default is the full commit SHA resolved from the repository by `go-git`.
Dry-run mode resolves and displays the same real SHA without spawning the Git
binary; only Docker execution is skipped. GitHub Actions defines one
`RELEASE_VERSION`, uses it for both image publication and GoReleaser, and
supplies the immutable event SHA and `latest` as additional tags. Mutable-tag
publication therefore remains visible in workflow source.

## GitHub Actions flow

The workflow bootstraps the CLI, then uses it to install repository dependencies
and run `homelabctl ci check --reports`. It uploads portable reports before any
later build or release activity. Pull requests invoke `homelabctl ci build`
without pushing. The main branch invokes `homelabctl ci publish` with the
shared semantic version,
immutable revision and `latest` tags for services, homelabctl and docs, then
pushes them after authentication. In parallel after checks, a main-only release
job uses that same version with the pinned GoReleaser action to publish static
`homelabctl` archives and `checksums.txt`.

Every `actions/setup-go` step enables its module and build-output cache
explicitly. The check job keys that cache from both `homelabctl/go.sum` and
`services/butler/go.sum`; image-publication and release jobs key their local CLI
build from `homelabctl/go.sum`. Cache misses affect duration, never correctness.

The check job also restores `ansible/.venv` and `ansible/collections` from an
exact operating-system, architecture, Python-version and requirements hash. On
a cache hit, CI skips `homelabctl setup ansible`; on a miss, it installs through
homelabctl and saves the completed runtime immediately, before any test or scan
can fail the job. Documentation and reporting setup still run every time: npm
and setup-go provide their package/build caches, while the commands continue to
verify their locked dependency contracts.

`homelabctl ci check --only workflows` parses every workflow and enforces the
local contract: read-only default permissions, concurrency cancellation,
bounded job timeouts, full Git history for merge-base selection, check-before-
publish/release ordering, SHA-tagged PR builds, CI-gated main publication,
main-only release execution, least-privilege release permissions, one shared
immutable semantic release version, required report/SARIF uploads, and the
absence of deploy, Terraform apply/destroy, Helmfile apply/sync and kubectl
apply commands. GitHub remains responsible for validating its complete Actions
schema.

Image publication is not deployment. Titan is not automatically mutated by the
current workflow. Deployment remains an explicit top-level operation:

```bash
homelabctl deploy diff
homelabctl deploy apply
```

If secure CD is added later, a trusted runner may invoke those same commands;
there should not be separate hidden deployment logic inside the workflow.

CLI release publication is also not deployment. Continue with [releases and
updates](/homelabctl/releases-update) for the artifact and update
contract.
