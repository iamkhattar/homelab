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
          { text: 'Cluster platform', link: '/engineering/cluster-platform' },
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

    sidebar: mainSidebar(),

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

function mainSidebar() {
  return [
    {
      text: 'Handbook',
      collapsed: false,
      items: [
        { text: 'How to use this handbook', link: '/handbook/' },
        { text: 'Current state', link: '/project/current-state' },
        { text: 'Architecture decisions', link: '/project/decisions' },
        { text: 'Delivery roadmap', link: '/project/roadmap' },
      ],
    },
    {
      text: 'Set up Titan',
      collapsed: false,
      items: [
        { text: '1. Understand the journey', link: '/getting-started/overview' },
        { text: '2. Complete setup runbook', link: '/getting-started/titan-setup' },
        { text: 'Debian installation', link: '/getting-started/debian-install' },
        { text: 'Move from Wi-Fi to Ethernet', link: '/getting-started/titan-networking' },
        { text: 'Control machine', link: '/getting-started/control-machine' },
      ],
    },
    {
      text: 'Operator runbooks',
      collapsed: false,
      items: [
        { text: 'Runbook index', link: '/operations/' },
        { text: 'Install K3s', link: '/operations/install' },
        { text: 'Bootstrap the platform', link: '/operations/platform-bootstrap' },
        { text: 'Routine maintenance', link: '/operations/maintenance' },
        { text: 'Backup and recovery', link: '/operations/backup-recovery' },
        { text: 'Home automation and Zigbee', link: '/operations/smart-home' },
        { text: 'Change and evidence record', link: '/operations/change-evidence' },
        { text: 'Troubleshooting', link: '/operations/troubleshooting' },
      ],
    },
    {
      text: 'homelabctl',
      collapsed: false,
      items: [
        { text: 'Introduction', link: '/homelabctl/' },
        { text: 'Install and configure', link: '/homelabctl/getting-started' },
        { text: 'Releases and updates', link: '/homelabctl/releases-update' },
        { text: 'Safety and execution model', link: '/homelabctl/safety-internals' },
        { text: 'Inventory and nodes', link: '/homelabctl/inventory-nodes' },
        { text: 'Cluster lifecycle and recovery', link: '/homelabctl/cluster-lifecycle' },
        { text: 'Deployments, builds and CI', link: '/homelabctl/deploy-build-ci' },
        { text: 'Documentation workflow', link: '/homelabctl/docs-workflow' },
        { text: 'Command reference', link: '/homelabctl/command-reference' },
        { text: 'Future control plane', link: '/homelabctl/control-plane' },
      ],
    },
    {
      text: 'Ansible',
      collapsed: false,
      items: [
        { text: 'Introduction', link: '/ansible/' },
        { text: 'Architecture and dependencies', link: '/ansible/architecture' },
        { text: 'Inventory model', link: '/ansible/inventory' },
        { text: 'Base role', link: '/ansible/base-role' },
        { text: 'Debian hardening', link: '/ansible/hardening' },
        { text: 'Playbooks', link: '/ansible/playbooks' },
        { text: 'Testing and upgrades', link: '/ansible/testing-upgrades' },
        { text: 'Reset or uninstall', link: '/ansible/reset-uninstall' },
      ],
    },
    {
      text: 'Engineering',
      collapsed: false,
      items: [
        { text: 'Engineering overview', link: '/engineering/' },
        { text: 'Cluster platform and applications', link: '/engineering/cluster-platform' },
        { text: 'Butler control plane', link: '/engineering/butler-control-plane' },
        { text: 'Documentation system', link: '/documentation/hosting' },
      ],
    },
    {
      text: 'Reference and future work',
      collapsed: false,
      items: [
        { text: 'Reference index', link: '/reference/' },
        { text: 'Future Hetzner and Tailscale', link: '/future/hetzner-tailscale' },
      ],
    },
  ]
}
