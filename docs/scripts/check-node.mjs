const minimumMajor = 24
const [major] = process.versions.node.split('.').map(Number)

const supported = major >= minimumMajor

if (!supported) {
  console.error(`\nThe homelab docs require Node.js >= 24; this shell is using ${process.version}.`)
  console.error('Activate Node 24 using your version manager, then run from the repository root:')
  console.error('  homelabctl docs setup')
  console.error('  homelabctl docs dev\n')
  process.exit(1)
}
