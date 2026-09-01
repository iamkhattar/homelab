---
name: operate-homelab-safely
description: Plan, preview, execute, verify, or record a live homelab operation against local or Hetzner nodes, K3s, Helmfile releases, Butler, Vault, Pocket ID, or recovery material. Use for bootstrap, deployment, upgrade, reboot, restore planning, incident diagnosis, credential ceremony, backup/export, or any request that may mutate Titan or external infrastructure. Do not use for repository-only implementation that will not contact live systems.
---

# Operate Homelab Safely

Use the repository's runbooks and `homelabctl` as the operator interface. Bound each change, establish a recovery point appropriate to its risk, verify actual service behavior, and keep sensitive evidence outside Git.

## Select the runbook and authority

1. Read `AGENTS.md`, `docs/project/current-state.md`, and the operation-specific guide before running commands:
   - node/K3s lifecycle: `docs/homelabctl/cluster-lifecycle.md`;
   - platform deployment/bootstrap: `docs/operations/platform-bootstrap.md` and `docs/homelabctl/deploy-build-ci.md`;
   - backup or restore: `docs/operations/backup-recovery.md`;
   - Butler/Vault/Pocket ID: `docs/homelabctl/control-plane.md` and `docs/engineering/butler-control-plane.md`;
   - troubleshooting: `docs/operations/troubleshooting.md`;
   - evidence: `docs/operations/change-evidence.md`.
2. Distinguish read-only inspection, repository planning, reversible mutation, and destructive recovery. A request to diagnose or plan does not authorize applying, rebooting, rotating, deleting, restoring, or changing provider state.
3. State the target context, node or release, intended revision, expected interruption, success checks, and rollback point. Stop if the target is ambiguous or the current-state runbook names an unmet gate.

## Preflight every mutation

Run from the repository root as applicable:

```text
homelabctl version
homelabctl doctor --strict
homelabctl inventory check
homelabctl cluster status
```

Confirm that the binary version, repository revision, kube context, private inventory, and artifact image tag are the intended ones. Explain any unhealthy state before changing it.

Never expose kubeconfig, K3s tokens, Vault material, Pocket ID keys, SSH keys, age identities, Secret data, or unredacted diagnostic logs. Do not place raw command output in the repository.

## Match protection to the change

- Before K3s upgrade or recovery-sensitive maintenance, create a fresh `homelabctl cluster recovery export`, encrypt and move it off the node and workstation, and verify that an authorized operator can locate the matching token and snapshot.
- Before a cluster reboot, create a named embedded-etcd snapshot.
- Before a stateful application upgrade, use that application's backup and restore procedure; an etcd snapshot does not back up every PVC or external data store.
- Keep the pre-change recovery point until workloads, persistent data, authentication, and observability have passed verification.
- Never improvise a destructive K3s restore against the only data copy. Preserve the failed data directory and follow documentation matching the pinned K3s version.

## Execute one bounded operation

### Helmfile change

1. Run `homelabctl deploy diff <release>` or `homelabctl deploy diff --stage <stage>`.
2. Stop on unexpected deletion, persistent-state replacement, namespace movement, or an image tag that has not been published.
3. Prefer the dependency-ordered `homelabctl deploy platform --through <stage> --confirm` for platform bootstrap or upgrade. Respect every readiness and human checkpoint.
4. Use `deploy apply` for a bounded routine change. Use `deploy sync` only for intentional full reconciliation, never as the first-bootstrap shortcut.
5. Remember that `needs` orders release submission but does not prove an API, Secret, certificate, or provider is ready.

### K3s lifecycle

- Upgrade only after reviewing release notes, updating the exact pinned version in both private and example inventories, exporting recovery material, and passing `homelabctl ci check --only ansible`.
- Reboot with `homelabctl cluster reboot` after a named snapshot; do not substitute raw SSH reboots for the health-gated playbook.
- Use `homelabctl cluster diagnose` for the fixed read-only evidence set when kubectl health is unclear.

### Butler and recovery

- Use `homelabctl control recovery` when normal Pocket ID authentication is unavailable; do not widen normal Butler's recovery access.
- Pass one-time API keys by private file where the CLI requires it, never as a command-line value.
- Export `butler-vault-init` only through the age-encrypted `homelabctl control recovery export` flow and only to a new path outside the repository.
- Treat credential rotation, bootstrap advancement, identity verification, and provider reconciliation as mutations requiring explicit user intent.

### Terraform

Use only `homelabctl infra fmt`, `validate`, and `plan`. Do not add or run Terraform apply/destroy while the legacy Hetzner token and cloud-init boundary remains unresolved.

## Verify and record the outcome

1. Re-run `homelabctl inventory check` and `homelabctl cluster status` after a cluster-affecting change.
2. Verify the component-specific outcome: rollout, conditions, HTTPS/TLS, authentication, telemetry, alerts, data integrity, and restore behavior where relevant. A successful command exit is not service verification.
3. Record UTC timing, revision, non-secret command flags, expected and actual interruption, deviations, rollback point, and results in the private encrypted recovery ledger.
4. Update `docs/project/current-state.md` only with non-sensitive, verified facts. Use `Ready in repo`, `Ready for testing`, `Deployed`, and `Verified` according to `docs/operations/change-evidence.md`.
5. Report clearly what was changed, what was verified, which checks remain, and where the recovery point is referenced without revealing its contents or private location details.
