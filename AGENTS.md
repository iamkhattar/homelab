# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

Homelab infrastructure supporting both local (home) nodes and external (Hetzner) nodes. Ansible bootstraps all nodes into a K3s cluster. Terraform provisions external cloud resources. Helm charts (managed via Helmfile) deploy cluster services. Custom services and CLIs are built and deployed from this repo.

## Architecture

The repo has five top-level layers:

1. **`infra/`** — Terraform (Hetzner Cloud provider, Terraform Cloud backend `iamkhattar/homelab`). Provisions external nodes with a private network (`10.0.0.0/16`, subnet `10.0.1.0/24` in `eu-central`), firewalls, and cloud-init templates (`infra/config/cloud-init-*.yml`).

2. **`ansible/`** — A thin wrapper around the pinned upstream `k3s.orchestration` collection. The local `homelab_base` role prepares and hardens Debian nodes before the upstream collection installs or upgrades K3s. One private `inventory/hosts.yml`, copied from `hosts.example.yml`, describes home servers and optional remote agents; node labels and taints keep location-specific workloads controlled.

3. **`docs/`** — VitePress operations guide. Covers Debian installation, the Ansible design, K3s installation and maintenance, backup/recovery, troubleshooting, and the future Hetzner/Tailscale boundary.

4. **`cluster/`** — Helmfile at `cluster/` root orchestrates releases with dependency ordering via `needs`. Charts are organized by platform domain: `core/`, `networking/`, `security/`, `databases/`, `storage/`, `monitoring/`, `cicd/`, `applications/`, `services/`, and disabled-by-default `smart-home/` increments.

5. **`homelabctl/` and `butler/`** — The Go operator/CI CLI and Butler control plane. Butler is a top-level, domain-packaged service with separate Pocket ID-authenticated normal and Kubernetes TokenReview-authenticated recovery runtimes.

## Key Commands

After the one-time `homelabctl` self-build, all normal repository procedures use
the CLI from the repository root. Native tools are implementation details or
explicit break-glass diagnostics.

### Workstation and inventory

```
homelabctl setup
homelabctl doctor
homelabctl inventory init
homelabctl inventory check
```

### Nodes and K3s

```
homelabctl node prepare --check
homelabctl node prepare
homelabctl cluster bootstrap
homelabctl cluster status
homelabctl cluster snapshot save --name before-change
homelabctl cluster recovery export --destination /path/to/encrypted-staging
homelabctl cluster upgrade
homelabctl cluster reboot
```

### Butler control plane

```
homelabctl control recovery
homelabctl control bootstrap --confirm
homelabctl control status
homelabctl control users list
kubectl get pocketidclients,managedcredentials,garagebuckets -A
```

### Deployments and optional infrastructure

```
homelabctl deploy diff
homelabctl deploy apply
homelabctl infra fmt
homelabctl infra validate
homelabctl infra plan
```

Do not add Terraform apply/destroy to the CLI until the legacy Hetzner
cloud-init and token flow has been redesigned.

### Documentation

```
homelabctl docs setup
homelabctl docs dev
homelabctl docs build
homelabctl docs preview
homelabctl build docs --tag dev
homelabctl docs serve --image iamkhattar/homelab-docs:dev
```

### Repository checks and images

```
homelabctl ci check
homelabctl ci check --only ansible
homelabctl build services
```

The only bootstrap exception is building `homelabctl` itself before a published
binary exists: from `homelabctl/`, build `./cmd/homelabctl` into `../bin/`.

## CI/CD

- **CI workflow** (`.github/workflows/ci.yml`): driven by `homelabctl`. It checks Go, docs, Ansible, Terraform, gosec and Trivy; retains test/SARIF/SBOM reports; builds changed images on pull requests; and publishes all release images from `main` so Butler and homelabctl share the release version.
- **Auto-assign workflow** (`.github/workflows/auto-assign.yml`): assigns the PR author as a reviewer.
- Push-model only: there is no in-cluster GitOps controller (no Argo CD / Flux). All deploys are initiated with `homelabctl deploy`.

## Conventions

- Namespaces are managed declaratively in `cluster/core/namespaces/values.yaml` — add new namespaces there, not via `kubectl`.
- RBAC service accounts and roles are in `cluster/core/rbac-policies/values.yaml`.
- Helm chart dependencies are declared in each chart's `Chart.yaml` and locked in `Chart.lock`.
- Vault is the source of truth for application credentials. Normal workloads receive only scoped Kubernetes Secrets through Vault Secrets Operator; do not put secret values in ConfigMaps or Helm values.
- The `butler-vault-init` Secret is recovery-only: normal Butler must never mount or read it, and operators must export it directly to an age-encrypted off-cluster bundle.
- Terraform state is remote (Terraform Cloud); never commit `.tfstate` files.
- Sensitive variables (`*.tfvars`, SSH keys, kubeconfig) are gitignored.
- Do not pass secrets in command-line extra vars or render K3s tokens into new cloud-init templates. Keep `no_log: true` scoped to the individual tasks that handle secrets.
