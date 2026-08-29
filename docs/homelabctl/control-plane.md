# Butler control-plane integration

The connected control commands are implemented against Butler's versioned API.
They remain separate from local recovery commands, which must work when Butler
or the cluster is unavailable.

## Interactive sign-in

```sh
homelabctl control login
homelabctl control status
homelabctl control verify-identity --confirm
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
├── bootstrap|verify-identity|recovery
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

Tokens do not belong in this future context file. The current Pocket ID session
uses a private mode-0600 file in the user config directory; an OS credential
store is a later hardening option. Local commands may require only the
repository or Kubernetes portion; connected commands should work without a
checkout.

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

## API behaviour

The versioned API exposes resources including:

```text
GET  /api/v1/status
GET  /api/v1/operations
POST /api/v1/reconcile
GET  /api/v1/events
GET  /api/v1/pki/ca-chain
POST /api/v1/access/kubernetes-credentials
POST /api/v1/bootstrap/identity-verification
```

A reconcile request should return an operation identifier and run as a
single-flight asynchronous operation. The CLI can then support `--wait` without
holding an HTTP request open or starting overlapping reconciliations.

## Authentication requirements

Human login uses Pocket ID Authorization Code with PKCE. The API validates
issuer, audience, expiry and authorisation claims and fails
closed when authentication configuration is missing. Viewer and operator
permissions should be distinct; a valid identity alone must not authorise a
mutation.

CI must use a separate machine identity rather than a copied human token.

## Bootstrap and trust boundary

The first internal CA cannot be trusted merely because it was downloaded from
an unauthenticated endpoint. Initial trust should be established through an
already authenticated Kubernetes connection or an independently verified CA
fingerprint.

Recovery Butler owns the explicitly confirmed, one-time Vault initialization.
For this single-node design it stores the root token and unseal key in the
narrowly RBAC-protected `butler-vault-init` Secret, which the operator must
export immediately to an age-encrypted off-cluster copy. Normal Butler cannot
read that Secret. After foundation setup, both Butler runtimes authenticate to
Vault with projected Kubernetes ServiceAccount tokens and bounded roles.

Bootstrap remains `awaiting-identity-verification` until the operator proves a
Pocket ID admin login to normal Butler and completes Vault's separate browser
OIDC flow. `homelabctl` verifies the Vault policies, revokes the temporary token
and submits only non-secret acceptance evidence through the recovery API.

## Current boundary

The typed client, Pocket ID browser login, role-aware API, persisted operation
history, application integrations, bounded credential issuance and recovery
workflow are implemented. Titan deployment, first-owner enrollment, real
integration execution and restore rehearsal remain operational checkpoints,
not repository claims. See [current state](/project/current-state) and the
[platform bootstrap runbook](/operations/platform-bootstrap).
