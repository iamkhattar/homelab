# Roadmap

The homelab is built in dependency order. Each phase must be operable and
recoverable before the next one becomes infrastructure that the household
depends on.

## Design goals

- One private home node should be useful without any cloud dependency.
- A clean Debian install should be enough to rebuild Titan from the repository
  and off-cluster recovery material.
- Normal maintenance should be boring: preview, apply, verify and record.
- Loss of the single node will cause downtime, but should not cause permanent
  loss of configuration, secrets or irreplaceable application data.
- Remote nodes may add capacity later, but must not silently weaken the home
  network or run sensitive workloads by default.

## Phase 0 — Repository and operator foundation

**State:** ready in the repository; physical-node execution remains outstanding.

Deliverables:

- isolated Ansible environment with pinned Python and collection dependencies;
- reusable Debian baseline with safe, staged SSH hardening;
- inventory model for one local server and future labelled or tainted agents;
- K3s install, upgrade, reboot and recovery runbooks;
- `homelabctl` as the local and CI orchestration entry point and sole normal
  runbook interface after its one-time self-build;
- internal VitePress handbook with a container build.

Done when repository checks pass and the operator can explain where every
machine-specific value and bootstrap secret belongs.

## Definition of done for every phase

A roadmap phase or increment is not complete until all of these are true:

- the typed `homelabctl` workflow exists and has tests or safe dry-run coverage;
- repository-native tools remain inspectable but are not required knowledge for
  the normal procedure;
- the relevant install, operation, recovery and troubleshooting pages use the
  real CLI commands;
- [current state](/project/current-state) distinguishes repository-ready work
  from changes verified on Titan;
- this roadmap and the [decision log](/project/decisions) reflect any changed
  scope or architectural choice;
- acceptance checks have been run at the appropriate level and their result is
  recorded accurately.

## Phase 1 — Recoverable Titan foundation

**State:** repository automation is ready; execution and recovery evidence on
Titan remain outstanding.

**Goal:** one dependable Debian host and one healthy K3s server.

Work:

1. install Debian, firmware and an administrative user;
2. reserve the LAN address and set the mini PC hostname to `titan`;
3. prove SSH keys, then apply the hardened SSH policy;
4. install K3s with secrets encryption and embedded etcd snapshots;
5. record node, system service and Kubernetes health checks;
6. export bootstrap recovery material off the mini PC;
7. perform at least one snapshot restore rehearsal before important workloads.

Acceptance criteria:

- `homelabctl cluster status` is healthy after reboot;
- no router port forward exposes SSH or the Kubernetes API;
- a second copy of the K3s token, kubeconfig and snapshot exists off Titan;
- the documented rebuild path does not depend on any service inside the cluster.

## Phase 2 — Cluster platform

**State:** private-address wildcard DNS is published and verified; public
DNS-01 automation is ready in Git while off-node backup selection and Titan
restore testing remain outstanding.

**Goal:** give applications stable networking, certificates, storage and backup.

Accepted implementation:

- dedicate `6940469.xyz` to flat application names, publish its wildcard and
  apex to Titan's private address, and keep all router ingress forwarding off;
- deploy Traefik through Helmfile because bundled K3s Traefik is disabled;
- issue one publicly trusted apex-and-wildcard certificate from Let's Encrypt
  through Butler, Vault, VSO, acme-dns and cert-manager;
- use bounded K3s local-path PVCs instead of claiming single-node Longhorn is
  highly available;
- keep one namespace per selected application.

Remaining work:

- complete the one-time Namecheap `_acme-challenge` CNAME ceremony on Titan;
- define off-node application backup targets, schedules and restore tests;
- measure the committed resource and retention budgets on Titan and adjust only
  from observed pressure.

Longhorn should not be assumed to provide high availability on one machine. Any
storage choice must be evaluated by how it restores after loss of Titan's disk,
not by how many replicas it claims locally.

Acceptance criteria:

- a disposable test application is reachable through an internal HTTPS name;
- deleting and restoring its persistent data has been rehearsed;
- backup data leaves Titan and failures are visible to the operator.

## Phase 3 — Observability

**State:** ready in the repository; Titan deployment and signal verification remain outstanding.

**Goal:** detect host and workload problems before adding household-critical apps.

Implemented stack:

- Prometheus for Kubernetes, node and application metrics, with
  kube-state-metrics and node-exporter;
- Grafana for dashboards and alert investigation;
- Loki for bounded log retention;
- monolithic Tempo for traces and Grafana Alloy for Kubernetes logs and OTLP;
- Alertmanager with initial Butler, Vault and storage rules; the final
  off-cluster receiver is an explicit deployment-time decision still to make.

Before deployment, set disk, memory and retention budgets appropriate for a
single mini PC. Avoid collecting high-cardinality data without a use case.

Acceptance criteria:

- dashboards show node CPU, memory, disk, temperature where available, pod
  restarts and persistent-volume usage;
- alerts cover disk pressure, failed backups, K3s/node loss and expiring
  certificates;
- observability storage cannot consume the disk without a bound.

## Phase 4 — Identity and secrets

**State:** repository implementation complete, including the real Butler and
Vault login acceptance gate; first-owner bootstrap, recovery export and Titan
verification remain outstanding.

**Goal:** centralise application sign-in and runtime secrets without creating a
bootstrap loop.

Planned components:

