# Releases and updates

`homelabctl` is distributed as a checksum-protected GitHub Release for Linux
and macOS. The release binary is the normal bootstrap path; a source build is a
contributor fallback.

## What main publishes

After the complete `check` job succeeds on a push to `main`, the release job
creates one immutable version:

```text
v0.1.<GitHub workflow run number>
```

The run number is monotonically increasing for this workflow. Cancelled or
failed runs can leave harmless gaps. A successful run never moves or replaces
an older release tag. The same value is the canonical version for the
`homelabctl` GitHub Release, the `homelabctl` container image and Butler's
container image. Both images also receive the source SHA and `latest` tags, but
those aliases do not define a second product version.

The release job waits for those images to publish, then calls
`homelabctl ci release-tag` to create an annotated tag at the immutable
push-event SHA through `go-git`. On a rerun the command verifies that an existing
tag resolves to that same SHA. It never force-moves a tag; any mismatch stops
before GoReleaser, preserving the source-to-binary identity.

GoReleaser builds static binaries with the same `v0.1.<run number>` version, full source commit
and build date embedded at link time. Each release contains:

| Asset | Host |
| --- | --- |
| `homelabctl_linux_amd64.tar.gz` | 64-bit Intel/AMD Linux, including Titan's Debian installation |
| `homelabctl_linux_arm64.tar.gz` | 64-bit ARM Linux |
| `homelabctl_darwin_amd64.tar.gz` | Intel macOS |
| `homelabctl_darwin_arm64.tar.gz` | Apple silicon macOS |
| `checksums.txt` | SHA-256 hashes used for initial installation and updates |

The archive name deliberately excludes the version. The immutable release URL
and checksum identify the exact content while the stable platform asset name is
what the updater expects.

## First install on Debian

Use `/usr/local/bin/homelabctl` for a machine-wide manually managed binary. Do
not put it under `/usr/bin`, which belongs to Debian packages. Select the exact
version from the [GitHub Releases page](https://github.com/iamkhattar/homelab/releases),
then download and verify it before invoking `sudo`:

```bash
install_dir="$(mktemp -d)"
cd "$install_dir"

HOMELABCTL_VERSION="v0.1.42" # replace with the chosen published release
archive="homelabctl_linux_amd64.tar.gz"
base="https://github.com/iamkhattar/homelab/releases/download/${HOMELABCTL_VERSION}"

curl --fail --location --remote-name "${base}/${archive}"
curl --fail --location --remote-name "${base}/checksums.txt"
grep "  ${archive}$" checksums.txt | sha256sum --check --strict
tar --extract --gzip --file "$archive" homelabctl
./homelabctl version
sudo install --owner root --group root --mode 0755 homelabctl /usr/local/bin/homelabctl
/usr/local/bin/homelabctl version
```

The `grep` must produce one `OK` result. Stop if it produces no line, more than
one line or a checksum error. The temporary directory can be removed after the
installed binary has been verified.

The normal architecture runs `homelabctl` from the operator workstation. The
Linux asset also works on Titan, but installing it there does not make Titan a
control machine: repository-aware commands still require a checkout and their
native dependencies. Keep cluster administration off the single cluster node
unless there is a deliberate break-glass reason.

## Check for and install updates

Release lookup and update do not require a repository checkout:

```bash
homelabctl update --check
sudo homelabctl update
homelabctl version
```

The command selects the current operating system and architecture, ignores
drafts and pre-releases, downloads the matching archive, requires
`checksums.txt`, verifies the checksum, and replaces the running executable
atomically. If the executable is user-owned, omit `sudo`. A binary installed in
`/usr/local/bin` by the commands above is root-owned and therefore needs it.

`--dry-run update` performs the same release check but suppresses binary
replacement. The public repository normally needs no credential. Set
`GITHUB_TOKEN` only when GitHub API rate limits require authenticated lookup;
do not store that token in the repository or shell history.

There is intentionally no background auto-updater. Check and install during a
controlled operator maintenance session so a CLI change never arrives halfway
through a cluster operation.

## Pin, reinstall or roll back

Install an exact published version when reproducing a workflow or rolling back:

```bash
sudo homelabctl update --version v0.1.42
```

An exact older version is allowed. If the requested version already runs, the
command performs no write; add `--force` to replace a damaged or suspect copy:

```bash
sudo homelabctl update --version v0.1.42 --force
```

Self-update changes only the local CLI executable. It does not update Debian,
K3s, Ansible dependencies, cluster workloads, container images or the
repository checkout.

## Contributor release checks

The release contract lives in `homelabctl/.goreleaser.yaml`. Before changing
platforms, archive names, checksums or linker metadata, validate and snapshot
the pinned configuration from `homelabctl/`:

```bash
goreleaser check
GORELEASER_CURRENT_TAG=v0.1.999 goreleaser release --snapshot --clean
```

Inspect `dist/`, verify all hashes in `checksums.txt`, extract at least one
archive and run `homelabctl version`. `dist/` is generated and Git-ignored.
Normal repository validation also checks that the release job is main-only,
depends on checks and successful image publication, creates or verifies the
exact-commit tag, has bounded permissions, uses the pinned release engine and
derives an immutable semantic tag.
