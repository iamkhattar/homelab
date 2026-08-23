# Install the K3s cluster

Run every command from the repository root through `homelabctl`. The CLI selects
the pinned Ansible environment and prints the underlying operation.

## 1. Verify inventory and SSH

```bash
homelabctl inventory check
```

The graph must contain exactly one server named `titan` and no agents. Fix the
inventory rather than overriding it repeatedly on the command line.

## 2. Preview Debian preparation

```bash
homelabctl node prepare --check --ask-become-pass
```

Some package and command tasks cannot predict every change in check mode. Treat
the preview as an aid, not a guarantee.

## 3. Apply the baseline

```bash
homelabctl node prepare --ask-become-pass
```

Review the recap. There must be no failed or unreachable hosts. If a reboot is
reported before K3s has been installed, reboot Titan through the managed
connection:

```bash
homelabctl node reboot --limit titan --ask-become-pass
```

Wait for SSH to return, then rerun the ping and preparation playbook. A second
preparation run should be mostly unchanged.

## 4. Activate SSH hardening

Follow the complete [safe activation procedure](/ansible/hardening#safe-activation-procedure).
Do not combine the first key installation and password-login shutdown without
testing a separate key-authenticated session.

## 5. Install K3s

```bash
homelabctl cluster bootstrap --ask-become-pass
```

The playbook updates the host baseline again, runs the pinned official K3s
installer, starts the server, writes kubeconfig for the operator and waits for
all nodes to become Ready.

Do not use `--check` for the K3s installation itself.

## 6. Verify the cluster and host

```bash
homelabctl cluster status --all-pods
homelabctl cluster snapshot list --ask-become-pass
homelabctl cluster diagnose --ask-become-pass
```

Expected results:

- one node named `titan` is `Ready`;
- its roles include control-plane, etcd and master;
- core system pods become Running or Completed;
- the K3s service is active;
- scheduled snapshots appear after the first schedule has elapsed.

## 7. Verify workstation access

The upstream role merges a `homelab` context into the operator kubeconfig when
`user_kubectl: true`:

```bash
homelabctl cluster nodes
```

Treat kubeconfig as an administrator credential. Do not commit or share it.

## 8. Capture recovery material

Create a fresh snapshot and fetch it together with the server token into a
private local directory:

```bash
homelabctl cluster recovery export \
  --destination /secure/homelab-recovery \
  --name first-install \
  --ask-become-pass
```

The command does not print the token and restricts the fetched file permissions.
Immediately encrypt and move the exported directory off the workstation. Do not
commit it, leave it in a cloud-synchronised plaintext folder, or treat the
workstation copy as the backup. Continue with [Backup and
recovery](/operations/backup-recovery).
