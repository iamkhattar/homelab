# Cluster platform and applications

This is the implementation contract for the first application platform on
Titan. It describes what is ready in Git; it does not claim that any release is
running until [current state](/project/current-state) records verification on
the node.

## Selected stack

| Domain | Releases |
| --- | --- |
| Foundation | namespaces, base RBAC |
| Secrets and identity | Vault, Butler, VSO, Pocket ID with Butler-managed groups/OIDC clients |
| Networking | cert-manager with production Let's Encrypt DNS-01 through acme-dns, one wildcard certificate and Traefik; Tailscale operator remains later work |
| Shared data | PostgreSQL, Redis, Garage with API-managed layout/bucket/key |
| Observability | Prometheus, kube-state-metrics, node-exporter, Loki, Tempo, Alloy and Grafana |
| Delivery | Actions Runner Controller and `titan` runner scale set |
| Applications | Homepage, KitchenOwl, ntfy, Vaultwarden, Paperless-ngx |
| Opt-in home automation | Home Assistant, Vault-backed Mosquitto, Titan-pinned Zigbee2MQTT for the Sonoff zStack coordinator |
| Deferred and removed | CrowdSec, Gatekeeper, Fluent Bit |

The first-install chart audit pins PostgreSQL chart `18.8.13` (PostgreSQL
`18.6`), Redis chart `28.0.12` (Redis `8.10`), cert-manager `1.21.1`, Vault
chart `0.34.1` (Vault `2.0.4`), VSO `1.5.1`, metrics-server chart `3.14.0` and
Traefik chart `41.4.0` (Traefik `3.7.12`). Grafana, Loki, Tempo, Alloy,
Prometheus and ARC were already at the current audited releases. Chart locks
are committed. The opt-in home-automation images are pinned to Home Assistant
`2026.8.3`, Zigbee2MQTT `2.13.0` and the newest published official Mosquitto
container, `2.0.22`, plus immutable multi-architecture digests. Bitnami's free community database images expose a mutable
`latest` tag, so PostgreSQL, Redis and exporter manifests are additionally
pinned by digest; updating those charts requires deliberately refreshing and
testing the matching digests.

On a fresh cluster, `homelabctl deploy diff` and `deploy apply` pass Helmfile's
`--skip-diff-validation-on-install` option so custom resources can be rendered
before their owning CRDs have been installed. The relaxation applies only when
Helmfile detects a new release; diffs for installed releases retain Kubernetes
API validation. Helm lint, the complete Helmfile render and repository security
checks remain mandatory before deployment.

cert-manager has an additional apply-time boundary: its controller and CRDs
are installed by the `cert-manager` release, while `ClusterIssuer` and
`Certificate` objects belong to the dependent `public-certificates` release.
Helm cannot submit those custom resources in the transaction that first makes
their kinds discoverable. Keeping the policy objects in a second release makes
both a clean installation and later idempotent reconciliation valid.

Fluent Bit is not part of the target design because Grafana Alloy will own the
Kubernetes log and OTLP collection path. CrowdSec has no useful blocking path
without public ingress. Gatekeeper is unnecessary before there are concrete
constraints that Pod Security Admission and CI cannot enforce.

## Release order

Helmfile encodes the dependency graph. The important path is:

```text
namespaces + RBAC
      /       \
cert-manager Vault
      |
public certificates
      \       /
       Butler -- registers acme-dns into Vault
        |
       VSO -- projects cert-manager credential
      /   \
shared data  ARC credentials
      |           |
applications   scale-to-zero runners
```

Butler recovery being active before VSO is intentional. It initializes and
configures Vault, then reconciles `ManagedCredential` declarations into Vault
KV. VSO materializes
only the Kubernetes Secrets required by a workload. A Helm `needs` edge orders
release submission; it does not prove that Vault is unsealed or a VSO Secret is
ready. Deploy and verify one checkpoint at a time during first bootstrap.

## Shared data is shared infrastructure, not shared identity

PostgreSQL runs once, but KitchenOwl, Paperless-ngx and Vaultwarden each receive
their own database, login and generated password. Butler stores the source at
`secret/databases/postgresql`, then publishes exact consumer projections such
as `secret/applications/paperless-ngx/database`. Namespace-local VSO roles can
read the projection only, never the shared source. The PostgreSQL first-boot
Secret contains an `init.sql` script generated from those same values.

The init script runs only for an empty PostgreSQL volume. Butler must eventually
own idempotent database/user reconciliation for applications added after first
boot; do not expect changing `init.sql` to mutate an existing database.

Redis is authenticated and initially serves Paperless-ngx. Butler creates both
the password and the complete client URI at `secret/databases/redis`; VSO gives
Paperless only the URI. Redis is cache/queue infrastructure, not primary durable
storage.

Garage provides an internal S3-compatible API. Its RPC and admin tokens come
from Vault. Butler calls Garage's v2 admin API to assign the one live node,
apply the layout, reconcile each app-owned `GarageBucket` and access key, grant its bounded
permissions and persist the one-time key to Vault. Garage data stored on Titan
is not an off-node backup.

These contracts are Kubernetes APIs, not entries in Butler's ConfigMap. Each
chart owns its `ManagedCredential` and `PocketIDClient` resources beside the
`VaultStaticSecret` that consumes the resulting path. The separate
`butler-crds` foundation release makes the types available before any consumer
release is rendered or applied.

## Application credentials and authentication

