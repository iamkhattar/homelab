# homelab_base role reference

`homelab_base` turns a supported Debian installation into a predictable K3s
host. It is applied by `homelabctl node prepare` and again at the start of
`homelabctl cluster bootstrap`.

## Variable reference

| Variable | Default | Behaviour |
| --- | --- | --- |
| `homelab_base_apply_package_upgrades` | `true` | Performs a full APT distribution upgrade |
| `homelab_base_disable_swap` | `true` | Disables active swap and comments persistent swap entries |
| `homelab_base_disable_sleep` | `true` | Masks sleep, suspend and hibernation targets |
| `homelab_base_manage_hostname` | `false` | Opts a host into hostname and `/etc/hosts` management |
| `homelab_base_hostname` | inventory hostname | Desired hostname when management is enabled |
| `homelab_base_timezone` | `Etc/UTC` | Configures the system timezone |
| `homelab_base_default_locale` | `en_GB.UTF-8` | Default `LANG` written to `/etc/default/locale` |
| `homelab_base_generated_locales` | `en_GB.UTF-8`, `en_US.UTF-8` | Locales generated for the host and SSH clients |
| `homelab_base_journal_max_use` | `1G` | Bounds persistent systemd journal disk usage |
| `homelab_base_journal_retention` | `14day` | Bounds journal retention time |
| `homelab_base_shell_prompt_enabled` | `false` | Opts a host into the managed Bash login prompt |
| `homelab_base_shell_prompt_environment` | `HOMELAB` | Environment label shown by the managed prompt |
| `homelab_base_packages` | package list below | Complete package set managed present |
| `homelab_admin_user` | `ansible_user` | Existing Debian account managed as administrator |
| `homelab_admin_authorized_keys` | empty list | Exclusive key set when non-empty |
| `homelab_ssh_hardening_enabled` | `false` | Installs the hardened SSH drop-in only after validation |
| `homelab_ssh_allowed_users` | administrator user | Users permitted by the SSH `AllowUsers` directive |

Titan enables hostname and shell-prompt management in its host entry. Future
nodes inherit neither the name `titan` nor the prompt because both management
defaults remain false.

## Managed packages

The default list installs:

- `acl` for filesystem ACL support;
- AppArmor and its utilities;
- `apt-listchanges`, `needrestart` and `unattended-upgrades`;
- CA certificates and `curl`;
- Chrony;
- locale data;
- OpenSSH server and sudo;
- smartmontools.

Changing `homelab_base_packages` replaces the entire role list. An override must
repeat every package that should remain installed; the role does not currently
provide a separate extension list.

## Task order

Role order is intentional because later tasks assume a validated and updated
system.

### 1. Compatibility and packages

The role validates Ansible, distribution and Debian release before refreshing
APT metadata. Metadata is accepted for one hour, and APT lock acquisition waits
up to 120 seconds. If enabled, a distribution upgrade runs before the base
package set is installed.

Package upgrades can require a reboot, but the role never reboots automatically.
It reports the correct `homelabctl node reboot` or `cluster reboot` workflow at
the end.

### 2. Network safety gate

Service facts are gathered before changing host identity or K3s prerequisites.
With the current `manage_firewall: false`, the role stops when it detects a
running UFW, firewalld or nftables service.

::: warning `manage_firewall` does not install rules
Setting `manage_firewall: true` only bypasses the unmanaged-firewall assertion;
the role contains no firewall implementation today. Do not enable it as a way
to make an unknown ruleset pass. Tailscale-aware policy must be designed and
implemented first.
:::

### 3. Locale, host identity and time

The role generates `en_GB.UTF-8` for Titan's UK default and `en_US.UTF-8` for
operator workstations that forward that locale through SSH. It writes only
`LANG` to `/etc/default/locale`; it does not set a global `LC_ALL`, so users and
individual processes can still select more specific locale categories. The
default locale must be present in the generated list or the role stops before
making changes.

When hostname management is enabled, systemd receives the desired hostname and
the `127.0.1.1` entry in `/etc/hosts` is updated. The role then applies the
configured timezone, defaulting to UTC.

Titan also opts into a lightweight Bash login prompt:

```text
[HOME | titan] operator:/current/path $
```

The role installs `/etc/profile.d/20-homelab-prompt.sh`; it does not modify a
user's `.bashrc` or install a prompt framework. Non-root sessions use green for
the identity marker, root uses red, and terminals without colour support use
plain text. The hostname is evaluated by Bash at login, so the prompt template
is reusable: a future node can choose its own environment label without
inheriting `titan`.

### 4. Administrator and SSH keys

The existing administrator is appended to the Debian `sudo` group. When the
authorised-key list is non-empty, it becomes the exclusive managed key set for
that user.

Exclusive management means removing a key from inventory removes it on the next
run. An empty list skips key management rather than erasing all keys, and SSH
hardening cannot be enabled with an empty list or empty allowed-user list.

### 5. SSH hardening

When enabled, the role renders
`/etc/ssh/sshd_config.d/10-homelab-hardening.conf` with mode `0644` and validates
it through `sshd -t` before replacement. The policy disables root login,
password and keyboard-interactive authentication, X11 forwarding, agent
forwarding and tunnels. Public-key authentication remains enabled and
`AllowUsers` limits login to the managed list.

A changed template notifies an SSH reload handler rather than restarting the
daemon. Follow the staged [SSH hardening procedure](/ansible/hardening) to avoid
lockout.

### 6. Kubernetes host prerequisites

Active swap is disabled when present, and non-commented swap entries in
`/etc/fstab` are commented so it stays disabled after reboot. Sleep, suspend,
hibernate and hybrid-sleep systemd targets are masked.

### 7. Logs and automatic maintenance

The role creates a persistent journald drop-in with compression, bounded size
and bounded retention. A changed journal configuration restarts journald.

APT is configured to check package lists and run unattended upgrades daily.
Unused dependencies may be removed, but automatic reboot is explicitly false
because Titan is the only server.

Chrony and `fstrim.timer` are enabled and started. Smartmontools is installed,
but SMART monitoring and alert delivery are not configured yet.

## Idempotency expectations

After a successful apply and reboot, another `node prepare` should be mostly
unchanged. Expected recurring work includes APT metadata checks and newly
available package upgrades. The immediate `swapoff` command reports changed
only when gathered facts show active swap.

Check mode cannot perfectly model package upgrades or arbitrary commands. Treat
`node prepare --check` as a preview, then inspect the real recap for failed,
unreachable and changed counts.

## Files and handlers

| Managed path | Source | Handler |
| --- | --- | --- |
| `/etc/ssh/sshd_config.d/10-homelab-hardening.conf` | `sshd-hardening.conf.j2` | Reload SSH |
| `/etc/systemd/journald.conf.d/10-homelab.conf` | `journald.conf.j2` | Restart journald |
| `/etc/apt/apt.conf.d/20auto-upgrades` | static file | None |
| `/etc/apt/apt.conf.d/52homelab-unattended-upgrades` | static file | None |
| `/etc/hosts` hostname entry | line management | None |
| `/etc/fstab` swap entries | regular-expression replacement | None |
| `/etc/profile.d/20-homelab-prompt.sh` | `homelab-prompt.sh.j2` | None |

Do not edit managed files directly on Titan. Change inventory, defaults,
templates or static role files and apply through `homelabctl`.
