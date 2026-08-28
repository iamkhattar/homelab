# Butler is Vault's declarative control plane after the one-time root bootstrap.
# It can manage only the mounts, auth configuration, policies and data paths
# owned by this homelab. It cannot read or rotate Vault's root token.
path "sys/mounts" { capabilities = ["read"] }
path "sys/mounts/*" { capabilities = ["create", "read", "update", "delete", "sudo"] }
path "sys/auth" { capabilities = ["read"] }
path "sys/auth/*" { capabilities = ["create", "read", "update", "delete", "sudo"] }
path "sys/policies/acl/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "sys/audit" { capabilities = ["read", "sudo"] }
path "sys/audit/*" { capabilities = ["create", "read", "update", "delete", "sudo"] }

path "auth/kubernetes/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "auth/jwt/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "secret/data/*" { capabilities = ["create", "read", "update", "delete"] }
path "secret/metadata/*" { capabilities = ["read", "list", "delete"] }
path "pki/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "pki_int/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "kubernetes/*" { capabilities = ["create", "read", "update", "delete", "list"] }
