# Maintenance and upgrades

Single-node maintenance always causes some interruption. The objective is a
controlled, observable interruption with a tested recovery path.

## Routine checks

Before changing the host or cluster:

```bash
homelabctl cluster status --all-pods
homelabctl node diagnose --limit titan --ask-become-pass
homelabctl cluster snapshot list --ask-become-pass
```

Investigate existing failures before introducing another change.

## Debian updates

Preview and apply the base role:

```bash
homelabctl node prepare --check --ask-become-pass
homelabctl node prepare --ask-become-pass
```

The role performs a full APT upgrade. If it reports a required reboot, create an
on-demand etcd snapshot and then use the rolling reboot playbook:

```bash
homelabctl cluster snapshot save --name pre-reboot --ask-become-pass
homelabctl cluster reboot --ask-become-pass
```

Afterward, repeat the routine health checks.

## Upgrade K3s

Never point production inventory at a floating release channel. Upgrade through
an explicit reviewed version change.

### 1. Select and review the target

Read the K3s release notes, Kubernetes version-skew policy and any deprecations
between the installed and proposed releases. Prefer incremental supported minor
upgrades rather than skipping several minors.

### 2. Create a pre-upgrade snapshot

```bash
homelabctl cluster recovery export \
  --destination /secure/homelab-recovery \
  --name pre-k3s-upgrade \
  --ask-become-pass
homelabctl cluster snapshot list --ask-become-pass
```

Copy the snapshot and server token off Titan before proceeding.

### 3. Change the pinned version

Update `k3s_version` in both the private inventory and the committed example:

```yaml
k3s_version: vX.Y.Z+k3sN
```

Keeping the example aligned ensures a rebuilt inventory does not downgrade or
silently select a different release.

### 4. Validate automation

```bash
homelabctl ci check --only ansible
```

### 5. Run the controlled upgrade

```bash
homelabctl cluster upgrade --ask-become-pass
```

The upstream playbook processes servers serially, then agents. The local final
play waits for all nodes to be Ready.

### 6. Verify

```bash
homelabctl cluster status --all-pods
homelabctl cluster diagnose --ask-become-pass
```

Do not delete the pre-upgrade snapshot until workloads and storage have been
checked and the cluster has remained healthy.

## Upgrade Ansible dependencies

K3s and Ansible automation are separate upgrades. To upgrade the automation:

1. select new exact versions or an upstream Git commit;
2. update `requirements.txt` or `requirements.yml`;
3. run `homelabctl setup ansible` to rebuild pinned local dependencies;
4. run `homelabctl ci check --only ansible`;
5. inspect upstream changes before running against Titan.

Do not update the upstream collection automatically as part of cluster
maintenance.
