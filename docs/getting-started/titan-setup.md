---
prev:
  text: Understand the setup journey
  link: /getting-started/overview
next:
  text: Record the verified current state
  link: /project/current-state
---

# Titan setup runbook

<RunbookHero
  :checkpoints="16"
  label="Physical foundation"
  description="Build Titan from bare metal to a verified, recoverable single-node K3s cluster without exposing its management plane."
/>

Follow the checkpoints in order. Stop when one fails; later automation assumes
every earlier layer is trustworthy. Pocket ID, Vault, observability, Home
Assistant, Zigbee and other workloads are later phases and must not be mixed
into host bootstrap.

<SetupOverview :phases="[
  { eyebrow: 'Checkpoints 1–6', title: 'Prepare the machine', detail: 'Hardware, firmware, Debian and the home network.', href: '#1-gather-what-you-need' },
  { eyebrow: 'Checkpoints 7–11', title: 'Establish trust', detail: 'Operator tooling, inventory, host identity, SSH and sudo.', href: '#7-bootstrap-homelabctl-on-the-operator-workstation' },
  { eyebrow: 'Checkpoints 12–14', title: 'Build the node', detail: 'Debian baseline, safe hardening and pinned K3s.', href: '#12-preview-and-apply-the-debian-baseline' },
  { eyebrow: 'Checkpoints 15–16', title: 'Prove recovery', detail: 'Off-node export and final acceptance evidence.', href: '#15-create-the-first-off-node-recovery-set' }
]" />

## 1. Gather what you need

Before erasing or installing anything, have:

- the AMD mini PC, its power supply and an Ethernet cable;
- a monitor and keyboard for installation and lockout recovery;
- an 8 GB or larger USB installer drive;
- access to the home router's DHCP reservation settings;
- a second computer for the repository and `homelabctl`;
- encrypted storage or a password manager outside Titan for recovery material;
- the mini PC vendor's current firmware and recovery instructions;
- enough time to finish through the first off-node recovery export.

Record the mini PC model, serial number, Ethernet MAC address, firmware version,
SSD model and recovery-key location. Do not store passwords or private keys in
the repository.

Prefer wired Ethernet for the server. Do not create router port forwards for
SSH, Kubernetes, ingress or application ports.

## 2. Decide the disk-encryption trade-off

Full-disk encryption protects a stolen device but requires a person or a
separately designed remote-unlock mechanism after every reboot. Titan cannot
provide home automation while waiting at an unlock prompt.

Make and record this decision before installation:

- use encryption when physical theft is the stronger risk and console unlock is
  reliably available;
- omit encryption when unattended recovery after power loss is required;
- never place the disk recovery key only inside Titan or its future Vault.

This repository does not automate disk unlocking.

## 3. Configure firmware

At the physical console:

1. update firmware using the vendor-supported procedure;
2. load stable defaults after the update if the vendor requires it;
3. enable automatic power-on after AC power is restored;
4. enable AMD virtualisation for possible future VM use;
5. keep Secure Boot enabled when supported by the installer and hardware;
6. disable firmware-level sleep or scheduled power-off features;
7. select USB temporarily for installation, then restore the internal SSD as
   the first normal boot device;
8. confirm the system clock is approximately correct.

Do not enable public remote-management features in firmware. Record the final
settings without capturing passwords.

## 4. Prepare and verify Debian media

Download the Debian 13 AMD64 installer from an official Debian mirror. Verify
its checksum against Debian's separately published checksum before writing it
to USB. Use the workstation's trusted imaging tool, eject the drive cleanly and
boot Titan from it.

Checksum verification and USB imaging are workstation bootstrap operations, so
they are outside `homelabctl`. Do not use an unverified image.

## 5. Install Debian

Use these installer choices:

| Installer item | Required choice |
| --- | --- |
| Hostname | `titan` |
| Domain | Blank unless a real internal DNS domain already exists |
| Network | Wired DHCP |
| User | One normal operator account; do not operate as root |
| Privilege | Ensure the operator can use `sudo` |
| Partitioning | Guided whole-disk unless preserving a reviewed layout |
| Package selection | SSH server and standard system utilities |
| Desktop | None |
| Bootloader | Internal SSD |

