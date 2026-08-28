# k8s-viewer: read+update on the homelab-viewer Vault Kubernetes secrets
# engine role. Maps to the vault-managed-viewer SA bound to the built-in
# 'view' ClusterRole.

path "kubernetes/creds/homelab-viewer" {
  capabilities = ["read", "update"]
}
