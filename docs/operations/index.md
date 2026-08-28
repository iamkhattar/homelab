# Operator runbooks

Use this section when changing or diagnosing a running Titan. Every supported
procedure starts from the repository root, uses `homelabctl`, states its safety
boundary and ends with verification.

## Select a runbook

| Situation | Runbook |
| --- | --- |
| K3s has not been installed yet | [Install the K3s cluster](/operations/install) |
| K3s is healthy and the platform is not installed | [Bootstrap the cluster platform](/operations/platform-bootstrap) |
| Apply Debian updates, reboot or upgrade K3s | [Maintenance and upgrades](/operations/maintenance) |
| Create, export or reason about recovery material | [Backup and recovery](/operations/backup-recovery) |
| Titan, SSH, K3s or workloads are unhealthy | [Troubleshooting](/operations/troubleshooting) |

## Standard operating loop

1. Read the current state and the complete relevant runbook.
2. Confirm the inventory and workstation toolchain.
3. Capture health evidence before making a change.
4. Create or verify recovery material when the change can affect K3s state.
5. Preview where the underlying tool supports a meaningful preview.
6. Apply one bounded change.
7. Verify node, cluster and workload health.
8. Update the current-state page when deployment reality changed.

::: warning Single-node maintenance
Titan has no failover node. Reboots, K3s restarts and some storage changes cause
an outage. Schedule them accordingly and prove recovery material first.
:::

For command syntax without operational context, use the
[homelabctl command reference](/homelabctl/command-reference).
