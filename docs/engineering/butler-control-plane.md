# Butler control plane

Butler is the private API control plane for provider-owned state that Helm
cannot safely manage. It is a top-level Go module under `butler/`, uses
domain-oriented packages, and ships one immutable image with two explicitly
wired modes: `normal` and `recovery`.

## Domain structure

| Package | Responsibility |
| --- | --- |
| `internal/access` | Provider-independent principals, roles and authorization hierarchy |
| `internal/identity` | Pocket ID users, group membership, clients and credential rotation |
| `internal/platform` | Typed decoding and status updates for Butler's Kubernetes APIs |
| `internal/operations` | Bounded asynchronous operation records and audit-safe Kubernetes journal |
| `internal/recovery` | Resumable Vault bootstrap state and Kubernetes TokenReview authentication |
| `internal/observability` | OpenTelemetry providers, OTLP export and W3C propagation |
| `internal/pocketid`, `vault`, `garage` | Provider adapters used by the domains |
| `internal/server` | Versioned HTTP transport and Pocket ID middleware for both runtimes; JSON only, no browser assets |

Transport handlers validate and authorize requests, then call a domain. They
do not contain provider credentials or return generated secret values.

## Two runtime identities

The normal and recovery Deployments use the same image but have different
service accounts, RBAC and HTTP surfaces.

| Property | Normal Butler | Butler recovery |
| --- | --- | --- |
| Kubernetes ServiceAccount | `butler` | `butler-recovery` |
| Network entry point | Traefik at `butler.6940469.xyz` | ClusterIP only; no Ingress |
| Human/client authentication | Pocket ID OIDC | ten-minute `butler-recovery-client` Kubernetes token |
| Vault identity after bootstrap | `auth/kubernetes/role/butler` | `auth/kubernetes/role/butler-recovery` |
| Recovery Secret access | none | exact access to `butler-vault-init` |
| Purpose | reconciliation and management | bootstrap, health and recovery input |

## Unsealing after a restart

Vault is sealed by any node or pod restart, and a sealed Vault takes normal
Butler down with it because normal Butler authenticates to Vault through
Kubernetes auth.

Only the recovery workload may read `butler-vault-init`, so unsealing is the
recovery workload's job. It unseals Vault at startup and whenever it observes a
sealed Vault, using the recovery key in that Secret. Normal Butler never reads
the Secret and never unseals Vault.

The recovery workload never *initializes* Vault. Initialization mints the root
token and the recovery key, so it stays a deliberate operator action on a
pristine cluster.

`homelabctl control bootstrap --confirm` performs the same unseal when the
persisted bootstrap phase is already `operational`: a restart does not reset the
platform configuration, so the ceremony is not replayed and the identity
verification gate is not reopened.

`homelabctl` creates an audience-bound token using TokenRequest. The recovery
server submits it to Kubernetes TokenReview and accepts only the exact
`system:serviceaccount:security:butler-recovery-client` username. A copied
normal service-account token or Pocket ID token is therefore insufficient.
The recovery and normal Butler NetworkPolicies allow TokenReview and other
Kubernetes API calls only to the configured API Service and node endpoint
`/32`s on ports 443/6443. Namespace selectors cannot represent a virtual
Service IP or host-network API server; future nodes must update these values
through their cluster profile rather than widening access to the LAN CIDR.
Recovery Butler additionally has pod-selected egress to Vault on port 8200 and
Pocket ID on Service/container ports 80/1411. Both Pocket ID ports account for
policy enforcement before or after Kubernetes Service DNAT. This path is
required only for the resumable identity bootstrap: recovery creates and
verifies the provider configuration before normal Butler's Pocket
ID-authenticated surface can become authoritative.
Normal Butler also resolves the public Pocket ID issuer and validates its OIDC
discovery and signing keys over `https://auth.6940469.xyz`. That address enters
the cluster through Traefik, so its policy narrowly permits egress to
Traefik-selected pods in `networking` on ports 443/8443. This covers policy
enforcement after Service DNAT without granting general namespace or internet
egress.

## Resumable bootstrap

Run:

```bash
homelabctl control bootstrap --confirm
homelabctl control recovery
```

The recovery API reports one of these phases:

1. `initialize-vault`;
2. `unseal-vault`;
3. `configure-vault`;
4. `awaiting-dns-delegation`;
5. `awaiting-certificate`;
6. `awaiting-pocket-id-credential`;
7. `configure-identity`;
8. `awaiting-identity-verification`;
9. `operational`.

Every Vault operation is idempotent. An initialized Vault without the selected
`butler-vault-init` recovery Secret is treated as unsafe and Butler refuses to
continue. Successful completion is recorded in the non-secret
`butler-bootstrap-state` ConfigMap. Vault configuration never skips the human
identity acceptance gate. Repeating bootstrap after `operational` is a no-op.
The foundation pass does not contact Pocket ID. Vault's JWT/OIDC method and
group roles are configured atomically only after Butler has created Vault's
confidential Pocket ID client and stored its complete credential.
The bounded recovery Vault role can create, read and update only the bootstrap
identity documents at `security/pocket-id`, `oauth/homelabctl`, and
`oauth/vault`, in addition to the acme-dns document.
It has no wildcard `oauth/*` permission; application clients remain owned by
normal Butler after the identity gate is operational.

The initialization root token and unseal key live in the named recovery Secret
because that is the chosen single-node recovery model. Normal Butler cannot
read or mount it. Recovery bootstrap holds root only in memory while creating
mounts, policies and Kubernetes auth; it then switches to the short-lived,
narrow `butler-recovery` Vault role.

Export the Secret without producing plaintext on disk:

```bash
homelabctl control recovery export \
  --output /secure/titan-vault.age \
  --age-recipient "$AGE_RECIPIENT"
```

The CLI reads the Secret into memory, retains only its data map, writes a new
mode-0600 age file with create-exclusive semantics, and refuses repository or
filesystem-root destinations.

## Pocket ID and authorization

There is one first-party client, `homelabctl` (the CLI), which logs in with
Authorization Code and PKCE against Pocket ID directly. Butler is not a client;
it is the resource server that verifies issuer, signature, expiry and audience
on every bearer token. Pocket ID stamps the requesting client's ID as the ID
token audience, so Butler accepts that client ID and nothing else - a token
minted for `vault`, `grafana` or any other client is rejected. Butler maps
Pocket ID groups to two roles. The operator tier exists in the model but is
disabled in production by naming no group for it:

| Pocket ID group | Butler role |
| --- | --- |
| `butlerviewer` | viewer |
| `butleradmin` | admin |
| any other group, including `household` | none - every request is denied |

The group names are configuration (`oidc.groups`), not constants. An empty name
disables a tier, which is how the middle operator tier is off: `operator: ""`
in both the embedded config and the chart values. The `access.Operator` role and
its hierarchy stay in the code so a deployment can re-enable a middle tier by
naming one, but no group grants it on Titan.

Authorization groups are a nine-name vocabulary shared by Butler, Vault,
Kubernetes and the applications. Each is enforced by whoever owns the
authority:

| Group | Enforced by | Grant |
| --- | --- | --- |
| `butleradmin` | Butler (`oidc.groups.admin`) | Butler admin |
| `butlerviewer` | Butler (`oidc.groups.viewer`) | Butler viewer |
| `vaultadmin` | Vault JWT role | `vault-admin` policy |
| `vaultread` | Vault JWT role | `vault-read` policy: may read secret values |
| `vaultmeta` | Vault JWT role | `vault-metadata` policy: metadata only |
| `k8sadmin` | Vault JWT role | `k8s-admin` policy |
| `k8soperator` | Vault JWT role | `k8s-operator` policy |
| `k8sviewer` | Vault JWT role | `k8s-viewer` policy |
| `household` | app client restrictions | access to the shared applications |

`vaultread` grants `read` on `secret/data/*`, so it is the explicit exception to
metadata-only access. `vaultmeta` grants `read` and `list` on `secret/metadata`
only: a member can enumerate paths and versions but `vault kv get` is denied.
`vaultadmin` remains the wildcard administrator. Note that the Kubernetes side
is *not* symmetric with Vault: `k8sviewer` binds the built-in `view` role, which
excludes Secrets, while `k8soperator` binds `edit`, which includes them. An
operator can therefore read a Kubernetes Secret directly even when they hold no
Vault secret-read policy.

Holding an account at Pocket ID grants nothing by itself: an identity that
belongs to no Butler group is denied, not silently treated as a viewer. Vault
and Kubernetes groups do not widen the Butler role either, because Butler never
reads them. A household member therefore cannot read Butler's status, identity
or operation data simply by being able to sign in to the applications.