- Pocket ID for passkey-based operator identity and application SSO;
- Vault for application and automation secrets that benefit from policy,
  rotation or short-lived credentials;
- an ingress authentication pattern appropriate to each application.

Vault will not create Debian users, distribute the first SSH key, or be the only
copy of credentials needed to recover Vault or K3s. Ansible owns host users and
SSH policy; an off-cluster password manager owns bootstrap recovery material.

Acceptance criteria:

- both services have tested data backups and documented recovery credentials;
- losing the cluster does not prevent the operator from rebuilding it;
- applications receive secrets without plaintext values being committed to Git;
- access policy is deny-by-default and understandable from the repository.

The repository workflow applies through identity, pauses for the interactive
Pocket ID owner ceremony, then runs `homelabctl control verify-identity`. Data
and later stages remain blocked until the persisted bootstrap phase is
`operational`.

## Phase 5 — Shared data and cluster delivery

**Goal:** provide the selected applications with bounded shared state and a
scale-to-zero deployment path.

`homelabctl deploy platform --through STAGE --confirm` now encodes the stage
order. It re-applies earlier idempotent stages and checks Butler's operational
acceptance gate before entering this phase.

Work:

- deploy one PostgreSQL instance with a separate database, role and
  Vault-generated password per application;
- deploy authenticated standalone Redis for real cache/queue consumers;
- deploy Garage as internal S3-compatible storage without calling its local
  copy a backup;
- deploy Actions Runner Controller and a one-runner `titan` scale set whose
  idle capacity is zero;
- deliver GitHub App credentials through VSO and deployment credentials as
  short-lived, bounded leases;
- prove initial bootstrap from the control machine and routine deployment from
  an ephemeral runner.

Acceptance criteria:

- each client can reach only its declared data services;
- database credentials are distinct and never committed to Git;
- state can be restored from a copy outside Titan;
- the runner returns to zero pods after a job and cannot read unrelated Secrets
  or use cluster-admin.

## Phase 6 — Selected applications

**Goal:** deploy the deliberately small initial application set.

Deploy Homepage, KitchenOwl, ntfy, Vaultwarden and Paperless-ngx one at a time.
An application remains internal until its Pocket ID pattern, HTTPS name,
NetworkPolicy, telemetry, backup and restore have been verified. Vaultwarden
uses native Pocket ID OIDC; applications without suitable native OIDC use the
reviewed shared ingress authentication pattern.

Acceptance criteria:

- every app has a separate Vault path and only its required database/cache
  identity;
- every stateful app has a successful restore rehearsal;
- no selected service is reachable from the public internet;
- Homepage shows only reviewed internal links and has read-only Kubernetes API
  access.

## Phase 7 — Home automation

**Goal:** run Home Assistant and Zigbee reliably enough for household use.

**State:** the staged architecture and hardened charts are ready in the
repository; the exact Sonoff `/dev/serial/by-id` path, Titan deployment and
restore evidence remain outstanding.

Work:

- deploy Home Assistant with persistent configuration and tested backups;
- identify the Sonoff dongle through `/dev/serial/by-id`, not a changing `ttyUSB`
  number;
- use Zigbee2MQTT as the sole owner of the Sonoff zStack coordinator;
- deploy authenticated Mosquitto with Butler-generated credentials;
- constrain USB access to the intended workload and Titan node;
- document maintenance windows, rollback and automation fallbacks.

The three opt-in increments are Home Assistant, Mosquitto, then Zigbee2MQTT.
The last increment cannot render with an empty or unstable USB path. See the
[home automation runbook](/operations/smart-home).

Acceptance criteria:

- the dongle remains attached after reboot and workload rescheduling;
- Home Assistant state can be restored on a clean deployment;
- essential lights or devices retain a manual path during cluster downtime.

## Phase 8 — Self-hosted operator services

**Goal:** host this handbook and selected operator tooling internally.

Work includes publishing versioned images, deploying the docs Nginx image,
putting it behind internal HTTPS, enforcing the chosen access policy and adding
health and rollout checks to `homelabctl`.

This phase follows identity so operational details are not accidentally exposed.

## Phase 9 — Tailscale and Hetzner capacity

**Goal:** add optional remote workers without making them part of the trusted
default scheduling pool.

Work includes a tailnet ACL model, runtime K3s join-token delivery, removal of
legacy cloud-init bootstrapping, cross-network CNI testing, node reachability
monitoring, location labels and `NoSchedule` taints. See the detailed
[Hetzner and Tailscale design](/future/hetzner-tailscale).

Acceptance criteria:

- the Kubernetes API is not publicly exposed;
- a newly provisioned remote node can join without a token entering Terraform
  state, cloud-init logs or Git;
- no workload schedules remotely unless it explicitly tolerates that location;
- loss of Tailscale or Hetzner does not disrupt Titan-only workloads.

## Cross-cutting future work

- a UPS and clean-shutdown behaviour for Titan and network equipment;
- automatic Debian security updates with controlled reboot reporting;
- SMART/NVMe health and temperature monitoring;
- signed provenance or attestations for the checksum-protected `homelabctl` release artifacts;
- a concise change log recording upgrades, restore tests and hardware changes.

These are not prerequisites for the first installation. The immediate gate is
a healthy, reproducible K3s node plus a verified encrypted recovery copy held
away from both Titan and the operator workstation.
