# Bootstrap the cluster platform

Use this runbook only after Titan is healthy and kubeconfig points at Titan.
The first deployment runs from the Mac; the scale-to-zero runner cannot deploy
the platform that creates it.

Recovery custody is not a precondition for this runbook. The platform stages
below are infrastructure and configuration: they are declared in this
repository and can be recreated from it. The constraint that matters is that
irreplaceable household data must not be stored in the cluster until an
off-cluster copy exists — see the [decision log](/project/decisions).

## Current installation checkpoint

Titan passed the cluster-readiness checkpoint on 31 August 2026:

- `homelabctl cluster status` reported the single `titan` control-plane and
  etcd node as `Ready` at `192.168.1.163`;
- `kubectl --context homelab get nodes` reported K3s `v1.36.4+k3s1`;
- CoreDNS and local-path-provisioner were both `Running`; and
- no pods were outside the `Running` or `Succeeded` phases.

The foundation stage passed its live acceptance checkpoint on 1 September
2026:

- the `v1beta1.metrics.k8s.io` APIService reported `Available=True`;
- Metrics Server returned live usage for `titan` (`227m` CPU and `1022Mi`
  memory at the checkpoint); and
- the next action is **2. Networking and certificate prerequisites** below.

The recorded usage values are evidence that the metrics pipeline works, not
capacity thresholds; they will naturally change between checks.

The networking stage passed its pre-secrets checkpoint on 1 September 2026:

- `cert-manager`, `public-certificates` and `traefik` are deployed as separate
  healthy Helm releases;
- all three cert-manager pods are `Running` without restarts;
- Traefik rolled out and its LoadBalancer address is Titan's reserved
  `192.168.1.163` on ports 80 and 443;
- the production issuer is `False` only because `cert-manager/acme-dns` does
  not exist yet; and
- the wildcard Certificate created its request and temporary private key and
  is waiting for that issuer.

Vault, recovery Butler, VSO, Pocket ID and the public certificate ceremony have
since converged. The first owner has enrolled a passkey, belongs to
`butleradmin`, Butler-managed identity resources report Ready, Pocket ID PKCE
login succeeds, and the normal Butler status API succeeds through
`https://butler.6940469.xyz`.

The identity stage completed on 1 September 2026: `homelabctl control
verify-identity` proved Pocket ID login to both Butler and Vault, and Butler's
recovery plane reports `operational`. The next action is **not** another step in
**4. Pocket ID and management handoff**. It is the recovery custody work recorded
in [current state](/project/current-state): export the Vault recovery material to
verified encrypted off-device storage, complete the K3s recovery export and
restore rehearsal, and only then advance to **5. Shared data and API
provisioning**.

Do not repeat that checkpoint merely to advance this runbook. Continue to use
`homelabctl cluster status` as a quick safety check before each apply.

The intended one-time certificate ceremony is:

1. Butler recovery initializes Vault and registers one acme-dns account.
2. Butler writes the credential to Vault and displays the generated CNAME
   target without logging the password.
3. The operator adds `_acme-challenge.6940469.xyz` as a permanent CNAME in
   Namecheap and asks Butler to verify it.
4. Vault Secrets Operator projects the credential into the `cert-manager`
   namespace.
5. cert-manager obtains one production Let's Encrypt certificate for
   `6940469.xyz` and `*.6940469.xyz`; Traefik serves it and cert-manager renews
   it without further Namecheap changes.

There is no staging-to-production promotion in the supported operator flow.
The supported configuration contains only the production issuer. Test changes
to the solver separately before altering an established registration.

## Before the first apply

Confirm `*.6940469.xyz` and `6940469.xyz` resolve to Titan's reserved LAN
address, no AAAA record exists and there is no router port-forward. Publish the
Butler image for the current commit.

Deploy commands default to the full Git SHA, so that immutable image must exist.

```bash
homelabctl cluster status
homelabctl deploy diff
homelabctl deploy platform --through secrets --confirm
```

Stop if the context is unexpected or the diff deletes persistent state.

