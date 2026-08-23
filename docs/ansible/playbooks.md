# Playbook reference

Local playbooks compose the base role, imported upstream K3s lifecycle and
fixed operational tasks. Operators invoke them through typed `homelabctl`
commands.

## Summary

| Playbook | Target | Mutation | homelabctl command |
| --- | --- | --- | --- |
| `prepare.yml` | `k3s_cluster` | Debian baseline | `node prepare` |
| `site.yml` | cluster, servers, agents | Baseline and K3s install/reconcile | `cluster bootstrap` |
| `upgrade.yml` | servers then agents | K3s version upgrade | `cluster upgrade` |
| `reboot.yml` | servers then agents | K3s-aware rolling reboot | `cluster reboot` |
| `reboot-node.yml` | `k3s_cluster` | Pre-K3s Debian reboot | `node reboot` |
| `diagnose-node.yml` | `k3s_cluster` | None intended | `node diagnose` |
| `diagnose-cluster.yml` | first server | None intended | `cluster diagnose` |
| `snapshot.yml` | first server | Optional snapshot creation | `cluster snapshot` |
| `recovery-export.yml` | first server and workstation | Snapshot plus local export | `cluster recovery export` |

## prepare.yml

This is the smallest mutating host workflow. It gathers facts, becomes root and
applies `homelab_base` to every host in `k3s_cluster`.

```bash
homelabctl node prepare --check --limit titan --ask-become-pass
homelabctl node prepare --limit titan --ask-become-pass
```

Use it for initial Debian preparation and routine operating-system maintenance.
It does not install, upgrade or restart K3s intentionally, although package and
host changes can still require a scheduled reboot.

## site.yml

The cluster bootstrap sequence is:

1. import local `prepare.yml`;
2. import `k3s.orchestration.site`;
3. run verification on the first server;
4. wait up to 180 seconds for every node to become Ready, retrying the wait up
   to three times with ten-second delays;
5. print the resulting wide node list.

```bash
homelabctl cluster bootstrap --ask-become-pass
```

The upstream playbook runs prerequisites against the full cluster, server setup
against `server`, then agent setup against `agent`. Packaged Traefik is disabled
by inventory, while secrets encryption and embedded-etcd snapshots are enabled.

## upgrade.yml

The local playbook imports the upstream upgrade sequence. Servers are upgraded
with `serial: 1` to protect etcd membership, while agents may upgrade without
that server constraint. Local verification then waits for all nodes to become
Ready.

```bash
homelabctl cluster upgrade --ask-become-pass
```

The target comes only from exact inventory `k3s_version`. The documented
workflow requires a recovery export and Ansible validation before this command.

## reboot.yml and reboot-node.yml

These playbooks serve different lifecycle stages:

- `node reboot` uses `reboot-node.yml` before K3s exists. Hosts are processed
  serially, waited for, then checked with Ansible ping.
- `cluster reboot` imports the upstream K3s-aware reboot. Servers are serial and
  use a Kubernetes node query as their reboot test; agents reboot serially
  afterward.

Using the pre-K3s playbook after installation would omit Kubernetes health
checks. Using the cluster playbook before installation would fail its K3s test.

## Diagnostic playbooks

`diagnose-node.yml` gathers hostname, failed services, SSH validation,
filesystem usage and time state. `diagnose-cluster.yml` gathers K3s service
state, version, snapshots, nodes, pods, events and recent logs.

```bash
homelabctl node diagnose --limit titan --ask-become-pass
homelabctl cluster diagnose --ask-become-pass
```

Each fixed diagnostic command uses `changed_when: false` and
`failed_when: false`. One failing probe therefore appears in its recorded return
code without preventing later evidence collection. The overall play can succeed
while an individual diagnostic reports a problem; read every result.

## snapshot.yml

The snapshot playbook accepts only `list` or `save`. Save names are validated
both by the Go CLI and Ansible before being passed as an argument to K3s.

```bash
homelabctl cluster snapshot list --ask-become-pass
homelabctl cluster snapshot save --name before-change --ask-become-pass
```

The first server performs the operation. Listing is unchanged; saving is
reported as changed.

## recovery-export.yml

The recovery playbook requires a controller-side export directory and a safe
snapshot prefix. It:

1. creates a fresh K3s snapshot;
2. finds matching files beneath the K3s snapshot directory;
3. selects the newest match;
4. creates a private host directory on the workstation;
5. fetches the snapshot with checksum validation;
6. fetches the K3s server token under task-scoped `no_log`;
7. restricts both local files to `0600`;
8. reports the non-secret destination.

The Go command creates an additional unique UTC-timestamped parent directory
before invoking Ansible:

```bash
homelabctl cluster recovery export \
  --destination /path/to/encrypted-staging \
  --name recovery \
  --ask-become-pass
```

The command stages recovery material; it does not encrypt, upload or validate a
restore. Those operator responsibilities are covered by [cluster lifecycle and
recovery](/homelabctl/cluster-lifecycle).

## Limit semantics

Ansible-backed CLI commands expose `--limit HOST_OR_GROUP`. Limiting a normal
node preparation to Titan is useful. Limiting cluster lifecycle plays can skip
required server or agent stages and should be done only when the playbook's
dependency order is understood.

In particular, a limit that excludes `server[0]` also excludes local
verification, snapshots, diagnostics or recovery export. An empty matched host
set is not proof that the desired operation occurred.
