# Network policy design

This page defines the network boundary the cluster intends to enforce, the
state of that boundary today, and the order in which it can safely be closed.

It is a design record. It changes no chart and applies nothing.

## How enforcement works in this cluster

K3s includes an embedded network-policy controller: kube-router's netpol
implementation in policy-only mode, running beside flannel. It is enabled by
default and is disabled only when every server starts with
`--disable-network-policy`, which this repository does not do. The controller
is per-node, so enforcement stays correct as the cluster grows to multiple
nodes. The [decision log](/project/decisions) records why no separate
policy-capable CNI is installed.

Three properties follow, and they drive every rule below.

1. **NetworkPolicy is allow-list only.** A pod selected by no policy is
   unrestricted. A pod selected by a policy may only reach what that policy
   admits.
2. **A namespace needs a namespace-wide baseline to fail closed.** A policy with
   `podSelector: {}` selects every pod in the namespace and denies everything
   it does not explicitly allow. Without one, a newly created pod in that
   namespace is open until somebody writes its policy.
3. **The absence of a policy is a silent hole.** A chart that declares no
   policy produces no error, no warning, and no missing object. Only a
   namespace-wide baseline makes the failure mode visible.

## Current state

Produced by rendering the whole Helmfile offline and listing the resulting
NetworkPolicy objects:

```bash
HOMELAB_IMAGE_TAG=$(git rev-parse HEAD) \
  helmfile -f cluster/helmfile.yaml.gotmpl template --skip-deps
```

| Namespace | Workloads | Policies | Namespace-wide baseline |
| --- | --- | --- | --- |
| `cert-manager` | cert-manager, Vault Secrets Operator | none | none |
| `networking` | Traefik, public certificates | none | none |
| `security` | Vault, Butler, Butler recovery, Pocket ID | Butler, Butler recovery, Pocket ID | none |
| `databases` | PostgreSQL, Redis | PostgreSQL, Redis | none |
| `storage` | Garage | Garage | none |
| `monitoring` | Prometheus, Grafana, Loki, Tempo, Alloy | monitoring-boundary | **yes** |
| `cicd` | ARC controller, runner scale set | runner scale set | none |
| `homepage` | Homepage | Homepage | none |
| `kitchenowl` | KitchenOwl | KitchenOwl | none |
| `ntfy` | ntfy | ntfy | none |
| `paperless-ngx` | Paperless-ngx | Paperless-ngx | none |
| `vaultwarden` | Vaultwarden | Vaultwarden | none |
| `smart-home` | Home Assistant, Mosquitto | Home Assistant, Mosquitto | none (opt-in releases) |
| `zigbee` | Zigbee2MQTT | Zigbee2MQTT | none (opt-in release) |

Three findings that are not visible from the chart directories:

- **Vault has no policy at all.** It is the cluster's secret store, and its
  listener runs with `tls_disable = 1`, so the traffic across it is plain HTTP
  inside the cluster. The network boundary is the only control standing between
  one pod and every application credential.
- **Traefik has no policy.** It is the public ingress point.
- **The Actions Runner Controller has no policy.** Only the runner scale set is
  covered.

Workload-scoped policies do exist where they matter most, and their intent is
already recorded in chart values rather than in prose. The PostgreSQL chart
already declares its consumer list and DNS egress, and the Redis chart declares
its consumer list.

## Rules for authoring a policy

1. **Every namespace gets a baseline.** `podSelector: {}`, both policy types,
   empty ingress and egress rules. It denies everything and is the only thing
   that makes a missing allow policy visible.
2. **A baseline and the allow policies it depends on land together.** A
   namespace whose pods have no allow policy is denied DNS and API access the
   moment its baseline exists. This is the main way a policy change causes an
   outage.
3. **DNS is explicit.** Every workload needs egress to kube-dns on UDP and TCP
   53. Namespace-level DNS access is not automatic.
4. **Kubernetes API access is explicit.** It is allowed by node or service CIDR,
   not by namespace selector, because namespace selectors do not match the
   virtual API service IP.
