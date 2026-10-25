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

# Pocket ID reads its static database credential through VSO. The path is
# explicit: the operator-wide VSO identity must not be able to read every
# database role.
path "database/static-creds/pocketid" {
  capabilities = ["read"]
}
