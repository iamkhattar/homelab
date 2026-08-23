# How to use this handbook

This is the internal engineering handbook for the Titan homelab. It documents
the platform as an operated system: what exists, how to build it, how to change
it safely, and how to recover it.

## Choose a reading path

| You are… | Read in this order |
| --- | --- |
| Setting up Titan for the first time | [Setup overview](/getting-started/overview) → [complete Titan runbook](/getting-started/titan-setup) → [K3s installation](/operations/install) |
| Operating an existing Titan | [Runbook index](/operations/) → the relevant maintenance, recovery or troubleshooting procedure |
| Changing repository code | [Engineering overview](/engineering/) → [homelabctl](/homelabctl/) or [Ansible](/ansible/) → [CI workflow](/homelabctl/deploy-build-ci) |
| Checking a fact or command | [Current state](/project/current-state) or the [reference index](/reference/) |
| Planning future work | [Architecture decisions](/project/decisions) → [roadmap](/project/roadmap) |

## Documentation types

Pages have one primary job:

- **Explanation** pages describe architecture, boundaries and reasoning.
- **Tutorials** lead through the first successful setup in a fixed order.
- **Runbooks** are repeatable operational procedures with preconditions and
  verification.
- **Reference** pages describe commands, variables and files without trying to
  teach the whole system.
- **Project records** distinguish implemented, deployed and planned work.

If a page starts mixing these jobs, split it and link between the resulting
pages. A runbook should not require the operator to reconstruct steps from an
architecture essay.

## Sources of truth

| Question | Authoritative source |
| --- | --- |
| Is it running on Titan? | [Current state](/project/current-state) plus verification recorded against the machine |
| How should an operator perform a task? | The relevant [runbook](/operations/) using `homelabctl` |
| What should Debian or K3s look like? | Ansible inventory, roles and playbooks |
| What should run inside Kubernetes? | Helmfile and chart values |
| Why was a design selected? | [Architecture decisions](/project/decisions) |
| What comes next? | [Delivery roadmap](/project/roadmap) |

Repository code proves intent, not deployment. A manifest marked ready in the
repository is not running until Titan has been checked and the current-state
page has been updated.

## Platform boundaries

Titan is one physical Debian machine and one K3s server. It is designed to be
reproducible and recoverable, not highly available. Management ports stay on
the private network. Vault, Pocket ID and the future control plane cannot be the
only holders of credentials required to rebuild the cluster that hosts them.

Continue with the [current state](/project/current-state) before choosing a
setup or operations procedure.