Use a unique operator password and store it in the off-cluster password
manager. Do not enable direct root SSH. The operator account is the initial
Ansible administrator; application users belong inside Kubernetes later.

After installation, remove the USB drive, reboot from the SSD and keep the
monitor attached until SSH hardening is proven.

## 6. Establish the home-network identity

At Titan's console, record:

- the wired Ethernet MAC address;
- the DHCP address and prefix;
- the default gateway;
- the DNS servers;
- the OpenSSH Ed25519 host-key fingerprint.

If Debian was installed over Wi-Fi or the Ethernet device is absent, complete
the [temporary Wi-Fi to wired migration](/getting-started/titan-networking)
before reserving an address or installing K3s.

Console commands are an unavoidable trust-bootstrap exception to the normal CLI
contract. Use the installed system tools to inspect addresses, routes, SSH and
the public host-key fingerprint; never display or copy a private host key.

In the router:

1. reserve Titan's current address for its Ethernet MAC;
2. choose an address inside the home LAN but outside the router's dynamic pool
   when the router requires that convention;
3. give the reservation the description `titan`;
4. confirm there are no port forwards to Titan;
5. reboot Titan or renew the lease, then confirm it receives the reservation.

The committed inventory uses `192.168.1.50` only as an example. Use the actual
reservation for this network.

## 7. Bootstrap homelabctl on the operator workstation

Clone or open the repository on the operator workstation. Install the
checksum-verified platform binary using the [release installation
procedure](/homelabctl/releases-update), then confirm it:

```bash
homelabctl version
homelabctl update --check
```

Building from source with Go 1.27 remains a contributor fallback, not the
normal Titan bootstrap path.

Do not install or run Ansible yet. The first trust path needs only
`homelabctl`, OpenSSH and `ssh-copy-id`; host identity, key access and sudo are
proved before configuration automation is allowed to connect.

## 8. Prepare an operator SSH key

Use an existing protected Ed25519 key if it is appropriate for administering
Titan. Otherwise generate a dedicated key with the workstation's trusted
OpenSSH tooling:

```bash
ssh-keygen -t ed25519 -a 100 \
  -f "$HOME/.ssh/homelab_titan_ed25519" \
  -C "homelab operator"
```

Key generation is a one-time user-identity bootstrap operation and deliberately
remains outside the repository. Give the private key a strong passphrase, add
it to the workstation's normal SSH agent or keychain, and do not overwrite an
existing key. The private file is `homelab_titan_ed25519`; only the corresponding
`.pub` file is installed on Titan.

## 9. Create the private inventory

Create, but do not overwrite, the Git-ignored inventory:

```bash
homelabctl inventory init
```

Edit `ansible/inventory/hosts.yml`. At minimum, set Titan's real address,
installer username and complete public key:

```yaml
titan:
  ansible_host: 192.168.1.50
  ansible_user: your-debian-user
  ansible_ssh_private_key_file: /absolute/path/to/.ssh/homelab_titan_ed25519
  homelab_base_manage_hostname: true
  homelab_base_hostname: titan
  homelab_base_shell_prompt_enabled: true
  homelab_ssh_hardening_enabled: false
  homelab_admin_authorized_keys:
    - "ssh-ed25519 AAAA... operator@workstation"
```

Preserve the example's `server_config_yaml`, labels, exact `k3s_version`, empty
agent group and cluster-wide variables. Do not add the K3s token, sudo password,
SSH private key or future Vault recovery material.

Render the inventory without contacting Titan:

```bash
homelabctl inventory show
```

It must show one server named `titan` and no agents.

## 10. Trust Titan and install the operator key

Open the first password-authenticated session:

```bash
homelabctl node connect titan
```

Compare the offered Ed25519 fingerprint with the value recorded at Titan's
physical console. Accept it only if they match. A mismatch means the address,
installation or device identity must be investigated.

Install only the public key through the typed bootstrap command:

```bash
homelabctl node authorize-key titan \
  --public-key "$HOME/.ssh/homelab_titan_ed25519.pub"
```

