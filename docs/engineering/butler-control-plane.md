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
| `internal/applications` | Non-secret `ApplicationIntegration` contracts and namespace validation |
| `internal/operations` | Bounded asynchronous operation records and audit-safe events |
| `internal/recovery` | Resumable Vault bootstrap state and Kubernetes TokenReview authentication |
| `internal/observability` | OpenTelemetry providers, OTLP export and W3C propagation |
| `internal/pocketid`, `vault`, `garage` | Provider adapters used by the domains |
| `internal/server` | Versioned HTTP transport, Pocket ID middleware and embedded UIs |

Transport handlers validate and authorize requests, then call a domain. They
do not contain provider credentials or return generated secret values.

## Two runtime identities

The normal and recovery Deployments use the same image but have different
service accounts, RBAC and HTTP surfaces.

| Property | Normal Butler | Butler recovery |
| --- | --- | --- |
| Kubernetes ServiceAccount | `butler` | `butler-recovery` |
| Network entry point | Traefik at `butler.home.6940469.xyz` | ClusterIP only; no Ingress |
| Human/client authentication | Pocket ID OIDC | ten-minute `butler-recovery-client` Kubernetes token |
| Vault identity after bootstrap | `auth/kubernetes/role/butler` | `auth/kubernetes/role/butler-recovery` |
| Recovery Secret access | none | exact access to `butler-vault-init` |
| Purpose | reconciliation and management | bootstrap, health and recovery input |

`homelabctl` creates an audience-bound token using TokenRequest. The recovery
server submits it to Kubernetes TokenReview and accepts only the exact
`system:serviceaccount:security:butler-recovery-client` username. A copied
normal service-account token or Pocket ID token is therefore insufficient.

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
4. `operational`.

Every Vault operation is idempotent. An initialized Vault without the selected
`butler-vault-init` recovery Secret is treated as unsafe and Butler refuses to
continue. Successful completion is recorded in the non-secret
`butler-bootstrap-state` ConfigMap. Repeating bootstrap after `operational` is
a no-op.

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

Normal UI/API requests use Authorization Code with PKCE and a stable public
client ID of `butler`. Butler verifies issuer, signature, expiry and audience.
It maps Pocket ID groups to a strict hierarchy:

| Pocket ID group | Butler role |
| --- | --- |
| `homelab-viewer` or no privileged group | viewer |
| `homelab-operator` | operator |
| `homelab-admin` | admin |

Viewers inspect status, identity and operation data. Operators may reconcile.
Administrators may create or disable non-admin users, replace membership,
rotate confidential OIDC clients and update application integrations. Butler
rejects Pocket ID administrator creation or promotion; first-owner and account
recovery remain deliberate Pocket ID procedures.

The first Pocket ID management API key is read from a local file and written
directly to Vault through the recovery API:

```bash
homelabctl control bootstrap --confirm \
  --pocket-id-api-key-file /secure/pocket-id-api-key
```

It is never passed as a command-line value, returned by Butler, or stored in an
operation event.

## Versioned management API

The current API is under `/api/v1`:

| Endpoint | Minimum role | Behaviour |
| --- | --- | --- |
| `GET /me`, `/status`, `/operations`, `/events` | viewer | Inspect control-plane state |
| `POST /reconcile` | operator | Queue an asynchronous reconciliation |
| `GET/POST /identity/users` | viewer/admin | List or create users |
| `PUT /identity/users/{id}` | admin | Update or disable a user |
| `PUT /identity/users/{id}/groups` | admin | Replace membership |
| `GET /identity/groups`, `/identity/clients` | viewer | Inspect provider metadata |
| `POST /identity/clients/{id}/rotate` | admin | Rotate directly into Vault |
| `GET/PUT /applications/{name}` | viewer/admin | Manage non-secret integration contracts |

Operations keep only ID, kind, actor, state, timestamps and sanitized errors.
Events contain no request bodies, tokens, API keys or provider responses.

## ApplicationIntegration

An integration records an application name, namespace, authentication pattern,
owner, ingress host and approved Vault paths. It is stored as JSON in the
`butler-application-integrations` ConfigMap; credentials are prohibited. The
domain reconciler confirms referenced namespaces exist. Provider-specific
reconcilers can consume this stable contract in later increments.

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
