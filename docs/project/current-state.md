# Current state

This page is the operational truth about the repository and Titan. It separates
code that exists, validation performed on a workstation, and changes confirmed
on the physical machine.

::: info Update this page after each milestone
Do not mark a component deployed because its manifests or automation exist.
Deployment means the result has been verified on Titan and its recovery path is
known.
:::

## Status legend

<StatusLegend />

## Pre-Titan handoff

The repository is at the pre-Titan boundary: the supported host, cluster,
platform and recovery workflows exist in Git and pass workstation validation.
The remaining foundation work requires facts or evidence from the physical
machine. Do not interpret **Ready in repo** as **Deployed**.

| Category | Remaining work |
| --- | --- |
| Required on Titan | Finish Debian, reserve the LAN address, establish verified SSH access, apply the Ansible baseline, install K3s, export recovery material and rehearse a restore. |
| Required before important data | Choose encrypted off-node storage and prove that K3s, Vault, database and application backups can be restored from it. |
| Required before relying on alerts | Choose an off-cluster Alertmanager receiver, deliver its credential through Vault and verify a test notification. |
| Required during platform bootstrap | Publish the private-address DNS records, define Vault recovery-share custody, complete the Pocket ID owner ceremony, initialize Vault through Butler, export recovery material and trust the private CA. |
| Later design decisions | DNS-01 automation, internal resolver failover, ZHA versus Zigbee2MQTT, and the future Tailscale/Hetzner design. |

Repository enhancements such as controlled automatic security updates,
SMART/NVMe telemetry and signed build provenance are useful follow-up work, but
they do not block the first recoverable Titan installation.

## Foundation snapshot

| Area | Status | Evidence and next action |
| --- | --- | --- |
| Debian on Titan | In progress | Finish the Debian installation, create the operator account and verify local login. |
| Hostname | Not deployed | Set `titan` only on the mini PC; the inventory does not force that name on other nodes. |
| Ansible host baseline | Ready in repo | Local role, private inventory pattern, optional Titan shell marker, pinned K3s collection, diagnostics/recovery playbooks, detailed contributor manual, syntax checks and lint exist. Run the preparation checkpoints against Titan next. |
| SSH hardening | Ready in repo | Key-only policy is opt-in so password access is not disabled before the key is proven. |
| K3s server | Not deployed | Installation and verification runbooks exist; run them only after the host baseline succeeds. |
| Cluster recovery | Not deployed | Embedded-etcd snapshots are configured, but an off-node export and restore test are still required. |
| `homelabctl` | Ready in repo | The Go CLI is the documented operator interface for setup, inventory, SSH access, Debian/K3s lifecycle, diagnostics, snapshots, recovery export, Butler bootstrap/control, docs, deployments, builds and checks. Reporting mode generates JUnit/JSON tests, gosec and Trivy SARIF, and an SPDX SBOM for retained CI artifacts and GitHub code scanning. Successful main builds publish checksum-protected Linux/macOS releases with built-in update support. |
| Documentation site | Ready in repo | Isolated VitePress project, intent-based handbook navigation, ordered section flows, unprivileged Nginx image and component engineering manuals are implemented. Internal cluster hosting waits on ingress and authentication. |

## Workload snapshot

