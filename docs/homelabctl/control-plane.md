# Butler control-plane integration

The connected control commands are implemented against Butler's versioned API.
They remain separate from local recovery commands, which must work when Butler
or the cluster is unavailable.

## Interactive sign-in

```sh
homelabctl control login
homelabctl control status
homelabctl control logout
```

Login opens Pocket ID, uses Authorization Code with PKCE on a fixed loopback
callback, validates issuer, audience, state and nonce, then saves the short-lived
ID token in the private user config directory. `--token` and `BUTLER_TOKEN` are
explicit overrides and take precedence over the cached session.

## Short-lived Kubernetes access

```sh
homelabctl control credentials issue --role homelab-viewer --ttl 15m
homelabctl control credentials issue --role homelab-operator --ttl 30m --format json
```

The default response is a Kubernetes `ExecCredential`. Butler enforces the
role, namespace and maximum TTL server-side and never persists the token.

## Two operating planes

`homelabctl` will remain one binary with two explicit execution paths:

| Plane | Examples | Availability requirement |
| --- | --- | --- |
| Local recovery plane | setup, inventory, node, K3s, Helmfile and backups | Must work when the cluster or control service is down |
| Connected control plane | authentication, reconciliation status and platform operations | Requires the in-cluster service and network path |

The connected service must never become necessary to reinstall K3s, restore its
datastore or redeploy the service itself.

## Command families

```text
homelabctl control
├── login|logout
├── bootstrap|recovery
├── status|operations|events
├── users|groups|clients|applications
└── credentials issue
```

Use `homelabctl control --help` for the executable reference.

## Context model

A future context should bind related, non-secret operator targets:

```yaml
current-context: home
contexts:
  home:
    repository: /path/to/homelab
    kube-context: homelab
    control-url: https://control.home.arpa
    oidc-issuer: https://id.home.arpa
```

Tokens must live in the operating-system credential store, not this file. Local
commands may require only the repository or Kubernetes portion; connected
commands should work without a checkout.

## Control API responsibilities

The service may own:

- reconciliation status and bounded operation history;
- Pocket ID client provisioning;
- limited Vault policy and authentication configuration;
- internal CA publication;
- platform component and backup status;
- carefully scoped application operations later.

It must not own Debian users, SSH keys, K3s installation, Terraform execution,
arbitrary shell commands, raw secret retrieval or the only recovery credentials.

Desired state remains in Git and chart values. The API reports and reconciles
that state; it does not become a second configuration database.

## Intended API behaviour

The initial versioned API should expose resources such as:

```text
GET  /api/v1/status
GET  /api/v1/components
POST /api/v1/reconciliations
GET  /api/v1/reconciliations/{id}
GET  /api/v1/events
GET  /api/v1/pki/ca-chain
```

A reconcile request should return an operation identifier and run as a
single-flight asynchronous operation. The CLI can then support `--wait` without
holding an HTTP request open or starting overlapping reconciliations.

## Authentication requirements

Human login will use Pocket ID through an appropriate public-client flow. The
API must validate issuer, audience, expiry and authorisation claims and fail
closed when authentication configuration is missing. Viewer and operator
permissions should be distinct; a valid identity alone must not authorise a
mutation.

Refresh credentials belong in the OS credential store. CI must use a separate
machine identity rather than a copied human token.

## Bootstrap and trust boundary

The first internal CA cannot be trusted merely because it was downloaded from
an unauthenticated endpoint. Initial trust should be established through an
already authenticated Kubernetes connection or an independently verified CA
fingerprint.

Vault initialisation and unseal material also remain outside this service. The
service should eventually authenticate to an unsealed Vault with a narrowly
scoped workload identity; it must not retain a permanent root token or store the
only unseal key inside the same Kubernetes cluster.

## Implementation stages

1. context file and command-specific repository requirements;
2. typed HTTP client with timeouts, TLS policy and stable error responses;
3. read-only health and status commands;
4. Pocket ID login and OS-keychain token storage;
5. role-aware asynchronous reconciliation;
6. event history and structured output;
7. narrowly scoped application and backup operations.

Each stage must satisfy the repository-wide [definition of
done](/project/roadmap#definition-of-done-for-every-phase).
