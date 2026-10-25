# vault-read: read stored secret values without administering Vault.
#
# This is the deliberate exception to metadata-only access. It exists for the
# identities that must fetch a value, while vault-metadata stays the default
# for anyone who only needs to know that a path exists.

path "secret/data/*" {
  capabilities = ["read"]
}

path "secret/metadata" {
  capabilities = ["list"]
}

path "secret/metadata/*" {
  capabilities = ["read", "list"]
}
