# k8s-viewer: read+update on the k8sviewer Vault Kubernetes secrets engine
# role. Maps to the vault-managed-viewer SA bound to the built-in 'view'
# ClusterRole.

path "kubernetes/creds/k8sviewer" {
  capabilities = ["read", "update"]
}