`authorize-key` validates that the selected file has a supported OpenSSH public
key shape, resolves Titan from the private YAML inventory in Go and delegates
password-authenticated installation to `ssh-copy-id`. It refuses a private-key
file and does not require Ansible.

Keep the console and original SSH session open. In a new terminal, prove this
logs in with the key:

```bash
homelabctl node connect titan
```

If it still requests the account password, fix SSH key selection before running
Ansible or enabling hardening.

## 11. Validate remote access and sudo

From the new key-authenticated `homelabctl node connect titan` session, run on
Titan:

```bash
id
sudo -v
sudo id
hostnamectl
timedatectl
ip -br address
ip route show default
getent ahosts deb.debian.org
systemctl --failed
```

The first `id` must identify the normal operator. `sudo id` must identify
`root`. Exit and open one more new key-authenticated session before proceeding.
The account password may be used for sudo, but it is never placed in inventory.
The remaining checks must show hostname `titan`, synchronized time, `eno1` up
with the reserved address, no Wi-Fi address, the default route through `eno1`,
working DNS and no unexplained failed units.

If Bash reports that it cannot change to `en_US.UTF-8`, the Mac is forwarding a
locale that the minimal Debian installation has not generated. The baseline
generates both `en_US.UTF-8` and `en_GB.UTF-8` and selects the US locale by
default. It normalises both `/etc/default/locale` and `/etc/locale.conf` so a
stale installer `LANGUAGE=en_GB:en` value cannot override that choice.
For access needed before the first baseline run, repair it interactively on
Titan with:

```bash
sudo dpkg-reconfigure locales
```

Enable `en_US.UTF-8 UTF-8`, retain any other desired locale, and select
`en_US.UTF-8` as the default. Verify `locale -a` contains `en_US.utf8`, then
reconnect. Do not set `LC_ALL` globally in `.bashrc`.

## 12. Preview and apply the Debian baseline

Only now install the pinned Ansible environment and validate it locally:

```bash
homelabctl setup ansible
homelabctl ci check --only ansible
homelabctl doctor
homelabctl inventory check --verbose
homelabctl node diagnose --limit titan --ask-become-pass
```

`doctor` reports tooling for the whole repository, so missing Docker, Node,
Terraform or Helm components do not block host preparation. The diagnostic must
confirm successful Ansible ping, hostname `titan`, active SSH, valid SSH
configuration, working time synchronisation, a default route, free disk space
and no unexplained failed services.

Preview supported changes, then apply after reviewing the target and diff:

```bash
homelabctl node prepare --check --limit titan --ask-become-pass
homelabctl node prepare --limit titan --ask-become-pass
```

The role performs Debian package upgrades, installs the administration
baseline, generates the managed UTF-8 locales, enforces Titan's hostname,
manages the declared key set, disables swap and sleep, bounds persistent logs,
enables automatic security updates without automatic reboot, and enables
Chrony and SSD trimming.

There must be no failed or unreachable tasks. Open a new
`homelabctl node connect titan` session after the apply; its prompt should begin
with `[iamkhattar@titan]`. The role adds only a marked prompt-loader block to the
administrator's `.bashrc` and leaves all other personal shell configuration
untouched.

If a reboot is reported before K3s is installed:

```bash
homelabctl node reboot --limit titan --ask-become-pass
homelabctl inventory check
homelabctl node prepare --limit titan --ask-become-pass
```

The second preparation run should be mostly unchanged.

## 13. Enable SSH hardening without lockout

Keep one working session and the physical console available. Confirm a fresh
`homelabctl node connect titan` session still uses the managed key. Then change
only this private inventory value:

```yaml
homelab_ssh_hardening_enabled: true
```

Apply and test again:

```bash
homelabctl node prepare --limit titan --ask-become-pass
homelabctl node connect titan
homelabctl inventory check
```

Do not close the recovery session until two separate post-change connections
work. The managed policy disables root, password and keyboard-interactive SSH,
and limits access to declared users. Adding or removing administrators later is
an inventory change followed by `node prepare`; Vault does not replace this
bootstrap trust path.

