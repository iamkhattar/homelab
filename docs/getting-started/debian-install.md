# Install Debian on Titan

Use the current Debian 13 stable installer for the AMD64 architecture. Verify
the installer image checksum against Debian's published checksum before writing
it to USB.

## Firmware settings

Before installing, review the mini PC firmware or BIOS:

- install available vendor firmware updates;
- enable automatic power-on after AC power is restored;
- enable AMD virtualisation if virtual machines may be used later;
- keep Secure Boot enabled when the hardware and installer support it;
- select the internal SSD as the first normal boot device after installation.

## Installer choices

Use these values when the installer asks:

| Prompt | Value |
| --- | --- |
| Hostname | `titan` |
| Domain | Leave blank unless a local DNS domain is already managed |
| User | A normal personal administrator account, not `root` |
| Software | SSH server and standard system utilities |
| Desktop environment | Do not install |

Use guided partitioning unless there is a specific storage layout to preserve.
Full-disk encryption protects data if the device is stolen, but it also requires
someone to unlock Titan after every reboot. Decide based on that availability
trade-off before installing.

## Network setup

Allow the installer to use DHCP. After the first boot, find Titan's MAC address
and assigned address, then create a DHCP reservation in the router. The example
Ansible inventory uses `192.168.1.50`, but the actual address must fit the home
network and must not collide with another reservation.

Do not configure a public address or router port forwarding.

## First-boot checks

Log in at Titan's physical console. Confirm that the login screen identifies the
host as `titan`, note its private LAN address, confirm the router supplied a
default route, and make sure the installer-selected SSH service started. These
are the only physical-console checks before the machine enters managed state.

After creating the private inventory and accepting Titan's verified host key,
collect the repeatable checks from the workstation:

```bash
homelabctl inventory check
homelabctl node diagnose --limit titan --ask-become-pass
```

Expected results:

- the static hostname is `titan`;
- a private LAN address is assigned;
- the default route points at the home router;
- the SSH service is active;
- time synchronisation is available.

If the hostname was entered incorrectly, keep hostname management enabled for
Titan in the private inventory. `homelabctl node prepare` will set the hostname
and ensure `/etc/hosts` contains:

```text
127.0.0.1 localhost
127.0.1.1 titan
```

Ansible also enforces these two hostname settings for Titan. Hostname management
is opt-in, so future nodes are not renamed to `titan`.

## Account bootstrap

The Debian installer account is the initial managed administrator. Confirm its
sudo path from the workstation:

```bash
homelabctl node diagnose --limit titan --ask-become-pass
```

Keep password login available for the initial connection. The Ansible role only
disables it after a public key has been supplied explicitly.
