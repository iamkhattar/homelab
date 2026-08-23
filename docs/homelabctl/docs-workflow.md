# Documentation workflow

The VitePress project is isolated under `docs/`, while all documented actions
are exposed through `homelabctl` from the repository root.

## Prepare dependencies

Activate Node.js 24 or newer using the workstation's version manager, then:

```bash
homelabctl docs setup
```

This performs the locked dependency installation. The docs package enforces its
Node engine and every VitePress lifecycle command runs an additional version
guard before loading the bundler.

## Develop locally

```bash
homelabctl docs dev
```

VitePress listens on `http://localhost:5173` by default and watches Markdown and
configuration changes.

Build and preview production output:

```bash
homelabctl docs build
homelabctl docs preview
```

`docs build` validates internal links while rendering the static site.
Generated cache and output directories remain Git-ignored.

## Build and serve the container

```bash
homelabctl build docs --tag dev
homelabctl docs serve \
  --image iamkhattar/homelab-docs:dev \
  --port 8080
```

`docs serve` starts the selected image as a temporary container named
`homelab-docs`, maps the requested local port to Nginx port 8080, and removes the
container after it stops. It does not build the image automatically.

## Node mismatch behaviour

Under an unsupported runtime, `homelabctl docs dev` stops before VitePress and
reports the active Node version. Activate the version declared in
`docs/.nvmrc`, then rerun:

```bash
homelabctl docs setup
homelabctl docs dev
```

Do not add a root `package.json` or restore the obsolete root-level docs script.

## Documentation required for every stage

Every implementation stage must update documentation in the same change:

- update [current state](/project/current-state) without claiming an unverified
  deployment;
- update the [roadmap](/project/roadmap) and decision log when scope changes;
- add the real `homelabctl` command to the appropriate task-oriented page;
- document flags, side effects, prerequisites and recovery implications;
- update install, maintenance and troubleshooting runbooks;
- run `homelabctl docs build` before considering the stage complete.

Planned commands belong in clearly marked design sections, never in runnable
procedures.
