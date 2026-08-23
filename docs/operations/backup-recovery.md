# Backup and recovery

A snapshot on Titan protects against some logical failures. It does not protect
against SSD failure, theft, fire or loss of the entire machine.

## Recovery set

At minimum, keep encrypted off-cluster copies of:

- the K3s server token;
- recent embedded-etcd snapshots;
- the exact inventory and K3s version, without adding secrets to Git;
- this repository revision;
- storage-specific application backups;
- future Vault recovery keys or auto-unseal recovery material.

Kubernetes datastore snapshots do not automatically back up every external or
host-mounted application volume. Each stateful application needs its own tested
backup procedure.

## Snapshot behaviour

Titan uses embedded etcd with:

```yaml
cluster-init: true
etcd-snapshot-compress: true
etcd-snapshot-retention: 14
```

K3s scheduled snapshots run automatically. List them with:

```bash
homelabctl cluster snapshot list --ask-become-pass
```

Create an on-demand snapshot before risky maintenance:

```bash
homelabctl cluster snapshot save --name before-change --ask-become-pass
```

The default local snapshot directory is beneath
`/var/lib/rancher/k3s/server/db/snapshots/`. Access it as root and copy snapshots
to encrypted storage that is not mounted permanently on Titan. The supported
export workflow creates a fresh snapshot and fetches both it and the server
token without printing the token:

```bash
homelabctl cluster recovery export \
  --destination /secure/homelab-recovery \
  --name scheduled-export \
  --ask-become-pass
```

The destination is staging space, not the final backup. Encrypt it, move it
off-device and remove the plaintext staging copy after verifying the encrypted
copy. Every run creates a new UTC-timestamped subdirectory so an earlier export
is never overwritten.

## Backup verification

A backup job is not complete merely because a file exists. Regularly verify:

- the copied file size and checksum match the source;
- the encryption key is available outside Titan;
- old snapshots expire according to policy;
- a documented operator can locate the server token;
- an isolated restore rehearsal can rebuild the cluster.

## Restore planning

The exact K3s restore command and service steps depend on the installed K3s
version and whether restoration is occurring on the original or replacement
host. Before a real restore:

1. obtain the documentation matching the pinned K3s version;
2. preserve the failed disk or data directory rather than overwriting it;
3. install the same K3s version on a replacement Debian host;
4. restore using the matching server token and snapshot;
5. verify Kubernetes and every stateful workload before declaring recovery.

Refer to the official [K3s backup and restore documentation](https://docs.k3s.io/datastore/backup-restore)
for the version-specific procedure. Do not improvise destructive reset commands
against the only copy of the data.

## Vault dependency rule

Vault will eventually improve day-to-day secret handling, but in-cluster Vault
cannot be the sole recovery store for the cluster that runs it. Its unseal or
recovery material and the credentials required to restore its storage belong in
the off-cluster recovery set.
