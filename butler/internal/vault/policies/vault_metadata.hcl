# vault-metadata: enumerate what exists in the KV store without reading a
# single value.
#
# Platform users who are not administrators can see that a path exists, its
# version history and its metadata, but never the stored credential. Secret
# values stay with the vault-admin policy.
#
# There is deliberately no "secret/data/*" stanza: without it, `vault kv get`
# returns a permission error while `vault kv list` and `vault kv metadata get`
# succeed.

path "secret/metadata" {
  capabilities = ["list"]
}

path "secret/metadata/*" {
  capabilities = ["read", "list"]
}
