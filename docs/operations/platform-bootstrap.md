# Bootstrap the cluster platform

Use this runbook only after Titan is healthy, kubeconfig points at Titan, and
the first K3s recovery export exists off-node. The first deployment runs from
the Mac; the scale-to-zero runner cannot deploy the platform that creates it.

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

The next action is **3. Vault, recovery Butler and VSO**. Do not alter or delete
the pending issuer, CertificateRequest or temporary private-key Secret.

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

Continue only when `certificateReady` is true and the bootstrap phase is
`awaiting-pocket-id-api-key`. cert-manager renews with the same account and
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
kubectl -n security rollout status deployment/pocket-id
```

Open `https://auth.6940469.xyz`, create the first owner, enroll a passkey,
and create one management API key. Pocket ID does not expose an unattended
first-owner operation.

Place the one-time key in a private staging file, import it directly into Vault
through recovery Butler, then remove the file:

```bash
chmod 600 /secure/pocket-id-api-key
homelabctl control bootstrap --confirm \
  --pocket-id-api-key-file /secure/pocket-id-api-key
rm /secure/pocket-id-api-key
```

Butler reconciles `homelab-admin`, `homelab-operator` and `homelab-viewer`, plus
stable OIDC clients for Butler, Vault, Grafana, Homepage and Vaultwarden. Assign the first
owner to `homelab-admin`. Bootstrap now pauses at
`awaiting-identity-verification`.

Complete both real login paths from the operator workstation:

```bash
homelabctl control login
homelabctl control status
homelabctl control verify-identity --confirm
homelabctl control recovery
```

`verify-identity` first proves the cached Pocket ID session reaches normal
Butler as an administrator. It then opens Vault's Pocket ID OIDC flow on a
loopback callback, verifies that the resulting short-lived token has exactly
the required `vault-admin` and `k8s-admin` capabilities, revokes that token,
and submits only the Pocket ID subject, Butler role and Vault policy names to
the private recovery API. No Vault token enters Butler, a file, or a Kubernetes
Secret. The recovery phase becomes `operational` only after both proofs pass.

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
