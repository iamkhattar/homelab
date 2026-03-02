# Butler CLI Integration

Plan for `hl butler` subcommands once Butler is deployed and stable.

## Connection
- Butler URL configurable in `~/.homelab/config.yaml` as `butler.url`
- Default: use `kubectl port-forward svc/butler -n security 8080:8080`
- Auth: JWT via OIDC device flow against Pocket ID (or skip if auth not configured)

## Commands

### Now (matches current Butler API)
- `hl butler status` — show last reconcile results per reconciler
- `hl butler reconcile` — trigger a full reconcile
- `hl butler secrets rotate <path>` — rotate a specific secret

### Near-term
- `hl butler secrets list` — list all managed secret paths + metadata
- `hl butler secrets show <path>` — show keys (not values) + last rotation time
- `hl butler vault status` — init/seal/version info
- `hl butler logs` — stream recent reconciler logs

### Future (migrations, identity)
- `hl butler migrations run <name>` — trigger a specific migration
- `hl butler migrations status` — list + status of each migration
- `hl butler identity users list` — list managed users
- `hl butler identity users sync` — trigger user sync

## Butler API Endpoints Needed
- `GET /api/v1/secrets` — list managed paths
- `GET /api/v1/secrets/{path}` — metadata only (keys, created, last rotated)
- `GET /api/v1/vault/status` — proxy sys/health with friendly response
- `GET /api/v1/logs` — recent reconciler logs
- `POST /api/v1/migrations/{name}/run`
- `GET /api/v1/migrations`
- `GET /api/v1/identity/users`
- `POST /api/v1/identity/users/sync`
