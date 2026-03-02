# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

Homelab infrastructure supporting both local (home) nodes and external (Hetzner) nodes. Ansible bootstraps all nodes into a K3s cluster. Terraform provisions external cloud resources. Helm charts (managed via Helmfile) deploy cluster services. Custom services and CLIs are built and deployed from this repo.

## Architecture

The repo has four top-level layers:

1. **`infra/`** — Terraform (Hetzner Cloud provider, Terraform Cloud backend `iamkhattar/homelab`). Provisions external nodes with a private network (`10.0.0.0/16`, subnet `10.0.1.0/24` in `eu-central`), firewalls, and cloud-init templates (`infra/config/cloud-init-*.yml`).

2. **`ansible/`** — Ansible roles for bootstrapping all nodes (local and external) into a K3s cluster. The `site.yml` playbook runs three roles in order: `prerequisites` (OS prep, networking, IP detection), `k3s_server` (downloads and starts K3s server with `--secrets-encryption`), `k3s_agent` (joins agent nodes to the server). Inventory files are per node type. The K3s version is pinned in inventory vars.

3. **`cluster/`** — Helmfile at `cluster/` root orchestrates Helm releases with dependency ordering via `needs`. Charts are organized into `core/` (namespaces, RBAC, cert-manager), `storage/` (Longhorn), and `apps/` (user-facing applications like Home Assistant, Vault, MQTT).

4. **`services/` and `cli/`** — Custom services and CLIs that get containerized and deployed to the cluster.

## Key Commands

### Terraform (run from `infra/`)
```
terraform init              # Initialize providers and Terraform Cloud backend
terraform fmt -check        # Check formatting (CI runs this)
terraform validate          # Validate configuration
terraform plan              # Preview changes (requires TF_VAR_* env vars)
terraform apply             # Apply changes
```
Required environment variables: `TF_VAR_hetzner_cloud_api_token`, `TF_VAR_ssh_public_key`.

### Ansible (run from `ansible/`)
```
ansible-playbook playbooks/site.yml -i inventory/inventory-server.yml -e 'token=<k3s_token>'   # Bootstrap server
ansible-playbook playbooks/site.yml -i inventory/inventory-agent.yml -e 'token=<k3s_token>'    # Bootstrap agent
ansible-playbook playbooks/upgrade.yml -i inventory/inventory-server.yml                        # Upgrade K3s
ansible-playbook playbooks/reboot.yml -i inventory/inventory-server.yml                         # Rolling reboot
```

### Helmfile (run from `cluster/`)
```
helmfile sync     # Deploy/update all releases
helmfile diff     # Preview changes
helmfile apply    # Apply only changed releases
```

## CI/CD

- **Infrastructure workflow** (`.github/workflows/infrastructure.yml`): Triggers on changes to `infra/` or the workflow file. Runs `fmt -check`, `init`, `validate`, `plan` (on PRs with comment output), and `apply` (on push to main only).

## Conventions

- Namespaces are managed declaratively in `cluster/core/namespaces/values.yaml` — add new namespaces there, not via `kubectl`.
- RBAC service accounts and roles are in `cluster/core/rbac-policies/values.yaml`.
- Helm chart dependencies (e.g., cert-manager, longhorn) are declared in each chart's `Chart.yaml` and locked in `Chart.lock`.
- Terraform state is remote (Terraform Cloud); never commit `.tfstate` files.
- Sensitive variables (`*.tfvars`, SSH keys, kubeconfig) are gitignored.
- Ansible `no_log: true` is used for tasks handling tokens.
