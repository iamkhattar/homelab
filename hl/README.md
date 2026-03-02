# Homelab CLI (`hl`)

A Go CLI using Cobra + Viper to manage the homelab cluster.

## Command Structure

```
hl
│
├── ci                               # CI/dev tooling
│   ├── lint                         # Run all linters
│   ├── test                         # Run all tests
│   ├── fmt                          # Format all code
│   └── check                        # Single CI entry point (lint + test + fmt check)
│
├── cluster                          # Node & cluster management
│   ├── status                       # Health overview
│   ├── nodes                        # List nodes
│   ├── kubeconfig                   # Fetch/configure kubeconfig
│   ├── ssh <node>                   # SSH into a node by name
│   └── bootstrap <inventory>        # Run ansible playbook against an inventory
│
├── deploy                           # Helmfile lifecycle
│   ├── sync                         # helmfile sync
│   ├── diff                         # helmfile diff
│   └── apply [release]              # helmfile apply, optionally targeting one release
│
├── infra                            # Terraform (optional, for external nodes)
│   ├── init                         # terraform init
│   ├── plan                         # terraform plan
│   ├── apply                        # terraform apply
│   └── destroy                      # terraform destroy (with confirmation)
│
├── app                              # Interact with any deployed service
│   ├── list                         # List all apps with status + endpoints
│   ├── status <app>                 # Detailed status (pods, events, etc.)
│   ├── logs <app>                   # Tail logs
│   ├── restart <app>                # Rollout restart
│   ├── forward <app> [port]         # Port-forward + open browser
│   └── exec <app> [cmd]             # Exec into a pod
│
├── config                           # CLI configuration
│   ├── init                         # Interactive setup
│   ├── show                         # Print current config
│   └── set <key> <value>            # Set a config value
│
└── version                          # Print CLI version
```

## Design Principles

- **`hl ci check`** is the single CI entry point — pipelines call this one command.
- **`hl app`** is the generic interface for all deployed services. No per-service commands — `hl app forward vault`, `hl app exec postgres -- psql`, `hl app logs grafana` all work the same way.
- **`hl infra`** is optional — only needed when managing external Hetzner nodes.
- All commands that wrap external tools (terraform, helmfile, kubectl) respect the Viper config for paths and contexts.

## Config

Config file at `~/.homelab/config.yaml` managed via Viper:

- `cluster.kubeconfig` — path to kubeconfig
- `cluster.context` — kubectl context name
- `cluster.namespace` — default namespace
- `infra.dir` — path to terraform directory
- `helmfile.dir` — path to helmfile directory
- `services.domain` — base domain for services

## Implementation Priority

1. **Phase 1**: `config`, `ci`, `cluster`, `deploy` — core workflow, CI integration
2. **Phase 2**: `app`, `infra` — operational access
3. **Phase 3**: extended `app` capabilities as services come online
