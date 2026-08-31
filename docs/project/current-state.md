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

## Titan foundation handoff

Repository preparation is complete and physical deployment has started. The
supported host, cluster, platform and recovery workflows exist in Git and pass
workstation validation. Debian, wired access, the reserved address, Titan's SSH
host identity, the operator key and locale-clean remote login have now been
observed on the physical machine. Do not interpret **Ready in repo** as
**Deployed**.

| Category | Remaining work |
| --- | --- |
| Required on Titan | Finish the host-health and post-reboot checks, apply and harden the Ansible baseline, install K3s, export recovery material and rehearse a restore. |
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
| Debian on Titan | In progress | Debian 13 is installed, local and key-authenticated remote login work, and the shell accepts the generated UTF-8 locale. Read-only diagnostics confirmed the signed `7.1.8+deb13-amd64` backports kernel, synchronized NTP, valid SSH configuration, no failed units and ample disk space. Finish the managed baseline and post-reboot checks. |
| Hostname | In progress | The physical console, remote SSH banner and Ansible diagnostics identify this machine as `titan`; Ansible enforcement has not run. The inventory does not force that name on other nodes. |
| Wired networking | In progress | `eno1` is driven by in-tree `r8169`, receives the router-reserved address and carries successful SSH traffic. Static DHCP is configured on the Hyperoptic EX3301-T0 and Wi-Fi was disabled. Verify both facts across the first managed reboot. |
| Ansible host baseline | In progress | The reviewed baseline applied with `ok=27`, `changed=13`, `unreachable=0` and `failed=0`. It preserved SSH access with hardening disabled, installed the administration packages, retained US locale, selected UTC, disabled swap and sleep, configured bounded logs and automatic security updates, and confirmed Chrony and trimming. No reboot was requested. Run expanded diagnostics and an idempotence pass before hardening. |
| SSH hardening | In progress | Titan's host fingerprint was verified and the dedicated operator key now opens a clean session. Password login remains enabled intentionally until the first baseline succeeds; apply hardening in a separate pass and prove two new sessions. |
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

Inventory connectivity and read-only diagnostics are complete. Continue with:

1. Open a fresh managed SSH session, run the expanded node diagnostics and
   verify EFI, swap, Chrony, trimming, locale, route, DNS and failed units.
2. Run a second baseline preparation and investigate any unexpected repeated
   changes; no reboot is currently requested.
3. Enable SSH hardening in private inventory, prepare again, and prove two new
   key-only sessions before closing the recovery session.
4. Run `homelabctl cluster bootstrap` and complete every verification in the
   install runbook.
5. Run `homelabctl cluster recovery export`, then encrypt and move the exported
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
