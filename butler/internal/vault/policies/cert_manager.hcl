# Policy used by cert-manager (via the kubernetes auth method) to request
# certificates from the Vault PKI intermediate. cert-manager only ever signs
# CSRs it generates itself, so it needs update on the sign/role and issue/role
# paths and read access to the role definition.

path "pki_int/sign/homelab-default" {
  capabilities = ["create", "update"]
}

path "pki_int/issue/homelab-default" {
  capabilities = ["create", "update"]
}

path "pki_int/roles/homelab-default" {
  capabilities = ["read"]
}