`deploy platform` is the normal installation path. It applies the Helmfile
stages in this fixed order: `foundation`, `networking`, `secrets`, `identity`,
`data`, `observability`, `cicd`, then `applications`. `--through identity` is
the safe default. Later stages are refused until Butler records successful
Pocket ID authentication to both Butler and Vault. The individual stage
commands below remain useful for inspection and recovery.

## 1. Foundation

```bash
homelabctl deploy apply --stage foundation
kubectl --context homelab get namespaces
kubectl --context homelab -n kube-system rollout status deployment/metrics-server
kubectl --context homelab get apiservice v1beta1.metrics.k8s.io
kubectl --context homelab top node titan
```

This creates platform namespaces, per-application namespaces, Pod Security
labels, foundational RBAC and the `platform.6940469.xyz` CRDs, then installs
the separately pinned Metrics Server release that replaces the disabled K3s
package. Do not continue until its deployment is available, the aggregated
Metrics API reports `Available=True` and `kubectl top` returns Titan CPU and
memory usage.

**Titan checkpoint:** complete on 1 September 2026. Do not reapply foundation
merely to continue the initial installation.

## 2. Networking and certificate prerequisites

```bash
homelabctl deploy apply --stage networking
kubectl --context homelab -n networking rollout status deployment/traefik
kubectl --context homelab -n cert-manager get pods
helm --kube-context homelab list --all --namespace cert-manager
helm --kube-context homelab list --all --namespace networking
kubectl --context homelab get clusterissuer letsencrypt-production
kubectl --context homelab -n networking get certificate homelab-wildcard
```

The stage deliberately uses separate `cert-manager` and
`public-certificates` Helm releases. A fresh Kubernetes API cannot validate a
`Certificate` or `ClusterIssuer` during the same Helm transaction that first
installs their CRDs. Helmfile therefore waits for cert-manager to finish, then
submits the issuer and wildcard certificate in the dependent release.

`letsencrypt-production` and `homelab-wildcard` initially remain unready because
the acme-dns credential does not exist yet. This is expected; cert-manager
retries after VSO creates the Secret.

Continue to the secrets stage only when Traefik has rolled out and every
cert-manager pod is `Running`. Do not wait for the ClusterIssuer or Certificate
to become ready at this point: Butler creates their missing acme-dns credential
in the next stage.

**Titan checkpoint:** complete on 1 September 2026. The observed issuer message
was exactly `failed to get secret "acme-dns"`, which is the intended handoff to
the secrets stage.

If the original combined first-install transaction failed with `no matches for
kind Certificate` or `no matches for kind ClusterIssuer`, no uninstall is
required: that failed Helm install did not create a cert-manager release.
Update to the split-release revision and rerun the networking stage.

## 3. Vault, recovery Butler and VSO

This is the first stage that consumes the repository-built Butler image. CI
builds pull-request images for validation but pushes immutable SHA tags only
after a successful `main` build. Before applying secrets, merge the reviewed
revision, wait for the `main` image-publication job, check out that exact clean
`main` commit and confirm its full SHA. Do not apply this stage from a feature
branch merely because foundation and networking succeeded; those stages do not
consume Butler.

```bash
git switch main
git pull --ff-only
git status --short
git rev-parse HEAD
homelabctl update
homelabctl deploy diff --stage secrets
```

`git status --short` must print nothing. The diff's Butler image tag must equal
the full `git rev-parse HEAD` value, and the matching image-publish job on
`main` must have succeeded.

```bash
homelabctl deploy apply --stage secrets
kubectl -n security rollout status deployment/butler-recovery
homelabctl control recovery
```

The recovery plane is CLI-only: every break-glass action, including Vault
recovery export and any operation that transports secret material, runs through
`homelabctl control` and never through a browser surface. Prefer the ordinary
command output during routine bootstrap; the recovery commands exist for
deliberate recovery work, not as a second daily dashboard.

The recovery command creates a ten-minute audience-bound token, opens a
loopback-only port-forward and asks recovery Butler to validate that token with
the Kubernetes TokenReview API. Its NetworkPolicy permits only the reviewed
K3s API Service and Titan endpoint `/32`s on ports 443/6443. A `503 recovery
authentication unavailable` response means that lower-layer API path is not
working; do not initialize Vault until it is fixed.

