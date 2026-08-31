# Debian hardening baseline

The `homelab_base` role aims for a safe, maintainable private-network server. It
does not claim to implement a complete CIS benchmark and it deliberately avoids
network policy until the future Tailscale topology is known.

## Supported systems

The role stops unless the target is Debian 12 or Debian 13 and the control
machine uses a compatible Ansible version. This prevents Ubuntu- or RHEL-specific
assumptions from being applied accidentally.

## Package maintenance

Every run refreshes APT metadata and, by default, performs a distribution
upgrade. It installs:

- AppArmor and its parser utilities;
- Chrony for time synchronisation;
- unattended-upgrades and apt-listchanges;
- OpenSSH server, sudo and basic TLS/download tooling;
- smartmontools for disk health inspection;
- fstrim support through the systemd timer.

Unattended package checks and upgrades run automatically. Automatic reboot is
disabled because rebooting the only server takes down the entire cluster.
Ansible reports `/var/run/reboot-required` so the operator can schedule it.

## SSH policy

SSH hardening is opt-in. When enabled, the managed drop-in applies:

```text
PermitRootLogin no
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
X11Forwarding no
AllowAgentForwarding no
PermitTunnel no
AllowUsers <managed-admin>
```

The generated configuration is validated with `sshd -t` before installation.
The role refuses to enable hardening when no allowed user or public key exists.

### Safe activation procedure

1. Generate or select a dedicated Ed25519 key on the control machine.
2. Add its public half to `homelab_admin_authorized_keys`.
3. Leave `homelab_ssh_hardening_enabled: false` and run `homelabctl node prepare`
   once.
4. Open a new terminal and prove `homelabctl node connect titan` works without a
   password.
5. Keep that session open.
6. Set `homelab_ssh_hardening_enabled: true`.
7. Run `homelabctl node prepare` again.
8. Prove a second `homelabctl node connect titan` session works before closing
   the existing session.

The managed key list is exclusive. Always keep at least one tested recovery key.

## Kubernetes host settings

Swap is disabled immediately and persistent swap entries in `/etc/fstab` are
commented out. Sleep, suspend and hibernation targets are masked so the mini PC
cannot disappear because of desktop-style power management.

## Logs and storage care

Systemd journal storage is persistent, compressed, limited to 1 GiB and retained
for at most 14 days. `fstrim.timer` is enabled for SSD maintenance. Smartmontools
is installed, but alerting and SMART scrape integration remain future work.

## Operator shell prompt

Titan enables the role's small Bash login prompt and identifies itself as
`[user@titan]`. This is an operational guardrail, not user-shell ownership:
Ansible manages one `/etc/profile.d` file and one marked source block in the
administrator's `.bashrc`; aliases, Git integration and all other personal
shell configuration remain untouched. Set
`homelab_base_shell_prompt_enabled: false` to remove both managed pieces.

## Firewall boundary

The inventory currently sets `manage_firewall: false`. The role refuses to
proceed when UFW, firewalld or the nftables service is already active but not
managed. This prevents an unknown partial firewall from producing an unreliable
K3s installation.

This does not mean the host should be exposed. Titan remains behind the home
router with no management port forwards. Host firewall rules will be introduced
with Tailscale because they must account for LAN, tailnet, pod and service CIDRs.

## What Vault does not replace

An in-cluster Vault deployment will manage workload secrets, certificates and
possibly dynamic credentials. It does not replace the initial Debian user, sudo
policy, SSH host keys or the off-cluster credentials needed to recover Titan.
Vault can later issue SSH certificates, but Ansible still owns the host's trust
configuration and Vault itself must have an independent recovery path.
