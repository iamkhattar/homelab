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
| Pin supported chart versions and container digests | Accepted | “Latest” is audited, not followed at runtime. Helm dependencies are locked; mutable community image tags such as Bitnami's are resolved to multi-architecture digests and chart/schema upgrades are rendered before merge. |
| Dedicate `6940469.xyz` to flat homelab names with private-address public DNS | Accepted | `home.6940469.xyz` is the dashboard and peers such as `auth.6940469.xyz` share one wildcard; LAN and future Tailscale clients use the same names without public ingress. |
| Use Let's Encrypt DNS-01 through acme-dns | Accepted | One permanent Namecheap CNAME delegates only `_acme-challenge`; Butler registers and stores the bounded credential in Vault, VSO projects it, cert-manager renews one apex-and-wildcard certificate, and Titan remains unreachable from the public internet. |
| Keep public certificate lifecycle in cert-manager | Accepted | Vault Community can act as an internal ACME CA, but Vault's external-CA integration is an Enterprise feature. A community Vault plugin or a second ACME client would add another renewal controller and plugin lifecycle. cert-manager already owns Kubernetes Certificates, renewal and Traefik Secret delivery; Vault stores only the bounded acme-dns credential. |
| Unseal Vault automatically from the recovery Secret, and keep an encrypted off-node copy | Accepted | A restart seals Vault again, and a sealed Vault takes normal Butler down with it. Only the recovery workload may read `butler-vault-init`, so the recovery workload unseals Vault on startup and whenever it observes a sealed Vault. It never initializes Vault: initialization stays a deliberate operator action. The recovery Secret is still not the only copy — `homelabctl` exports it to age ciphertext, and the off-cluster copy remains the one that survives losing the cluster. |
| Deliver workload secrets through Vault Secrets Operator | Accepted | Applications consume ordinary scoped Kubernetes Secrets while Vault remains the source of truth; workloads do not receive broad Vault credentials. |
| Declare provider integrations through Butler CRDs | Accepted | Each owning chart declares concrete `PocketIDClient`, `ManagedCredential`, or `GarageBucket` intent. Butler writes generated values directly to Vault and reports only non-secret status. A generic `ApplicationIntegration` adds no executable contract and is omitted. |
| Keep Butler stateless | Accepted | Its audit-safe journal is stored in bounded `ButlerOperation` resources through the Kubernetes API, so the controller needs no application PVC. |
| Use projected Kubernetes auth for normal Butler operation | Accepted | Butler receives short-lived, audience-bound identity from its ServiceAccount. Its private recovery deployment is separately authorised and is not an everyday root-token runtime. |
| Use Pocket ID for supported human application login | Accepted | Butler, Vault and applications with suitable OIDC support share one passkey-based identity layer. Lower-level console, SSH and encrypted recovery material remain independent break-glass paths. |
| Keep one namespace per application | Accepted | Namespace-scoped service accounts, Secrets, policies and quotas make application boundaries visible and testable. Shared platform services keep dedicated functional namespaces. |
| Use Prometheus, Loki, Tempo, Grafana and Alloy | Accepted | The bounded single-node stack provides correlated metrics, logs and traces without introducing separate agents for each signal. Retention must be tuned from Titan measurements. |
| Run Tailscale through the Kubernetes Operator only | Accepted | Remote application access may disappear with Kubernetes; host recovery stays on the trusted LAN through console or SSH and no host Tailscale daemon expands the recovery dependency chain. |
| Scale GitHub Actions runners to zero | Accepted | Ephemeral runners provide the later deployment mechanism without a permanently idle workload. Their GitHub App and Kubernetes permissions remain least privilege. |
| Use Zigbee2MQTT with the Sonoff zStack coordinator | Accepted | Mixed switches benefit from Zigbee2MQTT's device catalogue and diagnostics, and the coordinator remains independent of Home Assistant restarts. Mosquitto is therefore an intentional dependency; ZHA must never claim the same device. |
| Isolate the Zigbee USB exception | Accepted | Home Assistant and Mosquitto remain under baseline Pod Security in `smart-home`; only the Titan-pinned Zigbee2MQTT pod runs in the privileged `zigbee` namespace with an exact `/dev/serial/by-id` device. |
| Reconcile Pocket ID users through the API, not a custom resource | Accepted | `ManagedCredential`, `PocketIDClient`, `PocketIDGroup` and `GarageBucket` describe provider state the cluster should converge on, so they are reconciled. A user is a one-time operator action with no steady-state shape Butler should overwrite, so `homelabctl control users` calls the identity domain directly and `PocketIDGroup` supplies the group boundary that membership depends on. |
| Give every namespace a default-deny baseline | Accepted | A namespace without a `podSelector: {}` policy has no boundary: a new pod there may reach anything until someone writes its policy. Explicit allow policies then document each admitted flow, and a new workload fails closed instead of open. |
| Build the platform before recovery custody | Accepted | The current goal is a working, repeatable single-node cluster on Titan. Infrastructure and configuration are declared in this repository and can be recreated from it, so recovery must not gate platform deployment or delay it. The limit is data, not infrastructure: until an off-cluster copy exists, the cluster must not hold irreplaceable household data such as documents, credentials or recipes. Recovery custody and the off-cluster target remain an open later workstream. |

## Open decisions

### Prove K3s's bundled network-policy controller on Titan — Proposed

K3s ships an embedded network-policy controller (kube-router's netpol library,
policy-only, no other kube-router functionality). It is enabled by default and
is disabled only by starting every server with `--disable-network-policy`, which
this repository does not do. Policy enforcement is therefore expected to be
active on Titan today, and the embedded controller is per-node, so it remains
correct as the cluster grows.

The open item is evidence, not architecture: no one has confirmed on Titan that
the controller is programmed, nor that a policy denies what it should.

Required before this becomes Accepted, on Titan:

- `iptables-save | grep KUBE-ROUTER` on the node shows programmed controller
  rules rather than an empty result;
- a disposable pod proves one allowed path and one denied path across
  namespaces;
- the result is recorded in [current state](/project/current-state) rather than
  assumed.

Explicitly rejected, so the question is not reopened later:

- **Installing a policy-capable CNI (Cilium, Calico, or kube-router as a
  standalone DaemonSet) in place of the embedded controller.** K3s already runs
  exactly the recommended configuration — kube-router's netpol implementation in
  policy-only mode beside flannel. Replacing a working, default, multi-node-capable
  network path on a single-node cluster whose snapshot restore has never been
  rehearsed would add risk and remove nothing.
- **Enabling `--enable-network-policy` or treating a CNI swap as required.** The
  embedded controller is already on.

### Choose the off-cluster recovery target — Open

Recovery custody is deferred, but the target is worth deciding before it is
needed, because the choice changes how backups are produced and not only where
they are stored. An S3-compatible object store off Titan is the natural fit:
Garage already provides an S3 API inside the cluster, so an off-site bucket or a
second Garage instance could receive encrypted backups. The alternatives are a
separate provider's bucket, a second always-on machine, or encrypted export to
an operator-managed store. This decision is deliberately unmade until the
platform stages are working, and nothing about the platform depends on it.

## How to change a decision

Change the table when evidence or requirements change, and update every affected
inventory variable, runbook and roadmap phase in the same pull request. For a
high-impact reversal, add a short section below describing the old choice, new
choice, migration and rollback path.
