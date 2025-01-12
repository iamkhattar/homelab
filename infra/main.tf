terraform {
  required_version = ">= 1.0"
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


