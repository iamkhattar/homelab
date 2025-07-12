data "template_file" "server_node_config" {
  template = file("${path.module}/config/cloud-init-server.yml")
  vars = {
    local_ssh_public_key = file("${path.module}/config/.ssh/id_rsa.pub")
    k3s_api_token        = var.k3s_api_token
  }
}

resource "hcloud_server" "server_node" {
  name        = "server-node-0"
  image       = var.server.image
  server_type = var.server.type
  location    = var.cluster.location
  labels = {
    type : "server"
  }

  public_net {
    ipv4_enabled = true
    ipv6_enabled = true
  }

  network {
    network_id = hcloud_network.private_network.id
    ip         = var.server.ip
  }

  firewall_ids = [hcloud_firewall.public_nodes_firewall.id, hcloud_firewall.private_nodes_firewall.id]

  user_data = data.template_file.server_node_config.rendered

  # If we don't specify this, Terraform will create the resources in parallel
  # We want this node to be created after the private network is created
  depends_on = [hcloud_network_subnet.private_network_subnet]
}
