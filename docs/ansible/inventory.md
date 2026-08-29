# Inventory reference

The inventory models roles and placement independently from provider. Titan is
a server on the home LAN. Future Hetzner machines will normally be agents joined
through Tailscale.

## Titan

The example server entry is:

```yaml
k3s_cluster:
  children:
    server:
      hosts:
        titan:
          ansible_host: 192.168.1.50
          ansible_user: change-me
          homelab_base_manage_hostname: true
          homelab_base_hostname: titan
          homelab_base_shell_prompt_enabled: true
          homelab_base_shell_prompt_environment: HOME
```

The inventory key `titan` is also the Kubernetes node name unless K3s is given
an explicit alternative. `ansible_host` remains the fixed LAN address. Only
Titan opts into hostname enforcement and the `HOME` shell prompt. Both features
default off, so another node never inherits Titan's identity. Use a different
label, such as `HETZNER`, if a future host opts into the prompt.

## SSH hardening variables

```yaml
homelab_ssh_hardening_enabled: false
homelab_admin_authorized_keys: []
```

The example is intentionally safe for first use. After a key has been installed
and tested, add the full public key and enable hardening:

```yaml
homelab_ssh_hardening_enabled: true
homelab_admin_authorized_keys:
  - "ssh-ed25519 AAAA... operator@workstation"
```

The key list is authoritative and exclusive. Removing a key from the list
removes it from the managed administrator's `authorized_keys` file.

## K3s server configuration

`server_config_yaml` is merged into `/etc/rancher/k3s/config.yaml` by the pinned
upstream collection:

```yaml
server_config_yaml: |
  cluster-init: true
  secrets-encryption: true
  etcd-snapshot-compress: true
  etcd-snapshot-retention: 14
  disable:
    - traefik
    - metrics-server
  node-label:
    - homelab.io/location=home
    - homelab.io/hardware=mini-pc
```

- `cluster-init` selects embedded etcd, even with one server.
- `secrets-encryption` encrypts Kubernetes Secret data at rest.
- snapshots are compressed and 14 scheduled snapshots are retained locally.
- packaged Traefik is disabled because ingress will be installed declaratively.
- labels make workload placement rules readable.

## Cluster-wide variables

```yaml
k3s_version: v1.36.3+k3s1
api_endpoint: "{{ hostvars[groups['server'][0]]['ansible_host'] }}"
cluster_context: homelab
user_kubectl: true
manage_firewall: false
```

`k3s_version` is an exact deployment target, not a floating channel. The API
endpoint currently resolves to Titan's LAN address. `manage_firewall` remains
false until the Tailscale address space and inter-node policy are defined.

## Future Hetzner agent shape

Do not uncomment this until Tailscale is operating and the old cloud-init flow
has been replaced:

```yaml
agent:
  hosts:
    hetzner-agent-01:
      ansible_host: 100.x.y.z
      ansible_user: ansible
      agent_config_yaml: |
        node-label:
          - homelab.io/location=hetzner
        node-taint:
          - homelab.io/location=hetzner:NoSchedule
```

The taint prevents ordinary workloads from drifting onto a paid remote node.
Workloads must declare a matching toleration and, preferably, node affinity.

## Secrets

Do not put these in `hosts.yml`, command-line `-e` arguments, Terraform state or
cloud-init:

- K3s server and agent tokens;
- private SSH keys;
- sudo passwords;
- Vault root or recovery material;
- application credentials.

The first K3s server generates its token. Copy it through a secure operator
workflow into an off-cluster password manager after installation.
