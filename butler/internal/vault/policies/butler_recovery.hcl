# Post-bootstrap recovery identity. Initialization still uses the root token in
# memory; this role accepts Pocket ID bootstrap input, owns the single
# acme-dns account document, and performs recovery health checks after the
# root token is dropped.
path "secret/data/pocket-id/admin" { capabilities = ["create", "update"] }
path "secret/data/infrastructure/acme-dns" { capabilities = ["create", "read", "update"] }
path "sys/health" { capabilities = ["read"] }
