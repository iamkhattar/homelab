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
| `--tag TAG` | Add an image tag; repeat for multiple tags; defaults to the current full Git commit SHA |
| `--registry NAME` | Set the image namespace; default `iamkhattar` |
| `--changed` | Build services changed between `--base` and `HEAD` |
| `--base REVISION` | Required with `--changed` |
| `--push` | Push every generated image tag; allowed only when `CI` is set |

Explicit service names and `--changed` are mutually exclusive. Unknown service
names fail before Docker is invoked. Use `--tag dev` deliberately when a stable
local-development tag is more convenient than the immutable default.

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

## Build and publish the complete CI image set

Build every service image and the documentation image with one command:

```bash
homelabctl ci build --tag dev
```

For a pull-request-style incremental build, service selection uses the embedded
`go-git` implementation while the docs image is always built:

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
  --tag latest \
  --tag revision-id
```

`ci build` and `ci publish` call the same internal build operations exposed by
`build services` and `build docs`. Publication adds Docker pushes; it does not
deploy anything to Kubernetes. Use `--registry` to change the service image
namespace and `--docs-image` to change the complete docs image name.

The default is the full commit SHA resolved from the repository by `go-git`.
Dry-run mode resolves and displays the same real SHA without spawning the Git
binary; only Docker execution is skipped. GitHub Actions supplies both the
immutable event SHA and `latest` explicitly on main, so mutable-tag publication
remains visible in workflow source.

## GitHub Actions flow

The workflow bootstraps the CLI, then uses it to install repository dependencies
and run checks. Pull requests invoke `homelabctl ci build` without pushing. The
main branch invokes `homelabctl ci publish` to build immutable revision tags
plus `latest` and push them after authentication.

`homelabctl ci check --only workflows` parses every workflow and enforces the
local contract: read-only default permissions, concurrency cancellation,
bounded job timeouts, full Git history for merge-base selection, check-before-
publish ordering, SHA-tagged PR builds, CI-gated main publication, and the
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
