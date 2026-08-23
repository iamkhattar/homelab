# Future Hetzner and Tailscale expansion

This is a design boundary, not a procedure to run today.

## Intended topology

Titan remains the home K3s server. Optional Hetzner machines join as agents over
Tailscale and provide additional workload capacity. They do not become public
Kubernetes API endpoints.

```text
Titan (server, home LAN)
       |
       | encrypted tailnet
       |
Hetzner agent (tainted, remote)
```

## Required work before joining an agent

- define the tailnet device-authentication and key-expiry policy;
- decide whether Ansible connects exclusively through Tailscale addresses;
- constrain TCP 6443 and K3s inter-node traffic to the required identities;
- account for pod and service CIDRs in host firewall policy;
- replace the legacy Hetzner cloud-init self-bootstrap process;
- remove plaintext K3s tokens from Terraform variables and cloud-init;
- decide how a join token is delivered at runtime without entering Terraform
  state or logs;
- test workload networking across home NAT and the remote provider;
- add monitoring for tailnet and node reachability.

## Scheduling policy

Remote agents should carry a location label and a `NoSchedule` taint:

```yaml
node-label:
  - homelab.io/location=hetzner
node-taint:
  - homelab.io/location=hetzner:NoSchedule
```

Only workloads that explicitly tolerate the taint may run remotely. Add node
affinity as well when a workload must run there rather than merely being allowed.

## Current warning

The existing files under `infra/config/cloud-init-*.yml` still refer to the old
per-node Ansible inventories and render a K3s token into cloud-init. Do not run a
Hetzner `terraform apply` until that path has been redesigned and tested.
