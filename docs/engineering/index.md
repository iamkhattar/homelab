# Engineering guide

This section explains how the repository is built and maintained. Start here
before changing automation, CLI behavior, CI or documentation structure.

## Repository components

| Component | Responsibility | Start here |
| --- | --- | --- |
| `homelabctl/` | Typed operator workflows, validation and CI orchestration | [homelabctl introduction](/homelabctl/) |
| `ansible/` | Debian baseline and K3s host lifecycle | [Ansible introduction](/ansible/) |
| `cluster/` | Workload desired state composed with Helmfile | [Cluster platform](/engineering/cluster-platform) |
| `butler/` | Vault, identity and provider-API control plane | [Butler control plane](/engineering/butler-control-plane) |
| `infra/` | Optional future Hetzner infrastructure planning | [Hetzner and Tailscale](/future/hetzner-tailscale) |
| `docs/` | This isolated VitePress handbook and Nginx image | [Documentation system](/documentation/hosting) |
| `.github/workflows/` | Checks and image publication through `homelabctl` | [CI workflow](/homelabctl/deploy-build-ci) |

## Change workflow

1. Read the component introduction and its ownership boundaries.
2. Confirm whether the change affects repository intent, Titan deployment, or
   both.
3. Add or update validation at the lowest useful layer.
4. Preview external commands with `homelabctl --dry-run` where appropriate.
5. Run the focused check while iterating, then the complete CI check.
6. Update the relevant explanation, runbook and reference pages.
7. Update current state only after the change is verified on Titan.

```bash
homelabctl ci check --only go-format,go-test
homelabctl ci check --only workflows
homelabctl ci check
```

Real platform integration tests are opt-in. The Vault case creates and removes
a unique probe secret; Pocket ID and Kubernetes checks are read-only:

```bash
cd butler
BUTLER_INTEGRATION=1 VAULT_ADDR="$VAULT_ADDR" VAULT_TOKEN="$VAULT_TOKEN" \
POCKET_ID_URL="$POCKET_ID_URL" POCKET_ID_API_KEY="$POCKET_ID_API_KEY" \
KUBECONFIG="$KUBECONFIG" go test -tags=integration ./integration -count=1
```

Run these from the trusted LAN or a private runner after bootstrap and before
accepting an upgrade. Normal CI intentionally excludes them.

## Design rule

`homelabctl` owns workflow and validation, while Ansible, kubectl, Helmfile,
Terraform, Docker and npm remain the canonical execution engines. Prefer a Go
library for local repository plumbing or structured parsing. Do not recreate an
infrastructure engine merely to avoid invoking its supported CLI.

Continue with the component you intend to change: [homelabctl](/homelabctl/),
[Ansible](/ansible/), [Butler](/engineering/butler-control-plane) or the
[documentation system](/documentation/hosting).