## 14. Bootstrap K3s

Install the pinned K3s version through the official pinned Ansible collection:

```bash
homelabctl cluster bootstrap --ask-become-pass
```

Do not use check mode for installation. The first server generates its own
cluster token; no token belongs in inventory or Terraform state.

Verify the result:

```bash
homelabctl cluster status --all-pods
homelabctl cluster nodes
homelabctl cluster diagnose --ask-become-pass
homelabctl cluster snapshot list --ask-become-pass
```

Acceptance criteria:

- the only node is `titan` and it is `Ready`;
- it has control-plane and etcd roles and can also schedule workloads;
- K3s is active and core pods settle to Running or Completed;
- the workstation has the `homelab` kubeconfig context;
- Kubernetes secrets encryption is configured;
- bundled Traefik is absent by design;
- embedded-etcd snapshot configuration is active.

::: info Titan bootstrap verification — 2026-08-31
The workstation observed Titan Ready on K3s `v1.36.4+k3s1` as the sole
`control-plane,etcd` node. CoreDNS and local-path-provisioner were Running, the
node carried the intended `homelab.io/location=home` and
`homelab.io/hardware=mini-pc` labels, and bundled Traefik was absent. Recovery
export and managed-reboot acceptance remain outstanding.
:::

Treat kubeconfig as an administrator credential.

## 15. Create the first off-node recovery set

Use a private staging directory outside the repository:

```bash
homelabctl cluster recovery export \
  --destination /secure/homelab-recovery \
  --name first-install \
  --ask-become-pass
```

This creates a fresh snapshot and fetches it with the server token without
printing the token. Verify the files, encrypt the exported directory, copy it
to storage that is neither Titan nor the operator workstation, verify that copy
and remove the plaintext staging copy.

Record together:

- the repository revision used for installation;
- the exact K3s version;
- Titan's inventory values without embedding secrets in Git;
- the recovery export date and encrypted-storage location;
- the disk-encryption recovery location, if applicable.

The installation is not complete until another device or medium holds the
verified recovery set.

## 16. Final acceptance checklist

Before disconnecting the console, confirm:

- [ ] firmware, power-restore and disk-encryption decisions are recorded;
- [ ] Debian 13 boots from the internal SSD without a desktop;
- [ ] the hostname is `titan` and the router reservation survives reboot;
- [ ] no router management port is forwarded to Titan;
- [ ] the router IPv6 firewall blocks unsolicited inbound traffic and has no
      rule or pinhole exposing Titan;
- [ ] the normal operator has working key-only SSH and sudo;
- [ ] a second session works after SSH hardening;
- [ ] `node prepare` is idempotent and diagnostics have no unexplained failure;
- [ ] Titan is the single Ready K3s server and worker;
- [ ] core pods and the K3s service are healthy;
- [ ] a fresh snapshot and server token exist in verified encrypted off-node
      storage;
- [ ] the physical console remains accessible for recovery.

Update [current state](/project/current-state) only after these checks have been
performed against the physical machine. Complete a
[change and evidence record](/operations/change-evidence) for the first build,
then proceed through the project roadmap one recoverable layer at a time. Do
not jump directly to Vault or Home Assistant before storage, ingress, DNS and
backup decisions are complete.

Before starting the platform bootstrap, record these deployment decisions:

- the encrypted off-node backup destination and who can recover its key;
- the private DNS records for `home.6940469.xyz` and
  `*.home.6940469.xyz`, both resolving to Titan's reserved LAN address;
- the off-cluster Alertmanager receiver to be tested before alerts are trusted;
- the Vault recovery share count, threshold, encrypted recipients and custody
  locations.

DNS-01 automation and the Zigbee integration choice are later decisions. They
do not block the initial private-PKI platform or the recoverable K3s foundation.

## If a checkpoint fails

Do not reinstall or reset K3s reflexively. Work upward from router and address,
to SSH identity, sudo, Debian health, K3s and Kubernetes. Use the
[troubleshooting runbook](/operations/troubleshooting) and preserve the physical
console and any working SSH session during access-policy changes.