Butler does not broker cluster credentials. Cluster access is minted by Vault as
the requesting user, using the policy bound to their `k8s*` group, so
`butleradmin` no longer implies `cluster-admin`:

```bash
homelabctl credentials --role k8sviewer
KUBECONFIG=/tmp/homelab.kubeconfig kubectl get nodes
```

`homelabctl credentials` performs its own Vault OIDC login and calls
`kubernetes/creds/<role>`; the printed kubeconfig is the only artifact and goes
to stdout unless `--output` names a mode-0600 file. When the identity's group
does not carry the role, Vault's own permission error is printed verbatim
because that is the authorization decision.

Butler generates Pocket ID's machine credential as part of the existing
`pocket-id-runtime` `ManagedCredential`. It writes the 48-character value
directly to `secret/security/pocket-id`; VSO projects the same document to the
`pocket-id-credentials` Kubernetes Secret, and Pocket ID consumes the
`static-api-key` field through `STATIC_API_KEY`. The synthetic static API user
is deliberately separate from the first human owner, so owner creation and
passkey enrollment remain available.

The normal bootstrap therefore needs no credential file:

```bash
homelabctl control bootstrap --confirm
```

The generated value is never returned by Butler or stored in an operation
event. `--pocket-id-api-key-file` remains only as a break-glass replacement
mechanism. It preserves the Pocket ID encryption key already stored in the
same Vault document and must not be used during routine bootstrap.

After reconciliation, the operator must prove both authentication paths:

```bash
homelabctl control login
homelabctl control verify-identity --confirm
```

Normal `homelabctl control` requests use
`https://butler.6940469.xyz` through Traefik by default. An explicit
`--port-forward` remains available for diagnosing ingress failures. Bootstrap
and recovery never inherit the normal address: they continue through the
non-ingressed recovery Service using a fresh audience-bound Kubernetes token.

The first command proves Pocket ID issuer, audience, nonce, PKCE and Butler
role mapping. The second checks `/api/v1/me`, performs Vault's browser OIDC
flow against the `vaultadmin` role, looks up the temporary token's policies,
and revokes it. Only non-secret evidence reaches
`POST /api/v1/bootstrap/identity-verification`; Butler requires an admin role
and both `vault-admin` and `k8s-admin` policies before changing the persisted
phase to `operational`. The Vault token is held only in workstation memory and
is never sent to Butler.

## Versioned management API

The current API is under `/api/v1`:

| Endpoint | Minimum role | Behaviour |
| --- | --- | --- |
| `GET /me`, `/status`, `/operations`, `/events` | viewer | Inspect control-plane state |
| `POST /reconcile` | operator (admin while the operator tier is disabled) | Queue an asynchronous reconciliation |
| `GET/POST /identity/users` | viewer/admin | List or create users |
| `PUT /identity/users/{id}` | admin | Update or disable a user |
| `PUT /identity/users/{id}/groups` | admin | Replace membership |
| `GET /identity/groups`, `/identity/clients` | viewer | Inspect provider metadata |
| `POST /identity/clients/{id}/rotate` | admin | Rotate directly into Vault |
| `POST /bootstrap/identity-verification` | recovery identity | Accept non-secret evidence for both real human login paths |

There is deliberately no credential endpoint. `POST
/api/v1/access/kubernetes-credentials` was removed with the credential broker:
it gated cluster access behind Butler admin, so holding `butleradmin` implied
`cluster-admin`. Vault now mints cluster credentials as the requesting identity
through the policy bound to that identity's `k8s*` group, which means the
Butler role and the cluster role are independent decisions.

## Operator surfaces and break-glass access

Butler serves no browser surface. Its normal runtime exposes only the Pocket
ID-authenticated JSON API under `/api/v1`, and its recovery runtime exposes only
its own JSON API; neither serves HTML, CSS or JavaScript. `homelabctl control`
is therefore the only control surface, including the `control recovery` family
that uses the non-ingressed recovery service. Recovery export and Pocket ID
machine-key replacement stay CLI-only because secret material never leaves CLI
memory.

```bash
homelabctl control recovery
homelabctl control bootstrap --confirm
homelabctl control reconcile
```

