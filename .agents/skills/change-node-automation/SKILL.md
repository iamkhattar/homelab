---
name: change-node-automation
description: Design, implement, review, or test changes to the repository's Ansible node preparation, Debian hardening, inventory model, pinned K3s collection integration, diagnostics, snapshots, recovery export, upgrades, reboots, or homelabctl Ansible-backed commands. Use for work under `ansible/` or lifecycle CLI changes that affect managed hosts or K3s nodes. Do not use for Kubernetes workload deployment, routine execution of an unchanged playbook, or Terraform provisioning.
---

# Change Node Automation

Keep local host policy small and auditable while leaving K3s lifecycle mechanics to the pinned `k3s.orchestration` collection. Design every mutation around connectivity, idempotency, secret handling, and recovery.

## Establish ownership and lifecycle

1. Read `AGENTS.md`, `docs/ansible/architecture.md`, `docs/ansible/base-role.md`, `docs/ansible/playbooks.md`, and the relevant lifecycle or recovery runbook.
2. Classify the requested behavior:
   - local Debian policy belongs in `roles/homelab_base`;
   - inventory topology, labels, taints, and K3s config belong in inventory;
   - K3s installation, joining, upgrade, and cluster-aware reboot remain upstream collection responsibilities;
   - fixed diagnostics, snapshots, recovery export, and post-operation verification may use local playbooks;
   - Kubernetes workloads belong in Helmfile, not Ansible.
3. Do not copy or fork upstream K3s roles to avoid understanding or pinning an upstream change. Advance the audited collection commit deliberately when upstream behavior is required.
4. Define which lifecycle state the command supports: pre-K3s Debian, installed server, installed agent, or controller-side recovery. Do not mix pre-K3s reboot behavior with cluster-aware reboot behavior.

## Design safe role changes

1. Add explicit defaults for new behavior and keep disruptive or lockout-prone features opt-in.
2. Assert compatibility and unsafe combinations before the first mutation. Supporting a new OS requires real tasks and tests, not only widening a version assertion.
3. Prefer idempotent modules. For commands, set accurate `changed_when` and `failed_when`, protect inputs, and explain why no suitable module exists.
4. Preserve check-mode usefulness. Guard service activation or commands that depend on packages only simulated in check mode, and report deferred work clearly.
5. Use handlers for service reload/restart and validate configuration before replacement. For SSH, keep a tested key path, validate with `sshd -t`, and reload rather than restart.
6. Bound file edits narrowly. Do not let regular expressions consume unrelated `/etc/fstab`, SSH, locale, hosts, or shell configuration.
7. Keep package-list replacement semantics explicit: an override of `homelab_base_packages` must retain every package still required.
8. Never reboot automatically from the base role. Report the correct `homelabctl node reboot` or `cluster reboot` workflow.

## Preserve inventory and secret boundaries

- Keep `inventory/hosts.yml` private and ignored; mirror non-sensitive structure and version pins in `hosts.example.yml`.
- Never place K3s tokens, SSH private keys, kubeconfig, Vault material, or application credentials in committed inventory, extra vars, Terraform variables, or cloud-init.
- Let the first K3s server generate the token when possible. Keep `no_log: true` scoped to the individual tasks that handle recovery material.
- Treat `--limit` carefully: excluding `server[0]` can skip verification, snapshots, diagnostics, or recovery export. An empty matched set is not success.
- Keep node addresses, interfaces, labels, and hardware taints explicit. Do not invent or generalize private topology from the example inventory.

## Preserve playbook safety properties

### Bootstrap and upgrade

- Keep local base preparation before upstream K3s bootstrap.
- Upgrade servers before agents and keep servers `serial: 1` to protect etcd membership.
- Wait for all nodes to become Ready after install, upgrade, or cluster-aware reboot.
- Require an exact inventory `k3s_version`; do not introduce channels or floating versions.
- Do not import or expose the upstream destructive reset playbook through `homelabctl`.

### Reboot

- Use the pre-K3s node reboot only before installation.
- Use the upstream cluster-aware reboot after installation so Kubernetes health gates server and agent progression.
- Preserve serial ordering and explicit downtime expectations for the single-node Titan cluster.

### Diagnostics

- Keep diagnostic tasks read-only with `changed_when: false`.
- Use `failed_when: false` only to continue collecting a fixed evidence set; ensure recorded return codes and output still expose individual probe failures.
- Review diagnostic output for secrets before sharing it and never commit raw host logs.

### Snapshots and recovery export

- Validate snapshot actions and names in both Go and Ansible boundaries.
- Operate on the first server deliberately and verify an exact newly-created snapshot before fetching it.
- Create controller-side export paths privately, use create-exclusive or unique destinations, validate checksums, and set directories to `0700` and files to `0600`.
- Fetch the K3s server token only under task-scoped `no_log`. Never print, register into broad debug output, or inspect it in tests.
- Keep encryption, off-device transfer, retention, and restore verification as explicit operator responsibilities; do not imply that staging is a completed backup.

## Connect the supported CLI

1. Expose normal operations through typed `homelabctl` commands rather than documenting direct `ansible-playbook` use.
2. Add validation before constructing Ansible arguments and preserve global dry-run behavior as command preview only.
3. Update command documentation and behavior/root tests for playbook path, flags, limits, check mode, and rejected input.
4. Do not add Terraform apply/destroy or a destructive cluster restore/reset command as an incidental lifecycle change.

## Test and document

1. Add focused regression playbooks for fragile file transforms and lockout-prone behavior.
2. Run `homelabctl ci check --only ansible` after every Ansible change.
3. Run `homelabctl ci check --only go-format,go-test` when homelabctl behavior changes, followed by the full `homelabctl ci check` when practical.
4. Use `homelabctl node prepare --check --limit <host>` only when live preview is explicitly requested and authorized; check mode is evidence, not proof of a successful apply.
5. Update the Ansible reference, lifecycle runbook, command docs, and current-state record when the operator contract changes.
6. Do not run a real prepare, bootstrap, upgrade, reboot, snapshot, or export unless the user explicitly requests live operation. For live work, also use `$operate-homelab-safely`.
