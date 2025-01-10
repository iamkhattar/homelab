terraform {
  cloud {
    organization = "iamkhattar"
    workspaces {
      name = "homelab"
    }
  }
  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.45"
    }
  }
}


