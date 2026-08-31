# Move Titan from temporary Wi-Fi to wired Ethernet

Titan may use Wi-Fi during the Debian installer and initial repair, but wired
Ethernet is its supported long-term management and K3s network. Do not install
K3s until Ethernet survives a reboot, owns the default route and has a router
DHCP reservation.

This page documents the first Titan installation, including the newer Realtek
RTL8125D revision that Debian 13's original Linux 6.12 kernel detected but could
not drive.

## Understand the interface names

Inspect networking without changing it:

```bash
ip -br link
ip -br address
ip route
```

Common prefixes are:

| Name | Meaning |
| --- | --- |
| `lo` | Local loopback; not a physical network |
| `wl...` | Wi-Fi |
| `en...` | Ethernet, such as Titan's `eno1` |

A disconnected but supported Ethernet adapter still appears in `ip -br link`
as `DOWN` or `NO-CARRIER`. If no `en...` device appears, investigate the driver
before changing DHCP configuration or blaming the cable.

## Recover administrator access first

The Debian installer account must be able to use `sudo` before remote
automation begins. If `sudo` is absent or the user is not authorised, use the
physical console and the installer root password:

```bash
su -
apt update
apt install sudo
usermod -aG sudo YOUR_OPERATOR_USER
systemctl reboot
```

After reboot, log in as the operator and verify:

```bash
groups
sudo -v
```

The group list must include `sudo`. If the root account was deliberately left
locked and no sudo-capable user exists, use Debian recovery mode rather than
weakening SSH or enabling direct root login.

## RTL8125D on the Debian 13 installation kernel

Titan's onboard controller identifies as Realtek PCI device `10ec:8125`. On the
initial Linux 6.12 kernel, these checks showed that `r8169` loaded but did not
bind:

```bash
lspci -nnk | grep -A 3 -i ethernet
modprobe r8169
dmesg | grep -iE 'r8169|rtl8125|firmware'
```

The decisive message was:

```text
error -ENODEV: unknown chip XID 689
```

XID `689` is an RTL8125D revision supported by the in-tree `r8169` driver from
Linux 6.14 onward, as shown in the
[upstream Linux 6.14 driver](https://github.com/torvalds/linux/blob/v6.14/drivers/net/ethernet/realtek/r8169_main.c#L2292).
Installing `firmware-realtek` alone cannot add a chip ID that the kernel driver
does not recognise.

Prefer a signed Debian backports kernel over an out-of-tree vendor DKMS module.
It continues to work with Secure Boot and avoids rebuilding a third-party
module after every kernel update. From a root console, add only the backports
source:

```bash
printf '%s\n' \
  'deb https://deb.debian.org/debian trixie-backports main non-free-firmware' \
  > /etc/apt/sources.list.d/trixie-backports.list
apt update
apt install firmware-realtek
apt-cache policy linux-image-amd64
```

Confirm that the backports candidate is Linux 6.14 or newer, then install only
the kernel metapackage and its required dependencies:

```bash
apt install -t trixie-backports linux-image-amd64
systemctl reboot
```

Do not perform a general `-t trixie-backports` upgrade. After reboot:

```bash
uname -r
lspci -nnk -s 02:00.0
ip -br link
```

The PCI output must report `Kernel driver in use: r8169`, and an `en...`
interface must exist. Keep the previous kernel installed as a console recovery
option in GRUB until the new kernel and Ethernet path have been verified.

## Configure wired DHCP

The installer configures only the interface used during installation. When
Wi-Fi was selected, add Ethernet without removing the working fallback yet.
From a root shell, edit `/etc/network/interfaces` so it contains:

```text
source /etc/network/interfaces.d/*

auto lo
iface lo inet loopback

# Stable Titan management and K3s interface
auto eno1
iface eno1 inet dhcp

# Temporary installer fallback; remove after Ethernet is verified
allow-hotplug wlp3s0
iface wlp3s0 inet dhcp
    wpa-ssid "YOUR_PRIVATE_SSID"
    wpa-psk "YOUR_PRIVATE_PASSPHRASE"
```

Interface names vary by hardware; use the names reported locally rather than
copying `eno1` or `wlp3s0` to another machine. Protect a file containing a Wi-Fi
credential:

```bash
chmod 600 /etc/network/interfaces
ifup eno1
```

Verify carrier, addresses and route preference:

```bash
cat /sys/class/net/eno1/carrier
ip -br address
ip route
```

Carrier `1` proves the NIC sees a cable and active switch/router port. Ethernet
must have a private address. When both interfaces temporarily have a default
route, Ethernet must have the lower metric and therefore be preferred.

## Reserve the wired address

Read the Ethernet MAC only at the local console:

```bash
cat /sys/class/net/eno1/address
```

In the router, reserve the current wired address for that MAC and label it
`titan`. Confirm that no port forward exposes Titan. Reboot once and verify that
`eno1` receives the reservation before disabling Wi-Fi.

Do not commit the address or MAC to the public inventory example. Put the
reserved address in the Git-ignored `ansible/inventory/hosts.yml` later.

## Disable Wi-Fi after wired verification

Keep the physical console open. While the Wi-Fi stanza still exists, disconnect
it cleanly:

```bash
ifdown wlp3s0
```

Remove the `allow-hotplug wlp3s0`, `iface wlp3s0`, `wpa-ssid` and `wpa-psk`
lines from `/etc/network/interfaces`, then ensure the interface is down:

```bash
ip link set wlp3s0 down
ip -br address
ip route
```

Only `eno1` should retain a LAN address and default route. Reboot and repeat the
checks before moving to SSH or Ansible.

Removing the stanza stops automatic Wi-Fi association; it does not remove the
driver or make recovery difficult. Firmware or BIOS radio disablement is
optional and may also disable Bluetooth needed by a future workload.

## Re-enable Wi-Fi for recovery

Use this only from Titan's physical console when wired networking is unavailable.
Edit `/etc/network/interfaces` and restore a local, private stanza:

```text
allow-hotplug wlp3s0
iface wlp3s0 inet dhcp
    wpa-ssid "YOUR_PRIVATE_SSID"
    wpa-psk "YOUR_PRIVATE_PASSPHRASE"
```

Then:

```bash
chmod 600 /etc/network/interfaces
ifup wlp3s0
ip -br address
ip route
```

If Wi-Fi was explicitly radio-blocked earlier, run `rfkill unblock wifi` before
`ifup`. It is not needed when the interface was only brought down with `ifdown`
or `ip link set ... down`.

If Ethernet is also active, confirm that its default route remains preferred.
Remove the Wi-Fi stanza again after wired recovery.

Never photograph, paste into a task, or commit a file containing the SSID and
passphrase. If a credential is exposed, rotate it at the access point and
update every authorised client.
