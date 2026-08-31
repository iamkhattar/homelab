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

K3s uses its default schedule of midnight and noon in Titan's UTC timezone.
The configured retention of 14 applies to scheduled snapshots, so K3s removes
the oldest scheduled snapshot after the fifteenth is created. This provides
approximately seven days of local control-plane history.

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

## Retention and deletion policy

The three backup classes have different retention behaviour:

| Class | Location | Automatic deletion | Initial policy |
| --- | --- | --- | --- |
| Scheduled etcd snapshot | Titan | Yes, newest 14 | Let K3s maintain approximately seven days |
| Named manual snapshot | Titan | No | Keep until its maintenance or rollback window has passed |
| Recovery export | Operator-selected off-node storage | No | Keep the first install, latest seven daily, four weekly and six monthly recovery points |

Always keep a pre-upgrade export until the upgraded cluster and its stateful
workloads have passed verification. If the K3s server token is ever rotated,
keep each older snapshot with the token that existed when it was created.

Do not remove files directly from Titan's snapshot directory. K3s provides
metadata-aware `delete` and `prune` operations, but `homelabctl` does not expose
them yet. Scheduled snapshots need no manual cleanup. Until a guarded
`homelabctl cluster snapshot delete/prune` interface is implemented and tested,
leave named manual snapshots in place unless disk pressure creates an explicit
maintenance need.

Recovery exports are deliberately outside `homelabctl` deletion authority.
Delete an old export only after listing the retained recovery points, confirming
a newer encrypted off-device export is readable, and checking that the selected
directory is the exact timestamped export—not its parent storage volume. Keep a
small inventory containing creation date, K3s version, repository revision,
checksum and storage location, but never the server token itself.

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
