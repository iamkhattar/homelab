# Set the variable value in *.tfvars file
# or using the -var="hetzner_api_token=..." CLI option
variable "hetzner_cloud_api_token" {
  description = "Hetzner API token"
  sensitive   = true
  type        = string
}

variable "k3s_api_token" {
  description = "K3s Cluster API Token"
  sensitive   = true
  type        = string
}

variable "ssh_public_key" {
  description = "SSH Public Key to login to cluster"
  sensitive   = true
  type        = string
}

variable "networking" {
  type = object(
    {
      private_network_cidr = string
      private_subnet_zone  = string
      private_subnet_cidr  = string
    }
  )
  description = "Networking configuration"
  default = {
    private_network_cidr : "10.0.0.0/16"
    private_subnet_zone = "eu-central"
    private_subnet_cidr : "10.0.1.0/24"
  }
}

variable "cluster" {
  type = object(
    {
      location   = string
      datacenter = string
    }
  )
  description = "Cluster location configuration"
  default = {
    location : "nbg1",
    datacenter : "nbg1-dc3"
  }
}

variable "server" {
  type = object(
    {
      image = string
      type  = string
      ip    = string
    }
  )
  description = "Server node configuration"
  default = {
    image : "ubuntu-24.04"
    type : "cx32"
    ip : "10.0.1.1"
  }
}

variable "agent" {
  type = object(
    {
      image = string
      type  = string
      count = number
    }
  )
  description = "Agent node configuration"
  default = {
    image : "ubuntu-24.04"
    type : "cx32"
    count : 1
  }
}
