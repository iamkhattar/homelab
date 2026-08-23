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

## Foundation snapshot

| Area | Status | Evidence and next action |
| --- | --- | --- |
| Debian on Titan | In progress | Finish the Debian installation, create the operator account and verify local login. |
| Hostname | Not deployed | Set `titan` only on the mini PC; the inventory does not force that name on other nodes. |
| Ansible host baseline | Ready in repo | Local role, private inventory pattern, optional Titan shell marker, pinned K3s collection, diagnostics/recovery playbooks, detailed contributor manual, syntax checks and lint exist. Run the preparation checkpoints against Titan next. |
| SSH hardening | Ready in repo | Key-only policy is opt-in so password access is not disabled before the key is proven. |
| K3s server | Not deployed | Installation and verification runbooks exist; run them only after the host baseline succeeds. |
| Cluster recovery | Not deployed | Embedded-etcd snapshots are configured, but an off-node export and restore test are still required. |
| `homelabctl` | Ready in repo | The Go CLI is the documented operator interface for setup, inventory, SSH access, Debian/K3s lifecycle, diagnostics, snapshots, recovery export, docs, deployments, builds and checks. Reporting mode generates JUnit/JSON tests, gosec and Trivy SARIF, and an SPDX SBOM for retained CI artifacts and GitHub code scanning. Successful main builds publish checksum-protected Linux/macOS releases with built-in update support. Control-plane API commands remain future work. |
| Documentation site | Ready in repo | Isolated VitePress project, intent-based handbook navigation, ordered section flows, unprivileged Nginx image and component engineering manuals are implemented. Internal cluster hosting waits on ingress and authentication. |

## Workload snapshot

| Capability | Status | Prerequisite |
| --- | --- | --- |
| Ingress and internal DNS | Not deployed | Stable K3s, a LAN naming strategy and certificate approach |
| Persistent storage | Blocked | Choose and test a single-node storage and backup model |
| Prometheus, Grafana and Loki | Not deployed | Storage, retention and resource budgets |
| Pocket ID | Not deployed | HTTPS ingress, internal DNS, persistent storage and backup |
| Vault | Not deployed | Stable storage, unseal/recovery design and off-cluster bootstrap credentials |
| Home Assistant | Not deployed | Storage, backup and a tested USB device strategy |
| Zigbee and MQTT | Not deployed | Stable `/dev/serial/by-id` mapping and a decision on Zigbee2MQTT/Mosquitto |
| Internal docs hosting | Not deployed | Container registry, ingress and access policy |
| Tailscale | Not deployed | Tailnet identity, ACL and key-expiry decisions |
| Hetzner agents | Legacy review | Tailscale transport and replacement of token-bearing cloud-init |

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
