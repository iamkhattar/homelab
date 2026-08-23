# Inventory and node management

Inventory commands manage the boundary between repository-wide automation and
machine-specific facts. Node commands then use that inventory for SSH and
Ansible operations.

## Create the private inventory

```bash
homelabctl inventory init
```

This copies `ansible/inventory/hosts.example.yml` to
`ansible/inventory/hosts.yml` with mode `0600`. The destination is Git-ignored,
and the command refuses to overwrite an existing file.

Edit at least these Titan values before connecting:

```yaml
titan:
  ansible_host: 192.168.1.50
  ansible_user: operator
  homelab_base_manage_hostname: true
  homelab_base_hostname: titan
  homelab_base_shell_prompt_enabled: true
  homelab_base_shell_prompt_environment: HOME
```

Keep the inventory key and managed hostname as `titan`. Replace the address with
the router reservation and the user with the normal Debian installer account.
Prompt management is also host-scoped: Titan displays `[HOME | titan]`, while
other nodes remain untouched unless they opt in. Do not store private keys,
sudo passwords or the K3s token in this file.

## Inspect effective membership

```bash
homelabctl inventory show
```

The output should contain one host under `server`, named `titan`, and an empty
`agent` group. This command renders the effective Ansible graph, including
group nesting, without contacting a node.

## Establish the first SSH trust

```bash
homelabctl node connect titan
```

The CLI resolves `ansible_host`, `ansible_user` and `ansible_port` from the
effective inventory, then opens an ordinary interactive SSH connection. On the
first connection, compare the displayed Ed25519 host-key fingerprint with the
fingerprint obtained from Titan's physical console. Do not accept an identity
that has not been compared through that separate path.

Install the selected public key through the bootstrap command, then prove a new
session uses it before Ansible manages the authoritative key set:

```bash
homelabctl node authorize-key titan \
  --public-key "$HOME/.ssh/homelab_titan_ed25519.pub"
homelabctl node connect titan
```

The command accepts only a supported OpenSSH public-key file and delegates the
password-authenticated installation to `ssh-copy-id`. Private key material is
never copied to Titan.

Later host-key changes stop both SSH and Ansible. Investigate them rather than
deleting the known-host entry automatically.

## Check connectivity

```bash
homelabctl inventory check
```

This first renders the inventory graph and then runs Ansible's non-mutating ping
module against `k3s_cluster`. For connection-level detail:

```bash
homelabctl inventory check --verbose
```

Verbose output can contain private addresses and usernames. Review it before
sharing logs.

## Preview and apply the Debian baseline

```bash
homelabctl node prepare --check --limit titan --ask-become-pass
homelabctl node prepare --limit titan --ask-become-pass
```

`--check` adds Ansible check and diff modes. It is a useful preview, but package
and command tasks cannot always predict every change. The normal command
refreshes and upgrades Debian packages and applies the local `homelab_base`
role.

`--ask-become-pass` prompts through Ansible and never stores the sudo password.
Omit it after deliberately configuring passwordless sudo; do not put the
password in inventory or command-line variables.

`--limit` accepts an inventory host or group. Use it when intentionally
operating on Titan alone; omit it when the same baseline should apply to every
managed node.

## Activate SSH hardening safely

The example inventory keeps hardening disabled. The safe sequence is:

1. add at least one complete public key to
   `homelab_admin_authorized_keys`;
2. keep `homelab_ssh_hardening_enabled: false` and run `node prepare`;
3. open a second terminal and prove `node connect titan` uses the key;
4. keep the successful connection open;
5. enable hardening and run `node prepare` again;
6. prove another new connection before closing the recovery session.

The Ansible role refuses to disable password authentication with an empty key
list. The full policy is documented in [Debian hardening](/ansible/hardening).

## Reboot before K3s exists

```bash
homelabctl node reboot --limit titan --ask-become-pass
```

This uses the Debian-only reboot playbook, processes hosts serially, waits for
them to return and verifies Ansible connectivity. It is intended for the period
before K3s installation.

After K3s exists, use `homelabctl cluster reboot`; that command includes
Kubernetes-aware ordering and health checks.

## Collect node diagnostics

```bash
homelabctl node diagnose --limit titan --ask-become-pass
```

The diagnostic playbook gathers fixed, read-only evidence:

- hostname and operating-system facts;
- failed systemd units;
- SSH daemon configuration validation;
- filesystem usage;
- system time and synchronisation state.

It does not expose a generic remote execution endpoint. Individual diagnostic
commands may fail so the playbook can report as much evidence as possible in
one pass.

## Common failures

| Symptom | Next action |
| --- | --- |
| Inventory graph is empty | Verify `hosts.yml` exists and retains the required groups |
| Host is unreachable | Check the router reservation, address, username and verified host key |
| Sudo fails | Re-run with `--ask-become-pass` and confirm the Debian user belongs to `sudo` |
| Check mode reports changes every time | Inspect the task; some package or command modules cannot fully simulate state |
| SSH fails after hardening | Keep the old session open, disable hardening in inventory and reapply the baseline |
