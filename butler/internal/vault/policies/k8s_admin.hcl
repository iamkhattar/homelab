# k8s-admin: grants read+update on the k8sadmin Vault Kubernetes secrets
# engine role. A Vault token bearing this policy can mint short-lived
# ServiceAccount tokens for vault-managed-admin (which is bound to
# cluster-admin via ClusterRoleBinding).

path "kubernetes/creds/k8sadmin" {
  capabilities = ["read", "update"]
}