| Application | Vault material | Data dependency | Pocket ID state |
| --- | --- | --- | --- |
| Homepage | generated auth secret and OIDC client | Kubernetes read-only API and service links | native Pocket ID OIDC is configured in Homepage 2.1.2 |
| KitchenOwl | JWT key plus its database credential | PostgreSQL | review native support; use the shared proxy if required |
| ntfy | user/token lifecycle still to be managed through its API | local cache | login is required; Pocket ID proxy integration remains a gate for ingress |
| Vaultwarden | admin token, database URL and OIDC client | PostgreSQL | native OIDC is configured for Pocket ID |
| Paperless-ngx | application key, initial admin and database/Redis clients | PostgreSQL and Redis | review native OIDC before ingress |

Services remain `ClusterIP`. A DNS name in values is an intended canonical URL,
not permission to expose the service. Add ingress only after TLS, Pocket ID and
the application's callback/logout behaviour pass an end-to-end test.

## Network boundaries

- Pocket ID disables its version check and analytics heartbeat and has no
  arbitrary internet egress. Its pod NetworkPolicy permits only cluster DNS
  and OTLP/HTTP to Alloy; image pulls, Let's Encrypt and acme-dns remain
  node/controller responsibilities, so this is workload isolation rather than
  a claim that the whole cluster is physically air-gapped.
- PostgreSQL and Redis accept only their named per-application namespace
  clients, not a broad shared application namespace.
- Garage's admin port accepts the security namespace only. S3 access is opened
  per real consumer rather than to every application.
- Application pods accept HTTP from the networking namespace.
- Each application receives only its declared database/cache egress plus DNS
  and narrowly required HTTPS egress.
- Homepage uses a read-only ServiceAccount. It cannot create, patch or delete
  cluster resources.
- Runner pods have no inbound path. HTTPS and Kubernetes API transport are
  allowed, but Kubernetes RBAC still determines authority.

NetworkPolicy enforcement requires a CNI that enforces Kubernetes
NetworkPolicies. Verify this explicitly on K3s before treating these manifests
as a security boundary.

## Scale-to-zero deployment runners

ARC is split into two releases: the controller and the `homelab-runners` scale
set. The scale set uses Kubernetes job-container mode, `minRunners: 0` and
`maxRunners: 1`. It does not mount the host Docker socket.

Before installing the scale set, create a least-privilege GitHub App and import
these keys at `secret/cicd/github-actions`:

```text
github_app_id
github_app_installation_id
github_app_private_key
```

VSO creates `github-actions-credentials` in `cicd`. The first cluster bootstrap
must run from the control machine because the in-cluster runner does not exist
yet. After ARC is healthy, use a workflow job container based on the
`homelabctl` image. That image includes pinned Helmfile, Helm, the diff plugin,
kubectl and a small API/TLS/DNS diagnostic toolset (`curl`, `jq`, `yq`,
OpenSSL and bind tools). It intentionally excludes Docker, Ansible, Terraform,
Vault CLI and language build toolchains.

Do not bind the runner ServiceAccount to `cluster-admin`. The intended flow is
a short-lived Vault-issued Kubernetes credential scoped to the deployment
operations. Start by proving a read-only `homelabctl deploy diff`, then add the
smallest apply role that the rendered releases require.

## Observability contract

Prometheus is the bounded metrics store and directly owns Kubernetes scraping,
including kube-state-metrics and node-exporter. Loki runs single-binary for pod
logs. Tempo runs monolithic for traces. Alloy is a DaemonSet that reads
Kubernetes pod logs and exposes OTLP/gRPC `4317` plus OTLP/HTTP `4318`; it
routes each signal to the appropriate backend. Grafana provisions Prometheus,
Loki and Tempo datasources and authenticates with its Butler-managed Pocket ID
client.

This avoids duplicate metric collection: Alloy receives pushed OTLP metrics,
while Prometheus owns pull-based cluster scraping. Retention is seven days for
the initial single-node budget. None of Loki, Tempo or Prometheus has an
Ingress. Pocket ID emits JSON stdout logs and exports native OTLP metrics and
traces to Alloy. OTLP log export stays disabled because Alloy already collects
container stdout; enabling both would duplicate every record in Loki. Query
arguments remain excluded because they may contain credentials, tokens or
personal data. Other workloads should use JSON stdout where their supported
configuration exposes it, while Alloy remains the single log-shipping path.

## First bootstrap checkpoints

Run from the repository root:

```bash
homelabctl deploy diff
```

Then deploy and verify checkpoints rather than applying the whole graph blindly:

1. apply `foundation`, then `networking`, then `secrets`;
2. let Butler initialize/unseal Vault, verify its Kubernetes-auth handoff and
   export the `butler-vault-init` recovery Secret off Titan;
3. apply `identity`, enroll the first Pocket ID owner, import one Pocket ID API
   key through break-glass and verify Butler's OIDC login;
4. apply `data`; Butler then reconciles Garage through its API;
5. apply `observability` and verify all three Grafana datasources;
6. apply ARC and applications only after their credentials and backups exist.

The exact commands and stop conditions are in [Bootstrap the cluster
platform](/operations/platform-bootstrap).

Use `homelabctl deploy apply <release>` for an individual release. Before any
stateful upgrade, take and verify the application's documented backup. A full
`deploy sync` is a reconciliation tool, not the first-bootstrap procedure.

## Verification still required on Titan

- Pod Security compatibility for every third-party image;
- NetworkPolicy enforcement by the selected K3s CNI;
- PostgreSQL init and restore with the VSO-generated script;
- Garage layout, bucket and key reconciliation through Butler against Titan;
- Pocket ID group/client API reconciliation and Butler PKCE login;
- Prometheus/Loki/Tempo datasource health and Pocket ID OTLP delivery;
- ARC scale from zero, one test job and cleanup back to zero;
- Pocket ID login and logout for each exposed application;
- off-node backups and clean restore of all important application data.