| Capability | Status | Prerequisite |
| --- | --- | --- |
| Ingress and internal DNS | Ready in repo | Helmfile-managed Traefik, `home.6940469.xyz`, Vault private PKI and authenticated CA export are selected. Publishing the private-address Namecheap records and proving household DNS/trust remain deployment checkpoints; public DNS-01 automation is later work. |
| Persistent storage | Ready for testing | K3s local-path with bounded PVCs is the accepted single-node starting point. Select the encrypted off-node backup target and rehearse restores before storing important data. |
| Prometheus, Grafana, Loki, Tempo and Alloy | Ready in repo | Pinned bounded charts, seven-day retention, kube-state-metrics, node-exporter, OTLP receivers, log collection, Butler metrics/logs/traces, Grafana correlation, reusable all-workload dashboards and workload/Job/PVC/OOM alerts render. Choose and test the final off-cluster Alertmanager receiver before relying on alerts. |
| Pocket ID | Ready in repo | Pinned v2 deployment, Vault-provided encryption key, native OTLP, Butler-managed groups, users and OIDC clients, secret rotation into Vault, and Butler PKCE login exist; first owner/API key remain an interactive Titan checkpoint. |
| Vault and Butler | Ready in repo | Top-level Butler has separate normal and private recovery runtimes. Recovery performs confirmed, resumable initialization and now remains identity-pending until real Pocket ID logins to Butler and Vault pass. Normal reconciliation uses projected Kubernetes auth. VSO remains the application secret-delivery path. Export the recovery Secret to an age-encrypted off-cluster bundle and restore-test on Titan. |
| Shared PostgreSQL, Redis and Garage | Ready in repo | Helmfile order, least-privilege consumer projections, per-application PostgreSQL identities, persistence, NetworkPolicies and Garage v2 API reconciliation render successfully; deploy and restore-test on Titan |
| Actions Runner Controller | Ready in repo | Controller and one scale-to-zero runner set render successfully; create/import a least-privilege GitHub App and prove a read-only job before deployment authority |
| Homepage, KitchenOwl, ntfy, Vaultwarden and Paperless-ngx | Ready in repo | Each app has its own namespace; selected charts, pinned images, resources, persistence, scoped Vault credentials and initial NetworkPolicies exist; keep internal until per-app TLS/auth/backup checks pass |
| Home Assistant | Not deployed | Storage, backup and a tested USB device strategy |
| Zigbee and MQTT | Not deployed | Stable `/dev/serial/by-id` mapping and a decision on Zigbee2MQTT/Mosquitto |
| Internal docs hosting | Not deployed | Container registry, ingress and access policy |
| Tailscale | Not deployed | Tailnet identity, ACL and key-expiry decisions |
| Hetzner agents | Legacy review | Tailscale transport and replacement of token-bearing cloud-init |

## Newly implemented repository contracts

- `homelabctl control login` uses Pocket ID Authorization Code with PKCE and a
  loopback callback; logout removes the private cached session.
- Butler issues only configured short-lived Kubernetes roles from Vault and
  never records the returned bearer token.
- Butler operations and audit-safe events survive pod restarts on a dedicated
  PVC.
- Bootstrap pauses for the Pocket ID management credential, reconciles groups
  and OIDC clients, then requires real Butler and Vault Pocket ID login proofs
  before becoming operational. The temporary Vault token remains on the
  workstation and is revoked after policy verification.
- `homelabctl deploy platform` applies the supported Helmfile stage order and
  rejects data, observability, CI/CD and application deployment while Butler's
  bootstrap phase is not `operational`.
- `homelabctl trust export` obtains the private PKI chain through authenticated
  Kubernetes, validates every CA certificate and writes a non-overwriting public
  bundle with displayed SHA-256 fingerprints.
- Metrics Server, broader platform alerts, and Kubernetes/Vault/cert-manager
  dashboards are represented in Helmfile.
- Real Vault, Pocket ID and Kubernetes integration tests exist behind the
  explicit `integration` build tag and `BUTLER_INTEGRATION=1` guard.

## Immediate next checkpoint

The next safe sequence is deliberately small:

1. Complete [the Debian installation](/getting-started/debian-install).
2. Reserve Titan's LAN address in the router and verify time and DNS.
3. Run `homelabctl inventory init`, edit the private values, and do not commit
   the result.
4. Prove ordinary SSH and SSH-key access with `homelabctl node connect titan`.
5. Run `homelabctl node prepare --check`, then `homelabctl node prepare`.
6. Run `homelabctl node reboot` if required, then
   `homelabctl inventory check`.
7. Run `homelabctl cluster bootstrap` and complete every verification in the
   install runbook.
8. Run `homelabctl cluster recovery export`, then encrypt and move the exported
   K3s token and first etcd snapshot off the workstation.

Do not start the application platform until step 8 is complete. The detailed
acceptance criteria for later phases live in the [roadmap](/project/roadmap).
After the foundation passes, continue with the
[platform bootstrap runbook](/operations/platform-bootstrap): deploy only
through identity first, finish Pocket ID and Vault verification, export and
trust the CA, and then add data, observability, CI/CD and applications one stage
at a time.

## Sources of truth

| Concern | Source |
| --- | --- |
| Actual machine facts | Commands run against Titan and recorded verification results |
| Host membership and node-specific values | Private Ansible inventory |
| Debian and K3s desired state | Ansible roles, variables and playbooks |
| Cluster workloads | Helmfile and chart values |
| External cloud resources | Terraform after its legacy path is redesigned |
| Operator workflow | These runbooks and `homelabctl` |
| Bootstrap secrets | Off-cluster password manager, never Git or Terraform state |