The CLI creates the audience-bound ten-minute recovery token and the Kubernetes
port-forward for every recovery call. The token never appears in a URL, cookie,
local storage, or server response. Closing a command ends the port-forward, and
token expiry bounds access even if the workstation is left unattended.

Do not expose recovery Butler through Traefik. Do not place Pocket ID
forward-auth in front of Vault: Vault's native Pocket ID OIDC is the normal
login, while direct token login remains the lower-layer break-glass path when
Pocket ID is unavailable.

Operations keep only ID, kind, actor, state, timestamps and sanitized errors.
Events contain no request bodies, tokens, API keys or provider responses. Each
entry is persisted as a `ButlerOperation` object, so Butler remains stateless
and its API history survives pod replacement without a dedicated PVC.
Kubernetes writes refetch current objects and retry optimistic-lock conflicts.
Timer-driven and operator-triggered reconciliations are serialized because
provider creation and one-time secret responses are unsafe to race.

Butler also watches `PocketIDClient`, `PocketIDGroup`, `ManagedCredential`,
`GarageBucket` and `VaultOIDCRole` resources. Creates, deletes and spec-generation changes send a
coalesced immediate trigger to that same serialized scheduler; Butler's own
status updates do not retrigger it. The one-minute timer is a drift-repair
resync for missed events rather than the normal convergence path. Garage is
skipped until at least one `GarageBucket` exists, so installing earlier stages
does not poll an absent provider or credential.

## Declarative platform APIs

The `platform.6940469.xyz/v1alpha1` API group is installed by the standalone
`butler-crds` foundation release. Application charts own their declarations:

| Kind | Scope | Purpose |
| --- | --- | --- |
| `PocketIDClient` | Namespaced | Reconcile a public or confidential Pocket ID client and store its credential at the declared Vault path |
| `PocketIDGroup` | Cluster | Reconcile the small shared authorization group vocabulary |
| `ManagedCredential` | Namespaced | Generate immutable values, render dependent values, or project selected Vault fields into a consumer-specific path |
| `GarageBucket` | Namespaced | Create a Garage bucket, key and least-privilege grants, with credentials written directly to Vault |
| `VaultOIDCRole` | Namespaced | Converge one Vault `auth/jwt/role/<group>` from a declared group and policy set |
| `ButlerOperation` | Namespaced | Durable secret-free operation and event journal used by the Butler API |

No secret value is stored in a custom resource or status. Status contains only
the observed generation, provider identifier and a standard `Ready` condition.
Deleting a declaration does not delete provider data in this initial API; that
retain-by-default rule prevents an accidental Helm removal destroying an OIDC
client, Vault material, or bucket.

There is deliberately no `ApplicationIntegration`. It duplicated chart
metadata without driving a provider. A chart now declares only the concrete
capabilities it consumes, next to its deployment and `VaultStaticSecret`.

### Ownership and examples

The resource namespace is the owning chart's namespace; it is not where Vault
stores the value. Provider names and Vault paths must remain globally unique,
and Butler marks every conflicting declaration `Ready=False` rather than
allowing last-writer-wins behaviour. Ownership is checked across all three
secret-producing resource kinds, so a `ManagedCredential` cannot overwrite the
output of a `PocketIDClient` or `GarageBucket`.

```yaml
apiVersion: platform.6940469.xyz/v1alpha1
kind: ManagedCredential
metadata:
  name: paperless-database
  namespace: paperless-ngx
spec:
  vaultPath: applications/paperless-ngx/database
  fields:
    username:
      value: paperless
    password:
      sourceRef:
        path: databases/postgresql
        key: paperless-password
---
apiVersion: platform.6940469.xyz/v1alpha1
kind: PocketIDClient
metadata:
  name: grafana
  namespace: monitoring
spec:
  type: confidential
  redirectURIs:
    - https://grafana.6940469.xyz/login/generic_oauth
  vaultPath: oauth/grafana
```

`ManagedCredential` fields accept exactly one of `generate`, `value`,
`template`, or `sourceRef`. Generated fields are created only when missing;
explicit values and projections converge without rotating unrelated fields.
Templates reference sibling keys as `{{key-name}}`; dependency ordering is
deterministic and unknown references or cycles fail reconciliation rather than
being written literally. `PocketIDClient` writes
`client_id` and, for confidential clients, `client_secret` directly to its
Vault path. `GarageBucket` similarly writes the generated S3 access key without
placing it in Kubernetes resource status.

