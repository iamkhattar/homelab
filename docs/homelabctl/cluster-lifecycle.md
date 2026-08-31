# Cluster lifecycle and recovery

Cluster commands cover K3s installation, direct health inspection, controlled
maintenance and embedded-etcd recovery material. They remain usable when the
future in-cluster control-plane service is unavailable.

## Bootstrap or reconcile K3s

```bash
homelabctl cluster bootstrap --ask-become-pass
```

The command runs the local OS baseline, the pinned upstream K3s installation
playbook, then waits for every Kubernetes node to become Ready. It is
idempotent and may be re-run to reconcile supported configuration drift.

`cluster install` remains an alias, but documentation uses `bootstrap` because
the operation is broader than a one-time package installation.

Do not use global `--dry-run` as proof that K3s can install; it only prints the
Ansible invocation. Do not add Ansible check mode to initial K3s installation.

## Inspect health

```bash
homelabctl cluster status
homelabctl cluster status --all-pods
homelabctl cluster nodes
```

`status` prints nodes and, by default, pods that are neither Running nor
Succeeded. `--all-pods` includes healthy and completed pods for a full rollout
view. `nodes` adds all node labels, which is useful when checking location and
hardware placement metadata.

These commands use the kubectl context selected by global `--context`, which
defaults to `homelab`:

```bash
homelabctl --context homelab cluster status
```

Kubeconfig is an administrator credential. It is not stored by `homelabctl` and
must never be committed.

## Collect cluster diagnostics

```bash
homelabctl cluster diagnose --ask-become-pass
```

The fixed read-only evidence set includes:

- K3s systemd status and recent journal entries;
- installed K3s version;
- embedded-etcd snapshot list;
- nodes, pods and recent Kubernetes events.

Diagnostics run through Ansible on the first server, so they can still work when
workstation kubeconfig is broken but SSH and sudo remain available. Review logs
for credentials before sharing them.

## Create and list snapshots

```bash
homelabctl cluster snapshot list --ask-become-pass
homelabctl cluster snapshot save \
  --name before-change \
  --ask-become-pass
```

Snapshot names must start with a letter or number and may contain letters,
numbers, dots, underscores and hyphens. The default save prefix is `manual`.
K3s appends node and timestamp information to the resulting filename.

A snapshot left on Titan does not protect against SSD loss, theft or loss of the
whole machine. Use recovery export for an off-node copy.

Scheduled snapshots are created by K3s at midnight and noon UTC. Titan retains
the newest 14 scheduled snapshots automatically. Named snapshots created by
`snapshot save` have no automatic retention and are not currently deleted by
`homelabctl`; do not remove their files directly. The supported CLI needs a
guarded, exact-name delete/prune workflow before routine manual cleanup is
enabled. See [backup and recovery](/operations/backup-recovery) for the complete
retention policy.

## Export recovery material

```bash
homelabctl cluster recovery export \
  --destination /path/to/encrypted-staging \
  --name pre-maintenance \
  --ask-become-pass
```

This command:

1. creates a unique UTC-timestamped directory beneath the destination;
2. creates a fresh embedded-etcd snapshot;
3. verifies that a matching snapshot exists;
4. fetches the snapshot and K3s server token without printing the token;
5. restricts directories to `0700` and files to `0600`.

The resulting shape is:

```text
<destination>/
└── 20260823T120000Z/
    └── titan/
        ├── etcd-snapshot
        └── server-token
```

The destination is staging space. Immediately encrypt the export, move it to
storage outside Titan and the workstation, verify the copied checksum, and
remove the plaintext staging copy. Every invocation gets a new directory and
does not overwrite an older export.

`homelabctl` never deletes recovery exports. Retain the first-install recovery
point, then initially keep seven daily, four weekly and six monthly encrypted
off-device exports. Remove an older timestamped export only after verifying a
newer copy and resolving the exact directory outside the repository.

::: danger Sensitive output
The `server-token` is sufficient to join privileged K3s nodes and is part of the
cluster recovery root of trust. Do not inspect it through terminal commands,
paste it into issue trackers, or store the export in an unencrypted synced
folder.
:::

## Controlled K3s upgrade

The supported sequence is:

```bash
homelabctl cluster status --all-pods
homelabctl cluster recovery export \
  --destination /path/to/encrypted-staging \
  --name pre-k3s-upgrade \
  --ask-become-pass
homelabctl ci check --only ansible
homelabctl cluster upgrade --ask-become-pass
homelabctl cluster status --all-pods
homelabctl cluster diagnose --ask-become-pass
```

Before this sequence, review K3s release notes and change the exact
`k3s_version` in both private and example inventories. The upgrade playbook
processes servers before agents and waits for node readiness afterward.

Do not delete the pre-upgrade export until workloads, persistent data and
backups have been checked.

## Reboot an installed cluster

```bash
homelabctl cluster snapshot save --name pre-reboot --ask-become-pass
homelabctl cluster reboot --ask-become-pass
homelabctl cluster status --all-pods
```

Servers reboot serially and must pass Kubernetes health checks before the
playbook advances. Agents reboot separately. On the single-node cluster this
causes downtime; the ordering becomes important when remote agents are added.

## Restore boundary

There is intentionally no automated `cluster restore` command yet. Restore is
destructive, depends on the installed K3s version and needs rehearsal against a
disposable target. Follow [backup and recovery](/operations/backup-recovery)
and the upstream version-matched K3s procedure. A future restore command must
require an explicit snapshot, token and target and must preserve the failed data
directory before mutation.
