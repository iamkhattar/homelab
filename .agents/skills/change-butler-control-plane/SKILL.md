---
name: change-butler-control-plane
description: Design, implement, review, or test changes to the Butler Go control plane, its normal and recovery HTTP runtimes, Kubernetes platform APIs, Vault/Pocket ID/Garage adapters, reconciliation, operations journal, policies, Helm chart, or homelabctl control commands. Use whenever work touches `butler/`, `cluster/services/butler`, `cluster/core/butler-crds`, Butler-owned platform resources, or the control-plane API contract. Do not use for ordinary application chart changes that merely consume an existing Butler custom resource.
---

# Change Butler Control Plane

Preserve Butler's recovery boundary, provider-safe reconciliation, secret-free APIs, and explicit normal-versus-recovery identities while evolving the service.

## Build context before editing

1. Read `AGENTS.md` and `docs/engineering/butler-control-plane.md` completely.
2. Trace the requested behavior through the relevant layers:
   - `internal/server` for HTTP transport and middleware;
   - `internal/access` for principals and authorization;
   - `internal/identity`, `recovery`, or `reconciler` for domain behavior;
   - `internal/platform` for Kubernetes resource decoding and status;
   - `internal/pocketid`, `vault`, or `garage` for provider calls;
   - `internal/operations` for durable, sanitized operation state;
   - `homelabctl/internal/cli` for operator-facing commands.
3. Classify the change as normal-runtime, recovery-runtime, or shared. Inspect both runtime wiring paths when shared code changes authentication, configuration, Kubernetes access, or provider credentials.

## Preserve non-negotiable boundaries

- Keep normal Butler Pocket ID-authenticated and recovery Butler Kubernetes TokenReview-authenticated.
- Never let normal Butler mount, read, or gain RBAC access to `butler-vault-init`.
- Keep recovery ClusterIP-only and accept only the exact audience-bound recovery-client service-account identity.
- Keep generated secrets, provider API keys, Vault tokens, unseal material, request bodies, and raw provider responses out of logs, errors, CRDs, statuses, operation records, and API responses.
- Let transport validate and authorize, then call domain logic. Do not embed provider credentials or reconciliation algorithms in handlers.
- Serialize provider operations that return one-time secrets or are unsafe to race.
- Persist a replacement provider credential before revoking the previous one; revoke a newly created credential if Vault persistence fails.
- Refetch Kubernetes objects before status or journal writes and retain optimistic-lock conflict handling.
- Keep deletion retain-by-default for `v1alpha1` provider resources unless an explicit, reviewed destructive API changes that contract.
- Keep bootstrap steps resumable and idempotent. Never bypass the human identity-verification gate or continue an initialized Vault when required recovery material is absent.

## Implement by contract

1. Define input validation, authorization role, idempotency, failure behavior, audit-safe output, and recovery behavior before changing code.
2. Extend the smallest responsible domain interface. Keep provider-specific request/response details inside adapters.
3. For an HTTP API change, keep it under `/api/v1`, update middleware and role checks, add handler and domain tests, and update `homelabctl` when it is the supported operator surface.
4. For a custom-resource change:
   - update the typed platform model and validation;
   - update `cluster/core/butler-crds` schemas;
   - preserve status as observed generation, provider identifier, conditions, and sanitized reasons only;
   - update owning chart templates and reconciliation tests;
   - consider compatibility with already persisted `v1alpha1` objects.
5. For configuration or policy changes, update defaults, validation, chart values/templates, Vault policies, RBAC, and both runtime Deployments as applicable. Do not silently widen network or Kubernetes permissions.
6. For a new reconciliation capability, define uniqueness and ownership across all secret-producing resource kinds before writing provider state.
7. Update `docs/engineering/butler-control-plane.md`, command documentation, and any bootstrap/recovery runbook whose operator contract changes.

## Test the safety properties

1. Add focused unit tests in every changed package, including denial and partial-failure paths.
2. Prefer fakes that prove call order for create, persist, revoke, status, and retry behavior.
3. Test both positive and negative authorization cases for normal and recovery surfaces.
4. Test that errors and persisted operation/status objects exclude secret material.
5. Run focused Go tests while iterating, then `homelabctl ci check --only go-format,go-test`.
6. Run broader `homelabctl ci check` when Helm, CRD, documentation, workflow, or policy files change.
7. Build the Butler image with `homelabctl build services butler --tag dev` when the image contract changes and Docker is available.
8. Do not bootstrap, reconcile, rotate credentials, or deploy Butler to Titan unless the user explicitly requests the live operation. For live work, also use `$operate-homelab-safely`.
