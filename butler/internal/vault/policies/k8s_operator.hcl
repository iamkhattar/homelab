# k8s-operator: read+update on the homelab-operator Vault Kubernetes secrets
# engine role. Maps to the vault-managed-operator SA which is bound to the
# built-in 'edit' ClusterRole.

path "kubernetes/creds/homelab-operator" {
  capabilities = ["read", "update"]
}
