# Testing and changing Ansible

Ansible changes can affect login, storage, packages and the Kubernetes control
plane. Repository validation, preview and physical-node verification are
separate gates.

## Recreate pinned dependencies

```bash
homelabctl setup ansible --reset
```

Run this after changing Python or collection requirements and on a new
workstation. It removes generated runtime state, recreates the local virtual
environment and installs collections under the repository; it does not contact
Titan or remove private inventory. See [reset or uninstall](/ansible/reset-uninstall)
for the exact boundary.

## Static validation

```bash
homelabctl ci check --only ansible
```

The current check performs:

- offline `ansible-lint` across local Ansible content;
- syntax checks for every local playbook;
- syntax checks against committed `hosts.example.yml`, never private inventory.
- a local fstab regression playbook proving that all swap entries are commented
  while root and EFI filesystem mounts remain byte-for-byte intact.

Generated runtime directories and downloaded upstream collections are excluded
from local lint. The repository skips only the role-prefix variable rule because
the pinned upstream collection and inventory use established unprefixed names.

When adding a local playbook, also add it to the playbook list used by
`homelabctl ci check`; syntax coverage is currently explicit rather than file
discovery.

## Remote preview

For base-role changes:

```bash
homelabctl node prepare --check --limit titan --ask-become-pass
```

Read the complete diff and recap. Check mode cannot faithfully predict every
APT operation, command or handler, and it does not make an unsafe SSH policy
safe. Keep a physical console or established recovery session available for
authentication and network changes.

Package installation is simulated in check mode, so the base role reports but
defers enabling Chrony and `fstrim.timer` until the real apply. A preview must
still finish with `failed=0`. Reject any storage diff that changes a non-swap
mount; the intended `/etc/fstab` diff comments only a line whose third field is
`swap`.

There is no generic K3s installation check mode in the supported interface.
Test lifecycle changes against a disposable node or recovery rehearsal before
Titan when their effect cannot be established statically.

## Change checklist

For a local role or playbook change:

1. identify the target hosts and whether the task requires root;
2. use idempotent modules and explicit file ownership/modes;
3. validate external configuration before replacing it;
4. add a safety assertion before lockout, data loss or network-risk changes;
5. scope `no_log` only to tasks that handle secret values;
6. expose the workflow through a typed `homelabctl` command;
7. update static validation coverage and CLI tests or dry-run tests;
8. update the Ansible manual and affected operator runbooks;
9. run static checks, remote preview where supported, then a bounded apply;
10. run a second apply and investigate unexpected repeated changes.

## Upgrade Python dependencies

Change exact versions in `requirements.txt` deliberately. Then:

```bash
homelabctl setup ansible
homelabctl ci check --only ansible
```

Review the Ansible core porting guide and lint rule changes between the old and
new versions. Do not weaken lint globally merely to make an upgrade pass.

## Upgrade the K3s collection

The upstream collection is a Git commit, not a floating branch. To upgrade it:

1. select a specific upstream commit;
2. review changes between the old and proposed commits, especially server,
   agent, upgrade, kubeconfig and token tasks;
3. update the commit in `requirements.yml`;
4. run `homelabctl setup ansible`;
5. run `homelabctl ci check --only ansible`;
6. compare upstream variable assumptions with the private and example inventory;
7. test bootstrap, idempotent rerun, upgrade and reboot on a disposable target;
8. update this manual with changed behaviour.

Do not update the collection automatically during host maintenance or K3s
upgrade. Automation code and cluster version are related changes but distinct
review gates.

## Upgrade K3s itself

Change exact `k3s_version` in both private and example inventories, without
changing dependency pins unless required by reviewed compatibility evidence.
Then follow the recovery-first sequence in [cluster lifecycle and
recovery](/homelabctl/cluster-lifecycle#controlled-k3s-upgrade).

The upstream upgrade playbook can preserve the server-generated token when
inventory omits `token`. Do not add the token to inventory for convenience.

## Current test limitations

Repository checks prove lint, syntax, Go command construction and docs rendering.
They do not currently provide:

- Molecule role scenarios;
- disposable VM integration tests in CI;
- a real multi-server etcd upgrade test;
- a remote Hetzner/Tailscale join test;
- an automated K3s restore rehearsal;
- evidence that automation has run successfully on Titan.

The [current-state page](/project/current-state) must preserve that distinction
until physical execution and recovery rehearsals occur.
