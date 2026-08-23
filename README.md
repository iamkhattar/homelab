# Homelab

Home-first infrastructure for a single-node K3s cluster running on `titan`, an
AMD mini PC with Debian. Ansible prepares the operating system and manages the
K3s lifecycle. Helmfile deploys cluster services. Terraform remains available
for future, tainted Hetzner worker nodes after private Tailscale networking is
introduced.

[`homelabctl`](docs/homelabctl/index.md) is the Go operator and CI entry point. It
wraps the repository's native tools without replacing their configuration.

## Bootstrap the operator CLI

Building `homelabctl` itself is the one repository bootstrap command that cannot
go through `homelabctl`. Install Go 1.27 or newer; until versioned binaries are
published, run once:

```bash
cd homelabctl
go build -o ../bin/homelabctl ./cmd/homelabctl
cd ..
export PATH="$PWD/bin:$PATH"
```

All normal repository and homelab procedures use the CLI after that point:

```bash
homelabctl setup
homelabctl doctor
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
