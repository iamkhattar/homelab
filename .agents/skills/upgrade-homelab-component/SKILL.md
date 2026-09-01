---
name: upgrade-homelab-component
description: Research, implement, review, or validate an upgrade to a pinned homelab component, including K3s, Ansible/Python dependencies, Helm charts, CRDs, container images or digests, Go modules, VitePress packages, databases, observability tools, Butler, homelabctl, and applications. Use when changing versions, lockfiles, image tags, digests, APIs, schemas, or migration-sensitive configuration. Do not use for adding a brand-new workload or merely deploying an already-reviewed repository revision.
---

# Upgrade Homelab Component

Treat an upgrade as a compatibility and recovery change, not a version-string edit. Keep pins immutable, separate repository readiness from live verification, and preserve a rollback artifact until the upgraded system proves healthy.

## Define the upgrade envelope

1. Read `AGENTS.md`, `docs/engineering/cluster-platform.md`, `docs/project/current-state.md`, and the component's owning runbook.
2. Locate every active version surface with `rg`: source pin, chart dependency, `Chart.lock`, image tag and digest, generated manifests, build metadata, tests, docs, examples, and private inventory instructions.
3. Record the current and target versions, why the target is selected, supported architectures, upstream support window, and whether the change crosses a major or Kubernetes minor version.
4. Read authoritative release notes and migration guidance for every version crossed. Identify:
   - breaking configuration or CLI changes;
   - removed APIs and CRD/schema changes;
   - storage or database migrations and downgrade limits;
   - required intermediate versions;
   - Kubernetes, Go, Node, Python, chart, or controller compatibility;
   - changed ports, permissions, security contexts, probes, or resource use.
5. Stop when the exact artifact, digest, migration path, or rollback semantics cannot be verified. Do not substitute `latest`, a floating branch, or an unverified digest.

## Plan protection and rollback

- Classify the component as stateless, control-plane, credential-bearing, or stateful.
- For K3s or recovery-sensitive platform work, require a fresh encrypted off-device recovery export before live mutation.
- For stateful services, require component-specific backup and restore instructions; an etcd snapshot does not protect every PVC, database, or object store.
- Determine whether rollback means reverting configuration, restoring data, redeploying an older immutable image, or rebuilding the cluster. If an upstream migration is irreversible, say so before implementation.
- Preserve the old version, image digest, values, schema, repository revision, and recovery point for the observation window.

## Update the right surfaces

### Helm charts and CRDs

1. Update the dependency version in `Chart.yaml` and regenerate `Chart.lock`; never hand-edit the lock digest.
2. Review upstream default-value changes rather than blindly retaining or replacing current values.
3. Render all templates and inspect changes to RBAC, CRDs, webhooks, Services, storage, selectors, Pod Security, NetworkPolicies, and immutable fields.
4. Split CRD/controller or policy resources into dependency-ordered releases when a fresh API server cannot validate new kinds in the same transaction.
5. Keep Helmfile `needs`, stage labels, and readiness checkpoints aligned with any new dependency.

### Container images

1. Pin a supported semantic version and immutable multi-architecture digest where the repository uses digest locking.
2. Verify the digest belongs to the intended tag and includes every required node architecture.
3. Review user IDs, filesystem paths, entrypoints, health endpoints, exposed ports, and migration behavior.
4. Keep the old digest available until rollout and restore checks pass.

### K3s and Ansible

1. Update the exact `k3s_version` in both private and example inventories only after reviewing K3s release notes and version-skew constraints.
2. Keep Ansible Python and collection dependencies exact. Pin `k3s.orchestration` to an audited commit, not a movable tag or branch.
3. Review upstream collection changes before advancing the commit, especially defaults, variable names, destructive playbooks, token handling, server ordering, and reboot/upgrade behavior.
4. Use `$change-node-automation` when local roles, playbooks, inventory structure, or homelabctl lifecycle behavior also change.

### Databases and applications

1. Identify schema migration direction, backup format compatibility, and whether downgrades are supported.
2. Do not rely on a changed PostgreSQL initialization script to mutate an existing volume.
3. Quiesce writes when upstream requires a consistent backup or offline migration.
4. Test application authentication, data access, background jobs, uploads, and integration clients after migration.

### Toolchains and Go modules

1. Keep runtime, builder image, module, workflow, and documentation pins consistent where they represent the same toolchain.
2. Review transitive changes and generated files. Do not bundle unrelated upgrades merely because a lockfile can refresh them.
3. Preserve the bounded toolset and unprivileged runtime contracts of published images.

## Validate before deployment

1. Run the narrowest relevant checks while iterating, then `homelabctl ci check` before handoff when practical.
2. For Helm changes, run a complete render/lint and a focused `homelabctl deploy diff <release>` or `--stage <stage>` when cluster access is available.
3. Stop on unexpected deletion, PVC or namespace replacement, CRD incompatibility, privilege expansion, missing Secret producers, mutable images, or an unpublished image tag.
4. Add or update tests for config migration, API compatibility, rendered resources, and failure/rollback paths.
5. Update the owning engineering and operations docs with prerequisites, backup, exact sequence, stop conditions, verification, and rollback limits.
6. Record repository-only work as `Ready in repo` or `Ready for testing`. Never claim `Deployed` or `Verified` from a successful build or render.

## Deploy only with explicit live intent

Do not apply, upgrade, restart, migrate, or rotate anything merely because the repository change is ready. When the user requests the live upgrade, also use `$operate-homelab-safely`, create the required recovery point, apply one bounded component change, run component-specific acceptance checks, and retain the rollback point through the observation window.
