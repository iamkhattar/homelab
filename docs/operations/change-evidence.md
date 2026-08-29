# Record changes and verification evidence

Use this procedure after installing, upgrading, restoring or materially
changing Titan. It prevents repository intent from being mistaken for a
deployment result and leaves enough context for the next operator to understand
what was proved.

## Keep two records

The committed and private records have different purposes:

| Record | Location | Content |
| --- | --- | --- |
| Deployment state | [Current state](/project/current-state) in Git | Milestone, status, repository revision, broad result and remaining action; no credentials or machine-private values. |
| Recovery ledger | Encrypted storage outside Titan and the repository | Exact recovery export location, checksums, recipients, restore rehearsal details, host fingerprints and other sensitive operational evidence. |

Do not create a `records/` directory in this repository for raw command output.
Logs can contain tokens, private addresses, certificate details, resource
identifiers and environment values even when the command normally redacts
secrets.

## Before a change

Record the intended scope, expected interruption and rollback point. Then run:

```bash
homelabctl version
homelabctl doctor --strict
homelabctl inventory check
homelabctl cluster status
```

During the first build, run `cluster status` only after K3s installation and
record its earlier absence as expected. `homelabctl version` records the
binary's embedded build commit; separately record the repository revision that
the runbook is using when the binary was obtained from a release.

For a change that can affect cluster state, create and export recovery material
using the relevant [backup and recovery](/operations/backup-recovery) workflow.
Record only the encrypted copy's reference in the private recovery ledger.

Stop when the current health state is unexplained, the repository revision is
not the one intended for deployment, or the rollback material cannot be opened
by an authorised operator.

## Apply one bounded change

Follow the relevant runbook and retain:

- start and finish time in UTC;
- the `homelabctl` command and non-secret flags used;
- the repository revision and released artifact version;
- expected and actual interruption;
- warnings, deviations and manual actions;
- the rollback point selected before the change.

Do not paste kubeconfig content, Kubernetes bearer tokens, Vault tokens or
recovery material, Pocket ID sessions or API keys, SSH private keys, database
credentials, age identities, or unredacted Secret resources into either Git or
an issue tracker.

## Verify the outcome

Run the checks required by the component-specific runbook. At minimum:

```bash
homelabctl inventory check
homelabctl cluster status
```

For platform changes, also verify the affected stage rather than treating a
successful Helm apply as service health. Confirm rollout state, expected HTTPS
access, authentication, telemetry, alert delivery and restore behaviour where
the change touches them.

## Update repository state

Update [Current state](/project/current-state) only after verification against
Titan. Use one of these descriptions consistently:

| Status | Meaning |
| --- | --- |
| Ready in repo | Desired state and workstation validation exist; Titan has not proved it. |
| Ready for testing | The implementation can be deployed, but runtime or recovery behaviour is still unverified. |
| Deployed | The intended revision is running on Titan and basic health checks passed. |
| Verified | Health, access, observability and the relevant recovery path were exercised. |
| Blocked | A named prerequisite or failure prevents safe progress. |

Keep the entry short and link to the owning runbook. Put exact sensitive facts
in the encrypted recovery ledger, not in the status table.

## Private recovery-ledger template

Store the completed version encrypted outside Titan and the repository:

```text
Change:
UTC start / finish:
Operator:
Repository revision:
homelabctl version:
Titan / K3s version:
Scope:
Expected interruption:
Pre-change health:
Recovery export reference:
Recovery export checksum verified by:
Commands and non-secret flags:
Manual actions or deviations:
Post-change health:
Authentication checks:
Observability and alert checks:
Restore or rollback check:
Outcome:
Follow-up:
```

The record is complete only when another authorised operator—or the same
operator from a clean workstation—can locate and decrypt the referenced
recovery material.
