# k8s-operator: read+update on the k8soperator Vault Kubernetes secrets
# engine role. Maps to the vault-managed-operator SA which is bound to the
# built-in 'edit' ClusterRole.

path "kubernetes/creds/k8soperator" {
  capabilities = ["read", "update"]
}
