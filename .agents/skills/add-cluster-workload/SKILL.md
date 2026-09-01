---
name: add-cluster-workload
description: Integrate, replace, or materially change a Kubernetes workload in this homelab repository. Use for application, service, database, storage, monitoring, CI/CD, networking, security, or smart-home Helm chart work that must connect to Helmfile ordering, declarative namespaces and RBAC, Vault/Butler credentials, NetworkPolicies, persistence, images, documentation, and repository validation. Do not use for routine live deployment of an already-defined release or for an isolated template typo with no platform impact.
---

# Add Cluster Workload

Build the workload as part of the platform contract, not as a standalone set of Kubernetes manifests. Preserve dependency ordering, least privilege, recovery expectations, and the distinction between repository intent and verified runtime state.

## Establish the contract

1. Read `AGENTS.md`, `docs/engineering/cluster-platform.md`, and the relevant section of `docs/project/current-state.md`.
2. Inspect the closest existing chart in the same `cluster/<domain>/` directory. Reuse its naming, labels, helpers, probes, resources, security context, and template layout only where the new workload has the same requirements.
3. Determine before editing:
   - owning domain, namespace, Helmfile stage, criticality, and dependencies;
   - image source, immutable version or digest, and supported architectures;
   - service ports, ingress/authentication decision, and network peers;
   - configuration versus secret inputs;
   - persistence, backup, restore, and upgrade behavior;
   - Butler capabilities required: `ManagedCredential`, `PocketIDClient`, or `GarageBucket`.
4. Surface missing product decisions instead of inventing credentials, public exposure, broad RBAC, or backup guarantees.

## Implement the desired state

1. Create or update the chart under the correct `cluster/<domain>/<release>/` path.
2. Declare third-party chart dependencies in `Chart.yaml`, regenerate `Chart.lock`, and commit both. Keep repository and application versions pinned deliberately; preserve digest pins where the repo uses them.
3. Add the release to `cluster/helmfile.yaml.gotmpl` with:
   - an exact release name and namespace;
   - `historyMax` consistent with neighboring releases;
   - the smallest complete `needs` set;
   - `stage`, `domain`, and `criticality` labels;
   - an explicit `installed` switch for opt-in capabilities.
4. Add a new namespace only in `cluster/core/namespaces/values.yaml`. Select Pod Security labels from the workload's real requirements; do not weaken an existing namespace for one privileged pod.
5. Add shared service accounts, roles, and bindings through `cluster/core/rbac-policies/values.yaml`. Grant only required resources and verbs. Never default a workload or runner to `cluster-admin`.
6. Keep non-secret settings in values or ConfigMaps. Never put secret values in Helm values, ConfigMaps, custom-resource status, or command-line extra vars.
7. Declare provider-owned credentials beside the consuming chart through Butler custom resources. Use globally unique Vault paths and provider names. Project only the consumer-specific fields with `VaultStaticSecret`; Vault remains the source of truth.
8. Add default-deny-compatible NetworkPolicies with explicit ingress, DNS, API, database, cache, object-storage, and external HTTPS paths. A NetworkPolicy documents intent only when the installed CNI enforces it.
9. Keep application Services as `ClusterIP`. Add ingress only when TLS, Pocket ID or another approved auth path, callbacks, logout, and exposure policy are defined.
10. For stateful workloads, define storage, disruption behavior, resource limits, backup ownership, restore acceptance, and upgrade sequencing. Do not describe a PVC or Garage data on Titan as an off-node backup.

## Preserve platform boundaries

- Put concrete `ManagedCredential`, `PocketIDClient`, and `GarageBucket` declarations in the owning chart, never Butler's ConfigMap.
- Do not rely on PostgreSQL init scripts to add users or databases after the initial volume bootstrap.
- Treat Helmfile `needs` as submission ordering, not readiness proof. Document the API, Secret, rollout, or human checkpoint that gates dependents.
- Keep smart-home increments disabled until their hardware, security, and backup acceptance checks pass.
- Do not hand-apply namespaces, CRDs, or chart resources with `kubectl` as part of implementation.

## Validate and document

1. Render and lint with the repository-supported `homelabctl` workflows. Start focused, then run `homelabctl ci check` before handoff when practical.
2. Run `homelabctl deploy diff <release>` or `homelabctl deploy diff --stage <stage>` only when cluster access is available. Stop on unexpected deletion, namespace movement, immutable-field replacement, or persistent-state impact.
3. Verify that every referenced Secret has a declared producer and every `needs` edge uses the exact Helmfile release identifier.
4. Update the relevant engineering or operations guide and `docs/project/current-state.md` when the workload changes the documented platform.
5. Mark undeployed work as `Ready in repo` or `Ready for testing`; use `Deployed` or `Verified` only after live checks on Titan.
6. Do not apply the release unless the user explicitly asks for a live deployment. If asked, also use `$operate-homelab-safely`.
