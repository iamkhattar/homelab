import { defineConfig } from 'vitepress'

export default defineConfig({
  lang: 'en-GB',
  title: 'Homelab',
  description: 'Operations guide for the Titan K3s homelab',
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['meta', { name: 'theme-color', content: '#3451b2' }],
  ],

  themeConfig: {
    nav: [
      { text: 'Handbook', link: '/handbook/' },
      { text: 'Set up Titan', link: '/getting-started/overview' },
      { text: 'Runbooks', link: '/operations/' },
      {
        text: 'Engineering',
        items: [
          { text: 'Engineering overview', link: '/engineering/' },
          { text: 'homelabctl', link: '/homelabctl/' },
          { text: 'Ansible', link: '/ansible/' },
          { text: 'Documentation', link: '/documentation/hosting' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'Reference index', link: '/reference/' },
          { text: 'Command reference', link: '/homelabctl/command-reference' },
          { text: 'Architecture decisions', link: '/project/decisions' },
          { text: 'Roadmap', link: '/project/roadmap' },
        ],
      },
    ],

    sidebar: {
      '/handbook/': handbookSidebar(),
      '/project/': handbookSidebar(),
      '/getting-started/': [
        {
          text: 'Set up Titan',
          items: [
            { text: '1. Understand the journey', link: '/getting-started/overview' },
            { text: '2. Follow the complete runbook', link: '/getting-started/titan-setup' },
          ],
        },
        {
          text: 'Stage guides',
          items: [
            { text: 'Debian installation', link: '/getting-started/debian-install' },
            { text: 'Control machine', link: '/getting-started/control-machine' },
            { text: 'K3s installation', link: '/operations/install' },
          ],
        },
      ],
      '/operations/': [
        {
          text: 'Operator runbooks',
          items: [
            { text: 'Runbook index', link: '/operations/' },
            { text: 'Install K3s', link: '/operations/install' },
            { text: 'Routine maintenance', link: '/operations/maintenance' },
            { text: 'Backup and recovery', link: '/operations/backup-recovery' },
            { text: 'Troubleshooting', link: '/operations/troubleshooting' },
          ],
        },
      ],
      '/engineering/': engineeringSidebar(),
      '/homelabctl/': [
        {
          text: 'Learn homelabctl',
          items: [
            { text: 'Introduction', link: '/homelabctl/' },
            { text: 'Install and configure', link: '/homelabctl/getting-started' },
            { text: 'Safety and execution model', link: '/homelabctl/safety-internals' },
          ],
        },
        {
          text: 'Operator workflows',
          items: [
            { text: 'Inventory and nodes', link: '/homelabctl/inventory-nodes' },
            { text: 'Cluster lifecycle and recovery', link: '/homelabctl/cluster-lifecycle' },
            { text: 'Deployments, builds and CI', link: '/homelabctl/deploy-build-ci' },
            { text: 'Documentation workflow', link: '/homelabctl/docs-workflow' },
          ],
        },
        {
          text: 'Reference',
          items: [
            { text: 'Command reference', link: '/homelabctl/command-reference' },
            { text: 'Future control plane', link: '/homelabctl/control-plane' },
          ],
        },
      ],
      '/ansible/': [
        {
          text: 'Understand Ansible',
          items: [
            { text: 'Introduction', link: '/ansible/' },
            { text: 'Architecture and dependencies', link: '/ansible/architecture' },
            { text: 'Inventory model', link: '/ansible/inventory' },
          ],
        },
        {
          text: 'Implementation reference',
          items: [
            { text: 'Base role', link: '/ansible/base-role' },
            { text: 'Debian hardening', link: '/ansible/hardening' },
            { text: 'Playbooks', link: '/ansible/playbooks' },
            { text: 'Testing and upgrades', link: '/ansible/testing-upgrades' },
          ],
        },
      ],
      '/documentation/': engineeringSidebar(),
      '/reference/': referenceSidebar(),
      '/future/': referenceSidebar(),
    },

    search: {
      provider: 'local',
    },

    outline: {
      level: [2, 3],
      label: 'On this page',
    },

    docFooter: {
      prev: 'Previous',
      next: 'Continue',
    },

    socialLinks: [
      { icon: 'github', link: 'https://github.com/iamkhattar/homelab' },
    ],

    footer: {
      message: 'Private infrastructure, documented as code.',
    },
  },
})

function handbookSidebar() {
  return [
    {
      text: 'Homelab handbook',
      items: [
        { text: 'How to use this handbook', link: '/handbook/' },
        { text: 'Current state', link: '/project/current-state' },
        { text: 'Architecture decisions', link: '/project/decisions' },
        { text: 'Delivery roadmap', link: '/project/roadmap' },
      ],
    },
  ]
}

function engineeringSidebar() {
  return [
    {
      text: 'Engineering guide',
      items: [
        { text: 'Engineering overview', link: '/engineering/' },
        { text: 'homelabctl library and CLI', link: '/homelabctl/' },
        { text: 'Ansible automation', link: '/ansible/' },
        { text: 'Documentation system', link: '/documentation/hosting' },
        { text: 'Build and CI workflow', link: '/homelabctl/deploy-build-ci' },
      ],
    },
  ]
}

function referenceSidebar() {
  return [
    {
      text: 'Reference',
      items: [
        { text: 'Reference index', link: '/reference/' },
        { text: 'homelabctl commands', link: '/homelabctl/command-reference' },
        { text: 'Ansible base role', link: '/ansible/base-role' },
        { text: 'Ansible playbooks', link: '/ansible/playbooks' },
        { text: 'Architecture decisions', link: '/project/decisions' },
        { text: 'Future Hetzner and Tailscale', link: '/future/hetzner-tailscale' },
      ],
    },
  ]
}