Admission rejects traversal-like Vault paths, insecure non-loopback redirect
URIs, invalid bucket aliases and buckets with no permissions. Status and
operation records expose stable diagnostic reasons but never persist raw
provider errors. Manual OIDC secret rotation resolves the owning
`PocketIDClient` and uses its declared Vault path; ambiguous or undeclared
clients are refused before Pocket ID rotates anything. Rotation creates and
persists the replacement first, records its provider secret ID beside the
credential, and only then revokes older Pocket ID secrets. If Vault rejects a
one-time Pocket ID or Garage value, Butler immediately revokes the newly
created provider credential so an unknown valid key is not left behind.

Inspect reconciliation without reading any secret value:

```bash
kubectl get managedcredentials,pocketidclients,garagebuckets -A
kubectl get vaultoidcroles -n security
kubectl get pocketidgroups
kubectl describe managedcredential paperless-database -n paperless-ngx
kubectl get butleroperations -n security
```

### Vault owning its roles

`VaultOIDCRole` is declared by the Vault chart, beside the Vault release, and
declares exactly one group and its policy list. Butler reconciles it into
`auth/jwt/role/<spec.group>` with `role_type: oidc`, `user_claim: sub`,
`groups_claim: groups`, the Vault OIDC client ID as `bound_audiences`,
`bound_claims.groups` pinned to the declared group, the declared `policies`, the
declared `ttl`/`maxTTL` (defaults `1h`/`8h`), and both the loopback and UI
redirect URIs. Each pass compares the managed fields with the live role and
rewrites only on drift.

Two rules keep that authority safe. A declaration outside the Butler/Vault
namespace is ignored and logged: accepting any namespace would let a namespace
owner grant themselves a Vault policy. A group declared by more than one
resource fails reconciliation instead of last-writer-wins. The `k8sIssuance`
roles remain, but they configure the *Kubernetes secrets engine* only - the
`kubernetes/roles/<name>` entry and the ServiceAccount whose token it mints.
They no longer carry a group or policy list, so human authorization has exactly
one source of truth.

CRD schemas are upgraded through the `butler-crds` foundation chart. Do not
hand-apply copies of the definitions, and do not put provider declarations in
Butler's ConfigMap. Removal retains external provider state in `v1alpha1`; use
an explicit provider operation for destructive cleanup when one is added.

## Production envelope

This design is production-ready for the intended private, single-node Titan
homelab after the platform bootstrap and recovery acceptance tests pass. It is
not an HA control plane: Butler deliberately runs one replica with `Recreate`,
Vault and stateful workloads use single-node storage, and a Titan outage stops
normal service. Production readiness here means least privilege, bounded
resources, deterministic reconciliation, no secret-bearing CRDs or audit
records, validated recovery material and rehearsed restores—not uninterrupted
service through node loss.

Before trusting it with important data, complete the encrypted off-node K3s and
Vault exports, deploy by immutable image SHA, verify Pocket ID and Vault
identity handoff, prove wildcard certificate renewal, and restore each stateful
service. Multi-replica Butler would additionally require leader election and is
explicitly outside this single-node release.

## Telemetry

Butler exports OTLP/HTTP to Alloy when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.
Instrumentation includes:

- inbound normal and recovery HTTP traces and metrics;
- outbound Pocket ID, Vault and Garage HTTP spans;
- a span, run counter and duration histogram for every reconciler;
- W3C Trace Context propagation;
- JSON request logs containing trace and span IDs without queries or bodies.

Alloy routes metrics to Prometheus, logs to Loki and traces to Tempo. Grafana
provisions a Butler dashboard with deployment health, reconciliation rates,
logs and traces. Prometheus provisions Butler availability and reconciliation
alerts. Loss of the OTLP receiver does not fail API requests or reconciliation.

## Failure order

When normal authentication is unavailable, recover in this order:

1. use LAN Kubernetes administration and `homelabctl control recovery`;
2. initialize or unseal Vault;
3. verify VSO secret projection;
4. recover Pocket ID and complete owner login;
5. verify normal Butler Pocket ID login;
6. run normal reconciliation.

Neither normal Butler nor Pocket ID is needed to reach the lower-layer recovery
service. Losing Kubernetes itself requires the separate K3s recovery bundle and
Ansible runbooks.