Normal Butler cannot initialize Vault and cannot read the recovery Secret.
Advance the no-Ingress recovery service explicitly:

```bash
homelabctl control bootstrap --confirm
homelabctl control recovery
```

The phase progresses through `initialize-vault`, `unseal-vault` and
`configure-vault`, registers exactly one acme-dns account, stores its complete
credential at `secret/infrastructure/acme-dns`, then pauses at
`awaiting-dns-delegation`. The operation
is idempotent and refuses an already initialized Vault if `butler-vault-init`
is missing. Successful Vault foundation creates the bounded normal and
recovery Kubernetes-auth roles; it does not contact Pocket ID or claim that
identity works yet. Vault JWT/OIDC is deliberately deferred until the later
identity phase has created Vault's confidential Pocket ID client. A failure
from `auth/jwt/config` while Pocket ID is absent indicates an older Butler
image; deploy the current `main` image and repeat this resumable command. Do
not reset Vault or delete its PVC or recovery Secret.

After the certificate becomes ready, recovery Butler reconciles the
`pocket-id-runtime` `ManagedCredential`. It generates the Pocket ID encryption
key and a 48-character static machine credential directly at
`secret/security/pocket-id`, preserving either field when it already exists.
Its bounded Vault policy permits `create`, `read` and `update` only on that
exact KV path. If generation cannot yet converge, bootstrap reports
`awaiting-pocket-id-credential`; deploy the current `main` image, inspect the
`ManagedCredential`, and repeat the idempotent command.

Display the non-secret registration metadata:

```bash
homelabctl control certificate status
```

In Namecheap **Advanced DNS**, add the displayed record exactly once:

| Field | Value |
| --- | --- |
| Type | `CNAME Record` |
| Host | `_acme-challenge` |
| Value | the exact `cnameTarget` printed by Butler |
| TTL | `Automatic` |

Keep the record permanently. Do not change the existing apex or wildcard A
records. Verify propagation and let Butler accept the exact match:

```bash
dig +short CNAME _acme-challenge.6940469.xyz
homelabctl control certificate verify-dns --confirm
```

VSO projects only `acmedns.json` into the `cert-manager/acme-dns` Secret.
Wait for the production certificate and advance bootstrap once more:

```bash
kubectl get clusterissuer letsencrypt-production
kubectl -n networking get certificate homelab-wildcard --watch
homelabctl control certificate status
homelabctl control bootstrap --confirm
```

Continue only when `certificateReady` is true. The next bootstrap pass
generates the Pocket ID runtime credential and advances identity configuration.
cert-manager renews with the same account and
CNAME; there is no later Namecheap ceremony.

Export the root token and unseal key directly into a new encrypted file:

```bash
homelabctl control recovery export \
  --output /secure/homelab-recovery/titan-vault.age \
  --age-recipient "$AGE_RECIPIENT"
```

No plaintext recovery file is created. The output uses mode `0600`, existing
files are not overwritten, and repository destinations are rejected. Store a
verified copy away from Titan and the control workstation.

Verify normal Butler and VSO convergence:

```bash
kubectl -n security rollout status deployment/butler
kubectl -n security get vaultauth,vaultconnection
kubectl get vaultauth,vaultstaticsecret -A
kubectl get clusterissuer letsencrypt-production
kubectl -n cert-manager get secret acme-dns
kubectl -n networking get certificate homelab-wildcard
kubectl -n networking get secret homelab-wildcard-tls
kubectl get pocketidclients,managedcredentials,garagebuckets -A
kubectl get pocketidgroups
```

The custom resources contain intent and non-secret status only. Butler writes
generated or provider-issued credentials directly to Vault; VSO then projects
ordinary Kubernetes Secrets for workloads. `kubectl describe` should show a
`Ready=True` condition after reconciliation.

## 4. Pocket ID and management handoff

