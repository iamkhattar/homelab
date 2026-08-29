# Post-bootstrap recovery identity. Initialization still uses the root token in
# memory; this role exists only for write-only Pocket ID bootstrap input and
# basic recovery health checks after the root token is dropped.
path "secret/data/pocket-id/admin" { capabilities = ["create", "update"] }
path "sys/health" { capabilities = ["read"] }
