# Installation overview

The first build is deliberately split into small checkpoints. Do not continue
when a checkpoint fails; fix the underlying network, SSH or operating-system
problem first.

## End state

After completing this guide:

- the AMD mini PC is named `titan` and runs Debian 13;
- Titan has a fixed DHCP lease on the private home network;
- a normal administrator account is accessible with a tested SSH key;
- Ansible owns the repeatable host baseline;
- K3s runs as a single server that also schedules workloads;
- Kubernetes secrets encryption is enabled;
- embedded etcd creates compressed local snapshots;
- Traefik is not installed by K3s because ingress will be managed separately;
- no home-router port forwards expose SSH or the Kubernetes API.

## Order of work

Use the [complete Titan setup runbook](/getting-started/titan-setup) as the
authoritative first-build checklist. The shorter pages below provide focused
background for individual stages.

1. [Install Debian on Titan](/getting-started/debian-install).
2. [Move temporary Wi-Fi to wired Ethernet](/getting-started/titan-networking)
   and give the wired interface a DHCP reservation in the router.
3. [Prepare the Ansible control machine](/getting-started/control-machine).
4. Create and edit the private inventory through `homelabctl inventory init`.
5. Verify Titan's host key, install the operator public key with
   `homelabctl node authorize-key`, and prove a new key-authenticated session.
6. Run `homelabctl node prepare`.
7. Test SSH-key access, then enable SSH hardening.
8. Reboot if Debian reports that one is required.
9. Run `homelabctl cluster bootstrap` and verify that the node is Ready.
10. Run `homelabctl cluster recovery export` and move its output off the
    workstation.

## Important boundaries

::: warning Single node
Embedded etcd does not make one machine highly available. Snapshots stored on
Titan also disappear if its disk fails. Copy recovery material off the device.
:::

::: danger Do not expose management ports
Do not forward TCP 22 or TCP 6443 from the router. Remote administration and
future Hetzner traffic will use Tailscale after its network policy is designed.
:::

::: tip Vault comes later
Vault inside this cluster cannot be the only holder of the K3s token, Vault
unseal or recovery material, or the credentials needed to rebuild the cluster.
Keep bootstrap secrets in an off-cluster password manager.
:::
