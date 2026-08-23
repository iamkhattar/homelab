# Develop and host this site

The documentation is an isolated Node/VitePress project under `docs/`. Its
package manifest, lockfile, Node version and container build context do not
affect applications elsewhere in the repository.

## Local development

The site requires Node.js 24 or newer. Activate that version with the
workstation's preferred Node version manager, then use the operator CLI from the
repository root:

```bash
homelabctl docs setup
homelabctl docs dev
```

The root intentionally has no Node package. `homelabctl` selects `docs/` as the
working directory and keeps package-manager details out of operator runbooks.

VitePress listens on `http://localhost:5173` by default. Build and preview the
production output with:

```bash
homelabctl docs build
homelabctl docs preview
```

Generated `.vitepress/cache/`, `.vitepress/dist/` and `node_modules/` directories
are ignored by Git.

## Theme and Vue components

The site extends the standard VitePress theme from `.vitepress/theme/`. Global
styles define the restrained teal/blue operational palette, dark mode, mobile
breakpoints, code blocks, tables, callouts and reduced-motion behaviour.

Reusable Vue components live under `.vitepress/theme/components/`:

| Component | Intended use |
| --- | --- |
| `RunbookHero` | Summary and safety facts at the start of a major runbook |
| `SetupOverview` | A short phase navigator for a long ordered procedure |
| `FoundationMap` | The home-first trust and recovery topology |
| `StatusLegend` | Consistent project-state semantics |

Keep normal instructions in Markdown. Add a component only when it makes
sequence, topology or status materially easier to understand. Components must
render during the production build, work without client-only state, preserve
semantic labels, support light and dark themes, collapse cleanly below 700 px
and avoid essential information that exists nowhere in the Markdown source.

## Information architecture

The handbook is organised by reader intent rather than repository directory:

| Section | Page type | Reader question |
| --- | --- | --- |
| Handbook | Orientation and project record | What is this system, and what is true today? |
| Set up Titan | Tutorial | How do I reach the first working, recoverable cluster? |
| Runbooks | Operational procedure | How do I perform or recover a production task? |
| Engineering | Explanation and contributor guide | How is a component designed and changed? |
| Reference | Lookup material | What exactly does this command, role or decision mean? |

VitePress uses one persistent grouped sidebar across the complete handbook.
Readers keep the same navigation tree while moving between tutorials,
runbooks, component manuals, references and future work. The active item shows
the current location, while groups keep the distinct page types understandable.
Section landing pages must still state their audience and provide a reading
path.

When adding content:

1. choose one primary page type;
2. place the page in the matching section;
3. add it to `mainSidebar()` at the point it should be read;
4. link to reference detail instead of copying it into a runbook;
5. end tutorials and runbooks with an acceptance condition or explicit next
   step;
6. keep deployment truth in current state and planned work in the roadmap.

The `homelabctl` and Ansible introductions describe their component contract
and package map. Detailed flags, variables and playbook behavior belong in
their reference pages rather than growing those introductions indefinitely.

After changing theme code, validate both production rendering and the affected
page through the supported commands:

```bash
homelabctl docs build
homelabctl docs preview
```

## Node version troubleshooting

Check the workstation toolchain before diagnosing VitePress:

```bash
homelabctl doctor
```

Node must report at least `v24.0.0`. An error saying that `node:util` does not
export `styleText` means an older Node runtime loaded the current Vite/VitePress
dependencies. Node 20.11.1, for example, is too old.

Activate the version declared by `docs/.nvmrc` using the local version manager,
then reinstall only the isolated docs dependencies through the CLI:

```bash
homelabctl docs setup
homelabctl docs dev
```

The npm project has engine enforcement and pre-command checks, so future version
mismatches stop with setup instructions before VitePress starts.

## Container image

Build the isolated documentation image:

```bash
homelabctl build docs --image homelab-docs --tag dev
```

Run the image locally:

```bash
homelabctl docs serve --image homelab-docs:dev --port 8080
```

Open `http://localhost:8080` and verify clean routes such as
`http://localhost:8080/operations/install`.

## Image design

The Dockerfile has two stages:

1. Node 24 installs Git in the disposable build stage, runs `npm ci`, and
   creates the VitePress production build. VitePress invokes Git while
   collecting page timestamps; Git is not copied into the runtime image.
2. The official unprivileged Nginx image serves only the generated static files.

The runtime container:

- runs Nginx without root privileges;
- listens on port 8080;
- serves VitePress clean URLs using `try_files`;
- caches fingerprinted assets for one year;
- includes a Docker health check against the site root;
- contains no Node runtime, npm cache or Markdown source.

## Internal cluster hosting

The eventual cluster release should use the image produced by
`homelabctl build docs`, then deploy it behind the cluster ingress controller.
The Kubernetes workload should use:

- container port 8080;
- a read-only root filesystem where supported;
- no Linux capabilities;
- a ClusterIP Service;
- an authenticated internal hostname if the documentation contains operational
  information that should not be public.

Do not add the deployment to the cluster until ingress, internal DNS and Pocket
ID policy have been decided.
