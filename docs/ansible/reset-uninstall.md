# Reset or uninstall Ansible

Ansible has three different kinds of state in this repository. Decide which
one you mean before resetting anything:

| State | Location | Safe reset |
| --- | --- | --- |
| Generated workstation runtime | `ansible/.venv/`, `.ansible/`, `collections/` | `homelabctl setup ansible --reset` |
| Private inventory | `ansible/inventory/hosts.yml` | Preserve or back up separately; runtime reset never touches it |
| Changes already applied to Titan | Debian files, packages, users and K3s on the node | Not removed by uninstalling workstation dependencies |

The committed roles, playbooks and requirements files are source code. None of
the commands on this page delete them.

## Start the local Ansible environment again

Use reset when imports are broken, a Python environment is stale, a collection
upgrade was interrupted, or you want to prove the pinned setup is reproducible:

```bash
homelabctl --dry-run setup ansible --reset
homelabctl setup ansible --reset
homelabctl ci check --only ansible
homelabctl inventory show
homelabctl inventory check
```

Reset removes exactly these generated, Git-ignored paths:

```text
ansible/.venv/
ansible/.ansible/
ansible/collections/
```

It then recreates the virtual environment from `requirements.txt` and installs
the collection commit pinned in `requirements.yml`. It preserves
`inventory/hosts.yml`, repository source files, SSH keys, kubeconfig and all
remote node state.

The dry run prints both the exact paths that would be removed and the setup
commands that would follow. If reset fails halfway through, run the same reset
command again; the generated paths contain no authoritative state.

## Uninstall the repository-local Ansible runtime

Remove Ansible dependencies from this checkout without reinstalling them:

```bash
homelabctl --dry-run setup ansible --uninstall
homelabctl setup ansible --uninstall
```

This removes the same three generated paths and nothing else. It does not
remove system Python, Homebrew or Debian packages, a globally installed
`ansible` executable, another checkout's virtual environment, or documentation
dependencies. `homelabctl` deliberately prefers the repository-local
environment, so Ansible-backed commands will stop working until you run:

```bash
homelabctl setup ansible
```

If Ansible was separately installed with a system package manager or `pipx`,
manage that installation with its owner. `homelabctl` will not guess which
global installation is safe to remove.

## Start over with inventory

Runtime reset and uninstall deliberately preserve
`ansible/inventory/hosts.yml`, because it contains the private address, user and
SSH identity needed to reach Titan. First save that file outside the repository
or rename it to a private backup. Only after verifying the backup, remove the
working `hosts.yml` and recreate the committed template through:

```bash
homelabctl inventory init
homelabctl inventory show
```

`inventory init` refuses to overwrite an existing file. That refusal prevents
an environment reset from silently destroying the only working connection
details. Edit the newly created private inventory before running any command
that contacts a node.

## What local uninstall does not undo

Ansible is declarative automation, not an installation transaction log.
Removing its workstation runtime does not reverse tasks previously applied to
Titan. In particular, it does not:

- restore old SSH or sudo configuration;
- delete managed operator keys or users;
- downgrade or remove Debian packages;
- change Titan's hostname;
- remove K3s, its data directory, snapshots or server token;
- remove Kubernetes workloads.

To change one managed setting, update the desired inventory or role and run a
check-mode preview followed by the normal bounded apply:

```bash
homelabctl node prepare --check --limit titan --ask-become-pass
homelabctl node prepare --limit titan --ask-become-pass
```

Do not run K3s vendor uninstall scripts merely to repair the local Ansible
environment. They remove node-side cluster state and are outside the supported
routine workflow.

## Start the physical node from zero

A genuine node reset means reinstalling Debian, not uninstalling Ansible from
the workstation. Before doing that:

1. export K3s recovery material with `homelabctl cluster recovery export`;
2. encrypt it and move it off Titan;
3. preserve required Home Assistant, Vault and application data separately;
4. verify the recovery files can be read;
5. keep physical console access and the operator SSH public key available;
6. reinstall Debian using the complete [Titan setup runbook](/getting-started/titan-setup).

If the current cluster has data worth preserving, a Debian reinstall is a
destructive recovery operation. A local `--reset` or `--uninstall` is not.