```bash
homelabctl deploy apply --stage identity
kubectl -n security wait --for=condition=Ready \
  managedcredential/pocket-id-runtime --timeout=2m
kubectl -n security wait --for=condition=Ready \
  vaultstaticsecret/pocket-id-credentials --timeout=2m
kubectl -n databases wait --for=condition=complete \
  job/postgresql-database-provisioning --timeout=5m
kubectl -n security get vaultdatabaserole pocketid
kubectl -n security wait --for=condition=Ready \
  vaultdynamicsecret/pocket-id-database --timeout=2m
kubectl -n security rollout status deployment/pocket-id
```

Creating the `ManagedCredential` immediately triggers Butler's coalesced
Kubernetes event watcher. Butler writes both the generated encryption key and
Pocket ID static machine credential to Vault, VSO projects
`pocket-id-credentials`, and only then can the Pocket ID container start.

Pocket ID keeps its identity data in PostgreSQL. The PostgreSQL release is a
declared dependency of the Pocket ID release, so applying the identity stage
applies the database with it. The chain is:

1. the PostgreSQL provisioning Job waits for VSO to project the admin
   credential, then creates a passwordless `pocketid` role and a `pocketid`
   database owned by it. It never sets a password;
2. Butler configures `database/config/postgresql` from
   `secret/databases/postgresql` and converges the `VaultDatabaseRole` into
   `database/static-roles/pocketid`. Vault rotates the password once on import
   to learn it;
3. VSO reads `database/static-creds/pocketid`, composes
   `DB_CONNECTION_STRING`, and projects it into the `security` namespace as
   `pocket-id-database`;
4. the container reads that key and runs its migrations as the stable
   `pocketid` role, so the schema survives a later rotation.

Check the credentials without printing them:

```bash
vault read database/static-roles/pocketid
kubectl -n databases get job/postgresql-database-provisioning
kubectl -n security get vaultdatabaserole pocketid vaultdynamicsecret/pocket-id-database
kubectl -n security get secret pocket-id-database \
  -o jsonpath='{.data.DB_CONNECTION_STRING}' | base64 -d | sed 's#://[^@]*@#://<redacted>@#'
```

Once the `VaultDynamicSecret` is `Ready`, delete the legacy copied credential.
Vault retains provider state by default, so removing the `ManagedCredential`
does not remove the old path:

```bash
vault kv metadata delete secret/security/pocket-id/database
```

Rotation is deliberate, not a lease:

```bash
vault write -f database/rotate-role/pocketid
```

A pod restart re-reads the same password. Only `rotate-role` changes it, and
VSO restarts Pocket ID through `rolloutRestartTargets`. If Pocket ID cannot
start and the credential is suspect, break-glass is host access and then
`psql` on `postgresql-0` in the `databases` namespace; do not edit the shared
KV document.

Uploads such as logos and profile pictures use `FILE_BACKEND=database`, so they
live in PostgreSQL too. The persistent volume keeps only the re-downloadable
GeoLite database.
Butler retains a one-minute drift-repair resync if an event is missed.
Do not manually create the destination Secret or restart VSO while this chain
is converging.

### Rebuilding the identity database

The PostgreSQL and Pocket ID volumes hold only identity data. Rebuild them
together when the schema is compromised or the credentials cannot be
recovered. The sequence is destructive: it deletes every Pocket ID user,
client and upload, then recreates the declared clients from their CRs.

```bash
kubectl -n security scale deployment/pocket-id --replicas=0
kubectl -n security delete pvc pocket-id-data
kubectl -n databases delete statefulset postgresql
kubectl -n databases delete pvc data-postgresql-0
homelabctl deploy apply --stage data
kubectl -n databases wait --for=condition=complete \
  job/postgresql-database-provisioning --timeout=5m
vault write -f database/rotate-role/pocketid
homelabctl deploy apply --stage identity
```

The explicit `rotate-role` is required: the new PostgreSQL role was created
without a password while Vault still holds the password from the deleted
volume. Rotation writes the password to the recreated role, and VSO then
projects it.

Then verify the chain end to end:

1. the provisioning Job completes and `pocketid` exists as both a role and a
   database;
2. Butler has written `database/config/postgresql` and
   `database/static-roles/pocketid`;
3. `vaultdynamicsecret/pocket-id-database` is `Ready` and Pocket ID's rollout
   completes;
4. Pocket ID has migrated an empty identity database;
5. open `https://auth.6940469.xyz` and enroll the owner again by hand; there is
   no unattended first-owner operation;
