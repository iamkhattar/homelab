# Server creation with one linked primary ip (ipv4)
resource "hcloud_primary_ip" "master_node_public_ip" {
  name          = "master_node_public_ip"
  datacenter    = var.cluster.datacenter
  type          = "ipv4"
  assignee_type = "server"
  auto_delete   = true
}

resource "hcloud_server" "master-node" {
  name        = "master-node"
  image       = var.master.image
  server_type = var.master.type
  location    = var.cluster.location
  labels = {
    type: "master"
  }
  public_net {
    ipv4         = hcloud_primary_ip.master_node_public_ip.id
    ipv4_enabled = true
    ipv6_enabled = true
  }
  network {
    network_id = hcloud_network.private_network.id
    ip         = var.master.ip
  }

  user_data = file("${path.module}/config/cloud-init-master.yaml")

  # If we don't specify this, Terraform will create the resources in parallel
  # We want this node to be created after the private network is created
  depends_on = [hcloud_network_subnet.private_network_subnet]
}

output "master_node_public_ip" {
  value = tostring(hcloud_primary_ip.master_node_public_ip.ip_address)
}