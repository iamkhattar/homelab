# Homelab

Home-first infrastructure for a single-node K3s cluster running on `titan`, an
AMD mini PC with Debian. Ansible prepares the operating system and manages the
K3s lifecycle. Helmfile deploys cluster services. Terraform remains available
for future, tainted Hetzner worker nodes after private Tailscale networking is
introduced.

[`homelabctl`](docs/homelabctl/index.md) is the Go operator and CI entry point. It
wraps the repository's native tools without replacing their configuration.

## Bootstrap the operator CLI

Install a checksum-verified Linux or macOS binary from [GitHub
Releases](https://github.com/iamkhattar/homelab/releases). The detailed
[release guide](docs/homelabctl/releases-update.md) includes the Debian
install path and verification commands. Once installed:

```bash
homelabctl version
homelabctl update --check
```

Contributors can build from `homelabctl/` with Go 1.27 or newer.

All normal repository and homelab procedures use the CLI after that point:

```bash
homelabctl setup
homelabctl doctor
```

Generate CI-compatible test, SARIF and SPDX reports and run the pinned security
scanners with:

```bash
homelabctl ci check --reports
```

## Documentation

The operational guide lives in [`docs/`](docs/index.md) and covers Debian
installation, Ansible, K3s installation, upgrades, reboots, backup and recovery.
Start a new physical machine with the [complete Titan setup
runbook](docs/getting-started/titan-setup.md).

The documentation site is isolated under `docs/` and requires Node.js 24 or
newer. Use the operator interface from the repository root:

```bash
homelabctl docs setup
homelabctl docs dev
```

Build and preview the production site with:

```bash
homelabctl docs build
homelabctl docs preview
```

Build the internal Nginx image from the repository root with:

```bash
homelabctl build docs --tag dev
```

Build the non-root CLI image intended as the base for a future runner or
operator with:

```bash
homelabctl build homelabctl --tag dev
```