6. run `homelabctl control bootstrap --confirm` and
   `homelabctl control verify-identity` so Butler recreates the declared OIDC
   clients and rewrites Vault's JWT configuration.

Open `https://auth.6940469.xyz`, create the first human owner and enroll a
passkey. Pocket ID excludes its synthetic static API user from the initial
owner check, so the Butler credential does not consume or bypass this human
ceremony. Pocket ID does not expose an unattended first-owner operation.

Advance bootstrap after owner enrollment:

```bash
homelabctl control bootstrap --confirm
```

Recovery Butler contacts Pocket ID directly during this pass. Its bounded
NetworkPolicy permits only the `pocket-id` pods on TCP 80/1411; a persistent
`connect: connection refused` from the recovery command while Pocket ID is
healthy indicates that the current Butler Helm release predates that egress
rule. Deploy the current `secrets` stage and repeat the idempotent command. Do
not reset Pocket ID or delete its PVC for this failure.

The same pass writes the bootstrap clients only to `oauth/homelabctl`
and `oauth/vault`. A Vault `403 permission denied` for
those exact paths means the deployed Butler image predates the bounded recovery
OAuth policy. Wait for the current `main` image to publish, deploy Butler, and
repeat the idempotent command. Do not grant recovery a wildcard `oauth/*`
policy and do not modify the stored client documents manually.

The `--pocket-id-api-key-file` flag is retained only for break-glass rotation
of the generated machine credential. It replaces `static-api-key` in the same
Vault document without changing `encryption-key`; it is not part of normal
installation.

Butler reconciles nine groups: `butleradmin` and `butlerviewer` for the control
plane (the Butler operator tier is disabled, so `control reconcile` needs
`butleradmin`); `vaultadmin`, `vaultread`, `vaultmeta`, `k8sadmin`, `k8soperator` and
`k8sviewer` for Vault; and `household` for the shared applications. It also
reconciles stable OIDC clients for `homelabctl`, Vault, Grafana, Homepage and
Vaultwarden. The Vault chart declares its own six `VaultOIDCRole` resources
beside the Vault release; Butler converges them into `auth/jwt/role/<group>` and
repairs drift on the normal resync. The `homelabctl` client is restricted to `butleradmin` and
`butlerviewer`; the Vault client is restricted to every platform identity
(`butleradmin`, `butlerviewer` and the six Vault/Kubernetes groups) so anyone
with platform authority can reach the Vault UI. Bootstrap now pauses at
`awaiting-identity-verification`.

> **Migration warning.** Membership of the old `homelab-admin`,
> `homelab-operator` and `homelab-viewer` groups grants nothing after this
> change. An identity must be in `butleradmin` or `butlerviewer` for Butler, and
> in the relevant `vault*`/`k8s*` group for Vault, before authorization works.
> After the new groups exist in Pocket ID and **before** expecting Butler or
> Vault to authorize you, add your own account to those groups in Pocket ID. If
> you skip this step both surfaces deny you, and the failure looks like a broken
> token rather than a missing membership. The first owner is the only account
> that can perform the update, so do it immediately after the groups are
> reconciled.

Complete both real login paths from the operator workstation:

```bash
homelabctl control login
homelabctl control status
homelabctl control verify-identity --confirm
homelabctl control recovery
```

Vault's browser flow and OIDC API use the private
`https://vault.6940469.xyz` Traefik ingress. A plain `404 page not found` while
requesting Vault's OIDC authorization URL means Traefik has no matching Vault
Ingress; it is not evidence that `auth/jwt` or the `vaultadmin` role is
missing. Deploy the current Vault chart and confirm its HTTPS health endpoint
before repeating the identity proof. Do not reset Vault, recreate its PVC, or
reconfigure the auth mount for this routing failure.

Pocket ID's discovery document must advertise the public
`https://auth.6940469.xyz` origin for its authorization, token and JWKS
endpoints. Do not set Pocket ID's `INTERNAL_APP_URL`: it rewrites those
client-facing URLs to Kubernetes-only DNS and breaks `homelabctl` on the Mac.
Butler provider administration continues to use its separately configured
internal Pocket ID API URL.

