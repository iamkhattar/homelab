# Legacy migration policy retained only so Butler can recognize an earlier
# Vault-PKI installation. New clusters use cert-manager ACME DNS-01 and never
# write or attach this policy.
path "pki_int/sign/homelab-default" {
  capabilities = ["create", "update"]
}