5. **Peers are named by namespace label.** Use
   `kubernetes.io/metadata.name`, which Kubernetes maintains automatically, and
   keep the allow list identical to the credential consumer list in the owning
   chart.
6. **A workload's policy selects that workload's pods.** The baseline handles
   the namespace. Two layers keep each one readable.
7. **External egress is denied unless a flow requires it.** Only cert-manager
   needs internet egress, for the ACME directory and the acme-dns API.

## Per-namespace design

Required flows, to be reviewed before any of this is written as a chart.

| Namespace | Ingress admitted | Egress admitted | Baseline first? |
| --- | --- | --- | --- |
| `cert-manager` | webhook callers from `monitoring` and `security` | DNS, K8s API, ACME directory and `auth.acme-dns.io` on 443 | with its allow policies |
| `networking` | Traefik from the home LAN range and from `monitoring` | DNS, K8s API | with its allow policies |
| `security` | Vault from Butler, Butler recovery and VSO; Pocket ID from Traefik-proxied apps and Vault | DNS, K8s API, Pocket ID | after a Vault policy exists |
| `databases` | PostgreSQL from `kitchenowl`, `paperless-ngx`, `vaultwarden`; Redis from `paperless-ngx` | DNS | yes, existing allows cover the pods |
| `storage` | Garage from its declared consumers | DNS | yes |
| `monitoring` | existing boundary | existing boundary | already present |
| `cicd` | ARC controller API from the cluster; runner pods have no inbound path | DNS, K8s API | after a controller policy exists |
| application namespaces | existing workload policies | DNS | yes |

## Staging order

Each step is one reviewable change, applied and verified on its own. The order
is chosen so that no namespace is left without a working path.

1. `security` — write the Vault policy first, then the baseline.
2. `databases` — PostgreSQL and Redis already have allow policies, so the
   baseline is additive. This is the namespace that matters next.
3. `networking` — Traefik policy, then the baseline.
4. `cert-manager` — cert-manager and VSO policies, then the baseline.
5. `cicd` — controller policy, then the baseline.
6. Application namespaces — baselines only.
7. `smart-home` and `zigbee` — baselines when those opt-in releases are
   enabled.

## Verification on Titan

Enforcement is proven, never assumed.

```bash
homelabctl cluster status
sudo iptables-save | grep KUBE-ROUTER
```

The first proves the cluster is healthy. The second must show programmed
controller rules; an empty result means no policy is being enforced on this
node. Then, for one namespace at a time:

- a disposable pod proves an admitted path still works;
- a disposable pod in another namespace proves a denied path is refused;
- the failure is recorded in [current state](/project/current-state) rather
  than assumed.

## Tooling this design needs

The repository has no offline Helm validation today. `homelabctl ci check`
covers Go, documentation, workflows, Ansible and Terraform, and
`homelabctl deploy diff` needs a live cluster, so chart rendering only happens
when an operator is on the LAN with a cluster context. Two additions would
close that:

- **`homelabctl ci check --only charts`.** Render every chart offline and fail
  on a template error. This is the check that would have caught a malformed
  policy or a missing value without waiting for a deploy.
- **A namespace invariant check.** Assert that every namespace in
  `cluster/core/namespaces/values.yaml` has a namespace-wide baseline in the
  rendered output, and that every `namespaceSelector` refers to a namespace
  that exists. This turns the failure mode in this document into a CI failure
  instead of a review habit.

A `homelabctl deploy render` command is a smaller convenience than either: it
is the offline Helmfile render this page uses to produce its inventory table.

## Open questions

- Should the `smart-home` and `zigbee` namespaces get baselines at the same
  time as their opt-in releases, or earlier, so the namespace is never
  briefly unguarded?
- Is a single baseline per namespace enough, or should each namespace also
  carry a namespace-wide egress allow for DNS, leaving the baseline to deny
  everything else?
- The home LAN range is repeated in several charts. Should it become one
  reviewed value in `cluster/helmfile-values.yaml` instead of being copied?
