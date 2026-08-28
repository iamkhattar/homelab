# k8s-admin: grants read+update on the homelab-admin Vault Kubernetes secrets
# engine role. A Vault token bearing this policy can mint short-lived
# ServiceAccount tokens for vault-managed-admin (which is bound to
# cluster-admin via ClusterRoleBinding).

path "kubernetes/creds/homelab-admin" {
  capabilities = ["read", "update"]
}
