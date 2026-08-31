# Troubleshooting

Work from the lowest layer upward: network, SSH, sudo, Debian, K3s, then
Kubernetes workloads.

## Wired Ethernet is missing after installation

An unsupported NIC revision does not appear in `ip -br link`, even with a good
cable. Titan's RTL8125D reported `unknown chip XID 689` under the Debian 13
installation kernel and requires the signed backports kernel documented in the
[wired migration runbook](/getting-started/titan-networking#rtl8125d-on-the-debian-13-installation-kernel).

If the Ethernet interface exists but reports `NO-CARRIER`, test the cable and
router port. If it has carrier but no address, inspect the persistent DHCP
stanza before changing drivers.

## Ansible cannot reach Titan

Check the router reservation and local connectivity:

```bash
homelabctl inventory show
homelabctl inventory check --verbose
homelabctl node connect titan
```

Confirm `ansible_host`, `ansible_user` and `ansible_port` in `hosts.yml`. Do not
disable host-key checking to hide a fingerprint mismatch. A changed key may mean
Titan was reinstalled, its address was reassigned, or a different device is
answering.

## Sudo fails

Test the managed privilege path directly:

```bash
homelabctl node diagnose --limit titan --ask-become-pass
```

Use `--ask-become-pass` when the account requires a sudo password. Confirm the
account belongs to the `sudo` group on Debian.
If no sudo-capable account exists, repair membership from the physical root
console using the [administrator recovery procedure](/getting-started/titan-networking#recover-administrator-access-first).

## SSH hardening would lock out the operator

Do not close the working session. Set
`homelab_ssh_hardening_enabled: false` in the private inventory and run
`homelabctl node prepare`. Validate the public key, username and permissions
before trying again.

Keep the physical Titan console available until two new managed connections
succeed:

```bash
homelabctl node diagnose --limit titan --ask-become-pass
homelabctl node connect titan
```

## An unmanaged firewall is detected

The preparation role stops intentionally if UFW, firewalld or the nftables
service is active while `manage_firewall` is false. Determine who configured the
firewall and inspect its rules. Do not blindly flush rules on a remote host.

For the initial LAN-only installation, remove or disable the unmanaged policy
from Titan's local console. Managed rules will be designed with Tailscale later.

## K3s does not start

Collect the standard read-only evidence bundle:

```bash
homelabctl cluster diagnose --ask-become-pass
```

Do not publish logs containing tokens or credentials. Compare the rendered
configuration with the inventory and check disk capacity, time synchronisation
and port conflicts.

## Node is NotReady

```bash
homelabctl cluster status --all-pods
homelabctl cluster diagnose --ask-become-pass
```

Look for disk pressure, networking failures, certificate/time errors and pods
that cannot mount required storage.

## Upgrade fails

Stop and preserve evidence. Do not repeatedly reset or reinstall K3s. Record:

```bash
homelabctl cluster diagnose --ask-become-pass
homelabctl cluster snapshot list --ask-become-pass
```

Confirm the off-node snapshot and server token are accessible. Use the K3s
documentation matching both the previous and target versions before deciding
whether to retry, roll forward or restore.
