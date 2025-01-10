# Configure the Hetzner Cloud Provider
provider "hcloud" {
  token = var.hetzner_cloud_api_token
}