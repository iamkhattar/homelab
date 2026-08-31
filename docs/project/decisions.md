# Technical decisions

This is a lightweight decision log. It records choices that shape multiple
runbooks so they do not become accidental or get repeatedly reopened.

| Decision | State | Rationale |
| --- | --- | --- |
| Debian is Titan's host OS | Accepted | Small, stable base with predictable security maintenance and strong Ansible support. |
| `titan` is only the mini PC's hostname | Accepted | The name belongs in Titan's inventory entry; roles stay reusable for local and cloud nodes. |
| Begin with one K3s server | Accepted | Matches the available hardware and operational appetite. Recovery is prioritised over pretending one machine is highly available. |
| Use embedded etcd with snapshots | Accepted | Gives a supported K3s recovery mechanism and a later path to more servers, provided snapshots leave Titan. |
| Use the official pinned K3s Ansible collection | Accepted | Upstream owns K3s lifecycle mechanics; this repo owns local host policy and inventory. |
| Stage SSH hardening | Accepted | Key access is proven before password authentication is disabled, reducing lockout risk. |
| Ansible owns host users and SSH policy | Accepted | Vault is not available during first boot and should not be required to regain host access. |
| Keep bootstrap secrets off-cluster | Accepted | K3s and Vault must remain recoverable when the whole cluster is unavailable. |
| Do not forward SSH or Kubernetes API ports | Accepted | Home management stays private; future remote access will use an authenticated overlay network. |
| Disable bundled Traefik | Accepted | Ingress will be a separately versioned and recoverable cluster component. |
| Keep Helmfile's push model initially | Accepted | Avoids adding a GitOps controller before the cluster's own platform and recovery are stable. |
| Isolate the docs Node project under `docs/` | Accepted | Node dependencies and container context stay independent of Go services and repository root tooling. |
| Use `homelabctl` for normal runbooks | Accepted | Operators get one typed, auditable interface; native commands remain visible implementation details and break-glass tools. |
| Remote nodes are labelled and tainted | Accepted | Hetzner capacity is opt-in for workloads and cannot silently receive home-only services. |
| Start with K3s local-path storage | Accepted | One node gains nothing from replicated Longhorn scheduling. Stateful services use bounded local PVCs while the off-node backup target remains a separate required decision. |
| Dedicate `6940469.xyz` to flat homelab names with private-address public DNS | Accepted | `home.6940469.xyz` is the dashboard and peers such as `auth.6940469.xyz` share one wildcard; LAN and future Tailscale clients use the same names without public ingress. |
| Begin with Vault private PKI | Accepted transitional design | The authenticated Kubernetes CA export establishes trust now. Publicly trusted DNS-01 certificates replace it after DNS automation is selected. |
| Use manual Vault unseal and an encrypted off-node recovery export | Accepted | The Kubernetes recovery Secret supports single-node repair but is not the only copy; `homelabctl` exports it directly to age ciphertext. |
| Deliver workload secrets through Vault Secrets Operator | Accepted | Applications consume ordinary scoped Kubernetes Secrets while Vault remains the source of truth; workloads do not receive broad Vault credentials. |
| Use projected Kubernetes auth for normal Butler operation | Accepted | Butler receives short-lived, audience-bound identity from its ServiceAccount. Its private recovery deployment is separately authorised and is not an everyday root-token runtime. |
| Use Pocket ID for supported human application login | Accepted | Butler, Vault and applications with suitable OIDC support share one passkey-based identity layer. Lower-level console, SSH and encrypted recovery material remain independent break-glass paths. |
| Keep one namespace per application | Accepted | Namespace-scoped service accounts, Secrets, policies and quotas make application boundaries visible and testable. Shared platform services keep dedicated functional namespaces. |
| Use Prometheus, Loki, Tempo, Grafana and Alloy | Accepted | The bounded single-node stack provides correlated metrics, logs and traces without introducing separate agents for each signal. Retention must be tuned from Titan measurements. |
| Run Tailscale through the Kubernetes Operator only | Accepted | Remote application access may disappear with Kubernetes; host recovery stays on the trusted LAN through console or SSH and no host Tailscale daemon expands the recovery dependency chain. |
| Scale GitHub Actions runners to zero | Accepted | Ephemeral runners provide the later deployment mechanism without a permanently idle workload. Their GitHub App and Kubernetes permissions remain least privilege. |
| Native Zigbee versus Zigbee2MQTT | Open | Decide after USB passthrough and Home Assistant operational requirements are tested. |

## How to change a decision

Change the table when evidence or requirements change, and update every affected
inventory variable, runbook and roadmap phase in the same pull request. For a
high-impact reversal, add a short section below describing the old choice, new
choice, migration and rollback path.
