path "secret/data/security/pocket-id" {
  capabilities = ["read"]
}

path "secret/data/databases/*" {
  capabilities = ["read"]
}

path "secret/data/storage/garage" {
  capabilities = ["read"]
}

path "secret/data/cicd/github-actions" {
  capabilities = ["read"]
}

path "secret/data/monitoring/*" {
  capabilities = ["read"]
}

path "secret/data/oauth/grafana" {
  capabilities = ["read"]
}
