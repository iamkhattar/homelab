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

**Goal:** give applications stable networking, certificates, storage and backup.

Decisions and work:

- choose an internal DNS suffix and how LAN clients resolve it;
- deploy ingress separately because bundled Traefik is disabled;
- choose certificate issuance for private names;
- audit existing Helmfile charts before applying anything inherited from the old
  Hetzner cluster;
- choose a single-node persistent-storage approach based on restore simplicity;
- define off-node application backup targets, schedules and restore tests;
- establish resource requests, limits and namespace conventions.

Longhorn should not be assumed to provide high availability on one machine. Any
storage choice must be evaluated by how it restores after loss of Titan's disk,
not by how many replicas it claims locally.

Acceptance criteria:

- a disposable test application is reachable through an internal HTTPS name;
- deleting and restoring its persistent data has been rehearsed;
- backup data leaves Titan and failures are visible to the operator.

## Phase 3 — Observability

**Goal:** detect host and workload problems before adding household-critical apps.

Planned stack:

- Prometheus for Kubernetes, node and application metrics;
- Grafana for dashboards and alert investigation;
- Loki for bounded log retention;
- node exporter and Kubernetes state metrics;
- alert delivery to an off-cluster channel.

Before deployment, set disk, memory and retention budgets appropriate for a
single mini PC. Avoid collecting high-cardinality data without a use case.

Acceptance criteria:

- dashboards show node CPU, memory, disk, temperature where available, pod
  restarts and persistent-volume usage;
- alerts cover disk pressure, failed backups, K3s/node loss and expiring
  certificates;
- observability storage cannot consume the disk without a bound.

## Phase 4 — Identity and secrets

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

## Phase 5 — Home automation

**Goal:** run Home Assistant and Zigbee reliably enough for household use.

Work:

- deploy Home Assistant with persistent configuration and tested backups;
- identify the Sonoff dongle through `/dev/serial/by-id`, not a changing `ttyUSB`
  number;
- decide between Home Assistant's native Zigbee integration and Zigbee2MQTT;
- deploy Mosquitto only if the chosen design needs MQTT;
- constrain USB access to the intended workload and Titan node;
- document maintenance windows, rollback and automation fallbacks.

Acceptance criteria:

- the dongle remains attached after reboot and workload rescheduling;
- Home Assistant state can be restored on a clean deployment;
- essential lights or devices retain a manual path during cluster downtime.

## Phase 6 — Self-hosted operator services

**Goal:** host this handbook and selected operator tooling internally.

Work includes publishing versioned images, deploying the docs Nginx image,
putting it behind internal HTTPS, enforcing the chosen access policy and adding
health and rollout checks to `homelabctl`.

This phase follows identity so operational details are not accidentally exposed.

## Phase 7 — Tailscale and Hetzner capacity

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
- SBOM, vulnerability and secret scanning with actionable CI output;
- a concise change log recording upgrades, restore tests and hardware changes.
