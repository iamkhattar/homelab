---
layout: home

hero:
  name: Homelab Handbook
  text: Build, operate and evolve Titan
  tagline: Internal engineering documentation for the home-first K3s platform, its automation and its recovery model.
  actions:
    - theme: brand
      text: Start with the handbook
      link: /handbook/
    - theme: alt
      text: Set up Titan
      link: /getting-started/overview

features:
  - title: Orientation
    details: Learn what exists, what is planned and which files are authoritative before changing anything.
    link: /handbook/
  - title: Setup journey
    details: Move from bare hardware to a verified, recoverable single-node K3s cluster in explicit checkpoints.
    link: /getting-started/overview
  - title: Operator runbooks
    details: Install, maintain, back up, recover and troubleshoot Titan using supported homelabctl workflows.
    link: /operations/
  - title: Engineering guide
    details: Understand homelabctl, Ansible, CI and documentation as maintained repository components.
    link: /engineering/
---

## Start here

Use these docs like an internal platform handbook: orient yourself first, follow
a task guide when making a change, and use reference pages only when you need
implementation detail.

| If you want to… | Start with |
| --- | --- |
| Understand the platform and documentation model | [Handbook orientation](/handbook/) |
| See what is actually implemented or deployed | [Current state](/project/current-state) |
| Build Titan for the first time | [Setup journey](/getting-started/overview) |
| Operate an existing cluster | [Operator runbooks](/operations/) |
| Change the automation or CLI | [Engineering guide](/engineering/) |
| Look up a command, role or decision | [Reference index](/reference/) |

## Target foundation topology

<FoundationMap />

::: warning Target state, not deployment status
The diagram describes the first stable milestone. Consult the
[current-state page](/project/current-state) before assuming a component is
installed on Titan.
:::

The design is for a **single-node cluster**. It can be reproducible and
recoverable, but it cannot be highly available: hardware failure or maintenance
on Titan stops the cluster.

## Documentation scope

The host and K3s bootstrap layer is operational and documented. Pocket ID,
Vault, Butler and observability are implemented in the repository but remain
staged Titan deployment checkpoints. Home Assistant, Zigbee and remote Hetzner
capacity remain sequenced future work.
