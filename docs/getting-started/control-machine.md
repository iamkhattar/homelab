# Prepare the control machine

`homelabctl` runs from the operator workstation, not from Titan. It owns the
documented workflow and invokes the repository's pinned Ansible environment.
The workstation needs Go 1.27 or newer, Python 3, Node 24 or newer, and the
tools reported by `homelabctl doctor`.

## Bootstrap the operator CLI

From the repository root, verify `homelabctl` first:

```bash
homelabctl version
homelabctl update --check
```

OpenSSH and `ssh-copy-id` establish the first trust path. Ansible is installed
only after the host fingerprint, operator public key and sudo path have been
proved manually.

## Create the private inventory

```bash
homelabctl inventory init
```

`hosts.yml` is ignored by Git because it contains private network information.
Edit these values before running Ansible:

```yaml
titan:
  ansible_host: 192.168.1.50 # replace with the router reservation
  ansible_user: change-me    # replace with the Debian installer user
```

Do not add a plaintext K3s token. The first server generates one.

## Record and verify the SSH host key

Make the first connection manually:

```bash
homelabctl node connect titan
```

Compare the displayed fingerprint with the Ed25519 host-key fingerprint shown
from Titan's physical console. This first console comparison is necessarily
outside `homelabctl` because no remote identity has been trusted yet.

Accept it only when the values match. Ansible keeps host-key checking enabled,
so an unexpected host identity change will stop automation.

Install the operator's public key before asking Ansible to connect:

```bash
homelabctl node authorize-key titan \
  --public-key "$HOME/.ssh/homelab_titan_ed25519.pub"
```

Open a new `homelabctl node connect titan` session and prove it uses the key.
The complete staged procedure is in the [Titan setup
runbook](/getting-started/titan-setup#10-trust-titan-and-install-the-operator-key).

Inside that key-authenticated session, verify the privilege boundary before
installing Ansible:

```bash
id
sudo -v
sudo id
```

Exit and prove one more new key-authenticated session works.

## Install pinned Ansible dependencies

Only after SSH and sudo are verified, run from the repository root:

```bash
homelabctl setup ansible
homelabctl ci check --only ansible
homelabctl doctor
```

The Python requirements pin `ansible-core`, `ansible-lint` and `netaddr`. The
collection requirements pin the official `k3s.orchestration` collection to an
audited Git commit and pin all dependent collections. Upgrade these pins through
a reviewed change, never implicitly during a production run.

The CLI automatically selects `ansible/.venv` for Ansible-backed commands. Do
not activate it or reproduce its commands in normal operation.

## Test Ansible

```bash
homelabctl inventory check
```

If sudo requires a password, add `--ask-become-pass` to playbook commands:

```bash
homelabctl node prepare --ask-become-pass
```

Do not store the sudo password in the repository.
