# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

Homelab infrastructure on Hetzner Cloud. Terraform provisions cloud resources, cloud-init bootstraps nodes by cloning this repo and running Ansible, which installs and configures a K3s Kubernetes cluster. Helm charts (managed via Helmfile) deploy cluster services.

## Architecture

The repo has three layers that execute in sequence:

1. **`infra/`** — Terraform (Hetzner Cloud provider, Terraform Cloud backend `iamkhattar/homelab`). Provisions a private network (`10.0.0.0/16`, subnet `10.0.1.0/24` in `eu-central`), one server node (static IP `10.0.1.1`), N agent nodes, and two firewalls (public for HTTP/HTTPS/K8s API, private for intra-cluster). Cloud-init user data templates (`infra/config/cloud-init-*.yml`) clone this repo on each node and run the Ansible playbook automatically.

2. **`cluster/k3s/`** — Ansible roles for K3s installation. The `site.yml` playbook runs three roles in order: `prerequisites` (OS prep, networking, IP detection), `k3s_server` (downloads and starts K3s server with `--secrets-encryption`), `k3s_agent` (joins agent nodes to the server at `10.0.1.1:6443`). Inventory files are per node type (`inventory-server.yml`, `inventory-agent.yml`) and use `ansible_connection: local` because cloud-init runs Ansible on-node. The K3s version is pinned in inventory vars.

3. **`cluster/` (Helm layer)** — `helmfile.yaml` at `cluster/` root orchestrates Helm releases with dependency ordering via `needs`. Local charts live in `cluster/core/` and `cluster/storage/`. Current releases: namespaces, rbac-policies, cert-manager, longhorn. Each chart's `values.yaml` defines the configurable resources.

## Key Commands

### Terraform (run from `infra/`)
```
terraform init              # Initialize providers and Terraform Cloud backend
terraform fmt -check        # Check formatting (CI runs this)
terraform validate          # Validate configuration
terraform plan              # Preview changes (requires TF_VAR_* env vars)
terraform apply             # Apply changes
```
Required environment variables: `TF_VAR_hetzner_cloud_api_token`, `TF_VAR_k3s_api_token`, `TF_VAR_ssh_public_key`.

### Ansible (run from `cluster/k3s/`)
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
