# Bootstrap the cluster platform

Use this runbook only after Titan is healthy, kubeconfig points at Titan, and
the first K3s recovery export exists off-node. The first deployment runs from
the Mac; the scale-to-zero runner cannot deploy the platform that creates it.

## Before the first apply

Resolve `*.home.6940469.xyz` to Titan's reserved LAN address and confirm there
is no router port-forward. Publish the Butler image for the current commit.
Deploy commands default to the full Git SHA, so that immutable image must exist.

```bash
homelabctl cluster status
homelabctl deploy diff
homelabctl deploy platform --through identity --confirm
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
kubectl get namespaces
kubectl -n kube-system rollout status deployment/metrics-server
kubectl get apiservice v1beta1.metrics.k8s.io
kubectl top node titan
```

This creates platform namespaces, per-application namespaces, Pod Security
labels and foundational RBAC, then installs the separately pinned metrics-server
release that replaces the disabled K3s package. Do not continue until its
deployment is available, the aggregated Metrics API reports `Available=True`
and `kubectl top` returns Titan CPU and memory usage.

## 2. Networking prerequisites

```bash
homelabctl deploy apply --stage networking
kubectl -n networking rollout status deployment/traefik
kubectl -n cert-manager get pods
```

The Vault-backed `ClusterIssuer` remains `NotReady` until Vault PKI exists.

## 3. Vault, recovery Butler and VSO

```bash
homelabctl deploy apply --stage secrets
kubectl -n security rollout status deployment/butler-recovery
homelabctl control recovery
```

Normal Butler cannot initialize Vault and cannot read the recovery Secret.
Advance the no-Ingress recovery service explicitly:

```bash
homelabctl control bootstrap --confirm
homelabctl control recovery
```

The phase progresses through `initialize-vault`, `unseal-vault` and
`configure-vault`, then pauses at `awaiting-pocket-id-api-key`. The operation
is idempotent and refuses an already initialized Vault if `butler-vault-init`
is missing. Successful Vault foundation creates the bounded normal and
recovery Kubernetes-auth roles; it does not claim that identity works yet.

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
kubectl -n cert-manager describe clusterissuer vault
```

The current certificate path is Vault private PKI. Export the public CA chain
through the already-authenticated Kubernetes connection before opening HTTPS
services:

```bash
homelabctl trust export \
  --output /secure/homelab-recovery/homelab-ca.pem
openssl x509 -in /secure/homelab-recovery/homelab-ca.pem \
  -noout -subject -issuer -fingerprint -sha256
```

The CLI validates every certificate, refuses to overwrite a file, and prints
each SHA-256 fingerprint. Independently compare the fingerprint before
installing the root CA on LAN clients. Public Namecheap DNS-01
certificates are separate future networking work.

## 4. Pocket ID and management handoff

```bash
homelabctl deploy apply --stage identity
kubectl -n security rollout status deployment/pocket-id
```

Open `https://auth.home.6940469.xyz`, create the first owner, enroll a passkey,
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
stable OIDC clients for Butler, Vault, Grafana and Vaultwarden. Assign the first
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
- VSO or the Vault `ClusterIssuer` cannot authenticate;
- Pocket ID login, Butler audience validation or role mapping fails;
- Alloy, a bounded observability PVC, or alert delivery is unhealthy.

Only then continue with `homelabctl deploy platform --through cicd --confirm`
and finally `--through applications`. Helmfile remains idempotent, so earlier
stages are intentionally rechecked on each ordered run.
