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

Repository preparation is complete and physical deployment has reached a
installed single-node K3s control plane. The supported host, cluster, platform
and recovery workflows exist in Git and pass workstation validation. Debian, wired
access, the reserved address, Titan's SSH host identity, the operator key, the
managed baseline and the Kubernetes API have now been observed on the physical
machine. Do not interpret **Ready in repo** as **Deployed**.

| Category | Remaining work |
| --- | --- |
| Required on Titan | Reconcile K3s with its newly pinned Ethernet IPv4 configuration, prove the API is Ready, run the full cluster diagnostic, create the first off-node K3s recovery export, verify another managed reboot, and rehearse a restore. |
| Required before important data | Choose encrypted off-node storage and prove that K3s, Vault, database and application backups can be restored from it. |
| Required before relying on alerts | Choose an off-cluster Alertmanager receiver, deliver its credential through Vault and verify a test notification. |
| Required during platform bootstrap | Define Vault recovery-share custody, complete the Pocket ID owner ceremony, initialize Vault through Butler, export recovery material and trust the private CA. |
| Later design decisions | DNS-01 automation, internal resolver failover, ZHA versus Zigbee2MQTT, and the future Tailscale/Hetzner design. |

Repository enhancements such as controlled automatic security updates,
SMART/NVMe telemetry and signed build provenance are useful follow-up work, but
they do not block the first recoverable Titan installation.

## Foundation snapshot

| Area | Status | Evidence and next action |
| --- | --- | --- |
| Debian on Titan | In progress | Debian 13 is installed, local and key-authenticated remote login work, and the shell accepts the generated UTF-8 locale. Read-only diagnostics confirmed the signed `7.1.8+deb13-amd64` backports kernel, synchronized NTP, valid SSH configuration, no failed units and ample disk space. Finish the managed baseline and post-reboot checks. |
| Hostname | In progress | The physical console, remote SSH banner and Ansible diagnostics identify this machine as `titan`, and Ansible now enforces the host-scoped value. Verify it across the first managed reboot. The inventory does not force that name on other nodes. |
| Wired networking | In progress | `eno1` is driven by in-tree `r8169`, receives the router-reserved address and carries successful SSH traffic. Static DHCP is configured on the Hyperoptic EX3301-T0 and Wi-Fi was disabled. Verify both facts across the first managed reboot. |
| Ansible host baseline | In progress | The reviewed baseline applied without failures, installed the administration packages, retained US locale, selected UTC, disabled swap and sleep, configured bounded logs and automatic security updates, and confirmed Chrony and trimming. Expanded diagnostics found no failed services. Complete the post-K3s idempotence and managed-reboot checks. |
| SSH hardening | Deployed | Titan's host fingerprint was verified, the dedicated operator key remains usable, and the private inventory enables the managed key-only SSH policy. Preserve physical-console access and recheck SSH after the first managed reboot. |
| K3s server | Needs reconciliation | Titan was initially verified Ready as the sole `control-plane,etcd` node on K3s `v1.36.4+k3s1`. The latest reboot then exposed nondeterministic address selection: embedded etcd retained `192.168.1.163` while K3s selected Titan's global IPv6 address. Private inventory and the public example now pin `node-ip`, `advertise-address` and `flannel-iface`; rerun cluster bootstrap and verify readiness before continuing. |
| Cluster recovery | In progress | Embedded-etcd snapshot configuration is installed, but the first snapshot/token export has not yet been verified in encrypted off-node storage and no restore rehearsal has been completed. |
| `homelabctl` | Ready in repo | The Go CLI is the documented operator interface for setup, inventory, SSH access, Debian/K3s lifecycle, diagnostics, snapshots, recovery export, Butler bootstrap/control, docs, deployments, builds and checks. Reporting mode generates JUnit/JSON tests, gosec and Trivy SARIF, and an SPDX SBOM for retained CI artifacts and GitHub code scanning. Successful main builds publish checksum-protected Linux/macOS releases with built-in update support. |
| Documentation site | Ready in repo | Isolated VitePress project, intent-based handbook navigation, ordered section flows, unprivileged Nginx image and component engineering manuals are implemented. Internal cluster hosting waits on ingress and authentication. |

## Workload snapshot

| Capability | Status | Prerequisite |
| --- | --- | --- |
| Ingress and private-address DNS | Partially deployed | Namecheap now publishes `*.6940469.xyz` and the apex to Titan's reserved `192.168.1.163`, with no AAAA record. `home.6940469.xyz` is the dashboard and application names are flat peers. LAN resolution and the matching chart, Butler and homelabctl defaults are complete; Traefik, client HTTPS trust, remote Tailscale routing and public DNS-01 automation remain deployment checkpoints. |
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

The host baseline and SSH hardening are complete. Continue with:

1. Run `homelabctl cluster bootstrap --limit titan --ask-become-pass` to apply
   the pinned Ethernet IPv4 configuration, then prove `cluster status` is healthy.
2. Run `homelabctl cluster diagnose --ask-become-pass` and review the K3s
   service, journal, events and embedded-etcd snapshot configuration.
3. Run `homelabctl cluster snapshot list --ask-become-pass`.
4. Run `homelabctl cluster recovery export` into private staging space, encrypt
   it, move it off Titan and the workstation, and verify the stored copy.
5. Create a pre-reboot snapshot, perform a managed cluster reboot, then verify
   SSH, the router-reserved address, node readiness and all core pods.
6. Record the first-build evidence and resolve recovery custody and the
   off-cluster alert receiver before platform bootstrap.

Do not start the application platform until the recovery export and reboot
checks are complete. The detailed
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
