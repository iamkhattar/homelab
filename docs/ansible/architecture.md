# Architecture and dependencies

The Ansible layer intentionally separates homelab policy from K3s lifecycle
mechanics. Local code owns Debian preparation, inventory conventions,
diagnostics and recovery workflows. The pinned upstream collection owns K3s
installation, agent joining, version upgrades and cluster-aware reboots.

## Ownership boundary

| Concern | Owner |
| --- | --- |
| Debian packages, hostname, timezone, shell marker and sudo membership | Local `homelab_base` role |
| SSH keys and optional hardening | Local `homelab_base` role |
| Swap, sleep, journald, updates, Chrony and SSD trim | Local `homelab_base` role |
| Inventory topology and homelab node labels | This repository |
| K3s prerequisites, binaries, services and configuration | Pinned `k3s.orchestration` collection |
| K3s upgrade and cluster-aware reboot order | Pinned `k3s.orchestration` collection |
| Fixed node and cluster diagnostics | Local playbooks |
| Embedded-etcd snapshots and off-node export | Local playbooks |
| Workload deployment | Helmfile, outside Ansible |
| Future host firewall and Tailscale | Not implemented yet |

This boundary avoids carrying a private fork of the upstream K3s roles. Local
policy remains small enough to audit and can evolve without copying K3s install
logic into the repository.

## Repository layout

```text
ansible/
├── ansible.cfg
├── .ansible-lint
├── inventory/
│   ├── hosts.example.yml
│   └── hosts.yml                 # private and Git-ignored
├── playbooks/
│   ├── prepare.yml
│   ├── site.yml
│   ├── upgrade.yml
│   ├── reboot.yml
│   ├── reboot-node.yml
│   ├── diagnose-node.yml
│   ├── diagnose-cluster.yml
│   ├── snapshot.yml
│   └── recovery-export.yml
├── roles/
│   └── homelab_base/
├── requirements.txt
└── requirements.yml
```

Generated `.venv/`, `.ansible/` and `collections/` directories are local
runtime state. `homelabctl setup ansible` recreates their required contents.

## Dependency pins

Python dependencies are exact:

| Dependency | Current pin | Purpose |
| --- | --- | --- |
| `ansible-core` | `2.20.8` | Playbook engine and built-in modules |
| `ansible-lint` | `26.8.0` | Static policy and syntax validation |
| `netaddr` | `1.3.0` | Network filters required by Ansible content |

Collection dependencies are also exact:

| Collection | Current pin | Purpose |
| --- | --- | --- |
| `k3s.orchestration` | Git commit `281554e2522ab370014526ad734e0ec53b023e7c` | Official K3s lifecycle playbooks and roles |
| `community.general` | `13.3.0` | Timezone and supporting modules |
| `ansible.posix` | `2.2.2` | Authorised-key and POSIX modules/callbacks |
| `ansible.utils` | `6.1.0` | Networking filters used upstream |
| `community.library_inventory_filtering_v1` | `1.1.5` | Upstream inventory-filter dependency |

The Git commit is deliberate: a branch or tag could move, while an audited
commit identifies the exact upstream source used for installation and recovery.
Pins do not update during ordinary node or cluster operations.

## How upstream K3s installation runs

The imported upstream site playbook executes three plays:

1. `prereq`, `airgap` and `raspberrypi` roles against `k3s_cluster`;
2. `k3s_server` against the `server` group;
3. `k3s_agent` against the `agent` group.

For the first home deployment, `server` contains Titan and `agent` is empty.
The first server generates a token because inventory deliberately omits one.
The upstream roles pass that generated value to later servers or agents during
the same operation without requiring a plaintext inventory token.

The collection also contains a destructive reset playbook. This repository does
not import or expose it through `homelabctl`.

## Ansible configuration

`ansible.cfg` defines repository-local behaviour:

| Setting | Effect |
| --- | --- |
| `inventory` | Uses private `inventory/hosts.yml` by default |
| `roles_path` | Resolves the local `roles/` directory |
| `collections_path` | Resolves pinned downloads from `collections/` |
| `local_tmp` | Keeps temporary Ansible controller files under `.ansible/tmp` |
| `host_key_checking` | Refuses unknown or changed SSH identities unless explicitly accepted |
| `retry_files_enabled` | Avoids stale retry files that can accidentally retarget later runs |
| `callbacks_enabled` | Reports task timing through `ansible.posix.profile_tasks` |
| `timeout` | Uses a 30-second connection timeout |
| SSH pipelining | Reduces remote process and file-copy overhead |
| SSH ControlPersist | Reuses authenticated connections for 60 seconds |

`homelabctl` selects the local virtual-environment executables and sets
`ANSIBLE_HOME` automatically. Operator shells do not activate `.venv`.

## Compatibility gates

The local base role stops before mutation unless:

- Ansible is version 2.15 or newer;
- the target distribution is Debian;
- the Debian major release is 12 or 13.

The workstation currently pins a newer Ansible version than the minimum role
assertion. Supporting another operating system requires explicit role work and
tests; changing the assertion alone is insufficient.

## Design exclusions

The Ansible layer currently does not:

- provision Debian itself;
- manage BIOS, router DHCP reservations or port forwarding;
- configure a host firewall;
- install Tailscale;
- manage personal shell configuration or a prompt framework;
- deploy Kubernetes workloads;
- initialise Vault or store application secrets;
- destroy or reset K3s.

These exclusions are deliberate. New responsibilities should be added only with
inventory variables, safety gates, `homelabctl` commands and corresponding
runbooks.
