# Ansible automation

The Ansible component prepares Debian hosts and coordinates the K3s lifecycle.
Local roles contain homelab policy; the pinned official `k3s.orchestration`
collection contains upstream K3s installation, upgrade and rolling-reboot
mechanics.

Operators normally enter this component through `homelabctl`. Contributors use
this section to understand inventory, role and playbook boundaries before
changing automation.

## Start here

| Goal | Next page |
| --- | --- |
| Understand ownership and exact dependency pins | [Architecture and dependencies](/ansible/architecture) |
| Add or change a machine | [Inventory model](/ansible/inventory) |
| Change Debian preparation | [Base role reference](/ansible/base-role) |
| Change SSH policy safely | [Debian hardening](/ansible/hardening) |
| Understand an operational entry point | [Playbook reference](/ansible/playbooks) |
| Upgrade dependencies or validate changes | [Testing and upgrades](/ansible/testing-upgrades) |

Read architecture before editing a role or importing another upstream
playbook. The repository deliberately keeps K3s mechanics out of local roles.

## Responsibility boundary

| Local automation owns | Upstream K3s collection owns | Outside Ansible |
| --- | --- | --- |
| Debian packages and updates | K3s prerequisites | Debian installation media |
| Operator users and SSH policy | K3s binary and service installation | Router DHCP reservations |
| Hostname, timezone and host behavior | Server and agent joining | Workload Helm releases |
| Diagnostics and recovery export | Version upgrades and rolling reboots | Vault application secrets |
| Inventory labels and taints | K3s lifecycle ordering | Terraform cloud resources |

This division keeps local policy reviewable and makes upstream lifecycle fixes
available without maintaining a private copy of K3s roles.

## Execution flow

1. `homelabctl` validates operator flags and selects the repository-local
   Ansible executable.
2. `ansible.cfg` loads the private inventory and repository paths.
3. A small local playbook composes `homelab_base` with a pinned upstream
   lifecycle playbook where required.
4. The playbook performs its fixed verification, such as SSH reachability or
   Kubernetes node readiness.
5. Ansible returns its recap through `homelabctl` without the CLI reinterpreting
   task state.

## Repository map

```text
ansible/
├── ansible.cfg
├── inventory/
│   ├── hosts.example.yml       committed contract
│   └── hosts.yml               private operator values, ignored
├── playbooks/                  supported operational entry points
├── roles/
│   └── homelab_base/           reusable Debian policy
├── requirements.txt            Python dependency pins
└── requirements.yml            collection pins
```

Generated `.venv/`, `.ansible/` and `collections/` directories are local
runtime state. `homelabctl setup ansible` recreates them.

## Current maturity

The component has repository-level lint and syntax coverage, staged SSH
hardening, diagnostics, snapshots and off-node recovery export. That means the
automation is ready to run; it does not mean Titan has been changed. Deployment
truth remains on the [current-state page](/project/current-state).

Validate changes through the supported interface:

```bash
homelabctl ci check --only ansible
```

Continue with [Architecture and dependencies](/ansible/architecture).