`verify-identity` first proves the cached Pocket ID session reaches normal
Butler as an administrator. It then opens Vault's Pocket ID OIDC flow against
the `vaultadmin` role on a loopback callback, verifies that the resulting
short-lived token carries `vault-admin`, revokes that token, and submits only
the Pocket ID subject, Butler role and Vault policy names to
the private recovery API. No Vault token enters Butler, a file, or a Kubernetes
Secret. The recovery phase becomes `operational` only after both proofs pass.
Kubernetes authority is deliberately a separate proof: mint a credential with
the group that carries the cluster role you expect.

```bash
homelabctl credentials --role k8sadmin --ttl 15m > /tmp/admin.kubeconfig
KUBECONFIG=/tmp/admin.kubeconfig kubectl get nodes
```

A `403` from Vault naming `kubernetes/creds/k8sadmin` means your Pocket ID
account is not in `k8sadmin`; it is not a Butler or Vault availability
problem.

The Vault CLI needs its own token, which `homelabctl credentials` does not
provide. The auth method is mounted at `auth/jwt`, not at the conventional
`oidc` path, so the mount must be named explicitly:

```bash
export VAULT_ADDR=https://vault.6940469.xyz
vault login -method=oidc -path=jwt role=vaultadmin
```

Omitting `-path` targets a mount called `oidc`, which does not exist, and Vault
answers `403 permission denied` for that unknown path rather than a helpful
error. The role is the Pocket ID group name, exactly as `homelabctl credentials`
takes it. In the Vault UI the same flow is the `JWT` method. Set **Mount path** to
`jwt` explicitly: it defaults to `oidc`, a mount that does not exist here, and
Vault reports that as `Error fetching role: permission denied` rather than a
missing-mount error. The registered redirect URI is
`/ui/vault/auth/jwt/oidc/callback`, so the default path would also fail at
Pocket ID. Select the role that matches a group you belong to.

Once recovery is `operational`, treat the bootstrap ceremony as finished.
The next required work is to export `butler-vault-init` to age-encrypted
off-cluster storage, verify a disposable decrypt, and complete the K3s
recovery export and restore rehearsal before relying on any later platform
stage.

## 5. Shared data and API provisioning

```bash
homelabctl deploy platform --through data --confirm
kubectl -n databases get pods,pvc
kubectl -n storage get pods,pvc
```

Butler projects PostgreSQL and Redis values to consumer-specific Vault paths.
It calls Garage API v2 to assign Titan's layout and create the backup bucket and
key. Garage on Titan remains local object storage, not an off-node backup.

## 6. Observability

```bash
homelabctl deploy platform --through observability --confirm
kubectl -n monitoring get pods,pvc
kubectl -n monitoring logs daemonset/alloy --tail=100
```

Prometheus retains metrics and rules; Alertmanager evaluates delivery; Loki
stores logs; Tempo stores traces; Alloy routes Kubernetes logs and OTLP;
Grafana provisions the Prometheus, Loki and Tempo datasources and Butler
dashboard. Butler exports HTTP and reconciliation metrics/traces and emits
structured logs with trace IDs.

Verify in Grafana:

- normal and recovery Butler replicas equal one;
- reconciliation rate is visible by reconciler and outcome;
- an authenticated API request produces a Tempo trace;
- the trace ID finds its corresponding Loki request log;
- a test alert reaches the configured receiver.

## Stop conditions

Stop rather than applying later stages when:

- recovery status cannot be obtained with a fresh Kubernetes token;
- Vault cannot be unsealed with the encrypted recovery copy;
- the bootstrap phase is not `operational`;
- normal Butler has access to `butler-vault-init`;
- VSO cannot authenticate or the Let's Encrypt `ClusterIssuer` cannot read its
  projected acme-dns account;
- Pocket ID login, Butler audience validation or role mapping fails;
- Alloy, a bounded observability PVC, or alert delivery is unhealthy.

Only then continue with `homelabctl deploy platform --through cicd --confirm`
and finally `--through applications`. Helmfile remains idempotent, so earlier
stages are intentionally rechecked on each ordered run.
