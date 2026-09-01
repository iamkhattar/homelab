# Post-bootstrap recovery identity. Initialization still uses the root token in
# memory; this role accepts Pocket ID bootstrap input, owns the single
# acme-dns account document, and performs recovery health checks after the
# root token is dropped. Recovery may backfill or explicitly replace only the
# Pocket ID runtime document; the normal Butler identity never receives access
# to Vault initialization material.
path "secret/data/security/pocket-id" { capabilities = ["create", "read", "update"] }
path "secret/data/infrastructure/acme-dns" { capabilities = ["create", "read", "update"] }
path "sys/health" { capabilities = ["read"] }
